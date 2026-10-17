package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

// Permission Outbound (Desain Role & Approval 3.2, 3.5).
const (
	PermOutboundRead    = "warehouse.outbound.read"
	PermOutboundWrite   = "warehouse.outbound.write"
	PermOutboundPost    = "warehouse.outbound.post"
	PermOutboundQC      = "warehouse.outbound_qc.post"
	PermOutboundQCRd    = "warehouse.outbound_qc.read"
	EntityOutbound      = "outbound"
	FileOwnerOutboundQC = "outbound_quality_check"
)

// Role yang mendapat notifikasi baris Outbound ditahan (Sales/CS, TG 5.7.2).
var heldNotifyRoles = []string{"customer_service", "sales_mt", "sales_ecommerce", RoleLogisticsLeader}

// OutboundRejection adalah permintaan stok pengganti ke dokumen asal. Qty > 0: barang ditolak QC, dokumen asal
// memindah stok reserved batch itu ke Karantina lalu mengalokasikan pengganti FEFO. Qty nol: hanya mengisi baris
// yang ditahan (alokasi ulang).
type OutboundRejection struct {
	OutboundID     uuid.UUID
	OutboundNumber string
	SourceType     string
	SourceID       uuid.UUID
	LineID         uuid.UUID
	SourceLineID   *uuid.UUID
	ProductID      uuid.UUID
	BatchID        *uuid.UUID
	LocationID     uuid.UUID
	Qty            decimal.Decimal
	// Need adalah qty yang harus dicarikan stok (qty ditolak atau qty ditahan).
	Need   decimal.Decimal
	Reason string
}

// OutboundReplacement adalah jawaban dokumen asal: lokasi Karantina barang ditolak, batch pengganti, dan kekurangan.
type OutboundReplacement struct {
	QuarantineLocationID uuid.UUID
	Batches              []domain.ReplacementBatch
	Shortfall            decimal.Decimal
}

// OutboundResult adalah Outbound selesai (semua baris lulus QC, siap diserahkan) untuk dokumen asal.
type OutboundResult struct {
	Outbound domain.Outbound
}

// OutboundHandler adalah callback dokumen asal Outbound, dipanggil di transaksi aksi Outbound.
type OutboundHandler struct {
	// Replace wajib agar QC tolak bisa dilakukan: memindah barang ditolak ke Karantina + stok pengganti FEFO.
	Replace func(ctx context.Context, r OutboundRejection) (OutboundReplacement, error)
	// Completed opsional: Outbound selesai.
	Completed func(ctx context.Context, r OutboundResult) error
}

// OutboundReturn adalah barang reject QC Outbound yang dikembalikan ke pemasok/maklon.
type OutboundReturn struct {
	OutboundID     uuid.UUID
	OutboundNumber string
	LineID         uuid.UUID
	ProductID      uuid.UUID
	BatchID        *uuid.UUID
	LocationID     uuid.UUID
	Qty            decimal.Decimal
	Reason         string
}

// QCReturns membuat retur pembelian otomatis tanpa approval (D-24) dari barang Karantina hasil QC Outbound.
// Label kosong tanpa error = barang bukan dari Penerimaan Barang pemasok/maklon (tetap di Karantina).
type QCReturns interface {
	ReturnRejected(ctx context.Context, r OutboundReturn) (string, error)
}

// RegisterOutboundHandler mendaftarkan callback dokumen asal Outbound (registrasi terakhir menang).
func (s *Service) RegisterOutboundHandler(sourceType string, h OutboundHandler) {
	s.docs.mu.Lock()
	defer s.docs.mu.Unlock()
	if s.docs.outbound == nil {
		s.docs.outbound = map[string]OutboundHandler{}
	}
	s.docs.outbound[sourceType] = h
}

func (s *Service) outboundHandler(sourceType string) OutboundHandler {
	s.docs.mu.RLock()
	defer s.docs.mu.RUnlock()
	return s.docs.outbound[sourceType]
}

func outboundSnapshot(o domain.Outbound) map[string]any {
	return map[string]any{
		"number": o.Number, "sourceType": o.SourceType, "sourceLabel": o.SourceLabel, "status": o.Status,
		"lines": len(o.Lines), "packages": len(o.Packages),
	}
}

// CreateOutbound membuat Outbound (nomor OUT) di transaksi aktif tanpa memeriksa permission (API publik).
// Lokasi asal harus lokasi di dalam gudang; stok sudah reserved oleh dokumen asal.
func (s *Service) CreateOutbound(ctx context.Context, in domain.OutboundInput) (domain.Outbound, error) {
	d, err := s.deps()
	if err != nil {
		return domain.Outbound{}, err
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = clockToday(s)
	}
	var o domain.Outbound
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		loc, err := s.activeLocation(ctx, in.LocationID, "locationId")
		if err != nil {
			return err
		}
		if loc.WarehouseID == nil {
			return apperr.Validation(map[string]string{"locationId": "Outbound hanya dari lokasi di dalam gudang"})
		}
		if o, err = domain.NewOutbound(in); err != nil {
			return err
		}
		o.WarehouseID = *loc.WarehouseID
		if o.Number, err = d.Numbers.Next(ctx, document.TypeOutbound, o.BusinessDate); err != nil {
			return err
		}
		if o, err = s.Docs.InsertOutbound(ctx, o, actor.UserID(ctx)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityOutbound, EntityID: o.ID, EntityLabel: o.Number,
			After: outboundSnapshot(o), Note: "Dibuat dari " + domain.OutboundSourceLabel(o.SourceType) + " " + o.SourceLabel,
		})
	})
	return o, err
}

// OutboundBySource membaca Outbound dokumen asal tanpa cek permission (API publik).
func (s *Service) OutboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.Outbound, error) {
	if _, err := s.deps(); err != nil {
		return domain.Outbound{}, err
	}
	return s.Docs.OutboundBySource(ctx, sourceType, sourceID)
}

// CancelOutboundBySource membatalkan Outbound terbuka milik dokumen asal (DO dibatalkan) tanpa cek permission.
// Tanpa Outbound atau Outbound sudah selesai/dibatalkan: tidak melakukan apa pun.
func (s *Service) CancelOutboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID, reason string) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Docs.OutboundBySource(ctx, sourceType, sourceID)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if cur.Status != domain.StatusInProgress {
			return nil
		}
		_, err = s.mutateOutbound(ctx, cur.ID, nil, audit.ActionCancel, func(_ context.Context, o *domain.Outbound, _ *postgres.OutboundWrite) (string, error) {
			return "Dibatalkan: " + reason, o.Cancel(reason, s.now())
		})
		return err
	})
}

// OutboundFilter adalah kriteria daftar Outbound.
type OutboundFilter = postgres.OutboundFilter

// ListOutbounds mengembalikan daftar Outbound.
func (s *Service) ListOutbounds(ctx context.Context, f OutboundFilter, page paging.Page) (paging.Result[postgres.OutboundSummary], Refs, error) {
	if _, err := actor.RequireAny(ctx, PermOutboundRead, PermOutboundQCRd); err != nil {
		return paging.Result[postgres.OutboundSummary]{}, Refs{}, err
	}
	d, err := s.deps()
	if err != nil {
		return paging.Result[postgres.OutboundSummary]{}, Refs{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Docs.ListOutbounds(ctx, f, page)
	if err != nil {
		return res, Refs{}, err
	}
	locs := make([]uuid.UUID, len(res.Items))
	for i, it := range res.Items {
		locs[i] = it.Outbound.LocationID
	}
	refs, err := s.refs(ctx, d, nil, nil, locs)
	return res, refs, err
}

// OutboundDetail adalah detail Outbound beserta catatan QC dan data tampilan.
type OutboundDetail struct {
	Outbound domain.Outbound
	Checks   []domain.QualityCheck
	Refs     Refs
}

// GetOutbound mengembalikan detail Outbound.
func (s *Service) GetOutbound(ctx context.Context, id uuid.UUID) (OutboundDetail, error) {
	if _, err := actor.RequireAny(ctx, PermOutboundRead, PermOutboundQCRd); err != nil {
		return OutboundDetail{}, err
	}
	return s.outboundDetail(ctx, id)
}

func (s *Service) outboundDetail(ctx context.Context, id uuid.UUID) (OutboundDetail, error) {
	d, err := s.deps()
	if err != nil {
		return OutboundDetail{}, err
	}
	o, err := s.Docs.GetOutbound(ctx, id, false)
	if err != nil {
		return OutboundDetail{}, err
	}
	checks, err := s.Docs.QualityChecks(ctx, "outbound", id)
	if err != nil {
		return OutboundDetail{}, err
	}
	var products, batches []uuid.UUID
	for _, l := range o.Lines {
		products = append(products, l.ProductID)
		batches = append(batches, derefIDs(l.BatchID)...)
	}
	refs, err := s.refs(ctx, d, products, batches, []uuid.UUID{o.LocationID})
	return OutboundDetail{Outbound: o, Checks: checks, Refs: refs}, err
}

// OutboundActions mengembalikan aksi header Outbound untuk actor.
func OutboundActions(ctx context.Context, o domain.Outbound) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok || !a.Has(PermOutboundWrite) {
		return out
	}
	return append(out, o.Actions()...)
}

// OutboundLineActions mengembalikan aksi baris Outbound untuk actor; QC tidak ditawarkan ke penyiap barang.
func OutboundLineActions(ctx context.Context, o domain.Outbound, l domain.OutboundLine) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok {
		return out
	}
	for _, act := range o.LineActions(l) {
		switch act {
		case domain.ActionQC:
			if a.Has(PermOutboundQC) && (o.PreparedBy == nil || *o.PreparedBy != a.UserID) {
				out = append(out, act)
			}
		case domain.ActionReallocate:
			if a.Has(PermOutboundWrite) {
				out = append(out, act)
			}
		case domain.ActionDrop:
			if a.Has(PermOutboundPost) {
				out = append(out, act)
			}
		}
	}
	return out
}

type outboundMutation func(ctx context.Context, o *domain.Outbound, w *postgres.OutboundWrite) (string, error)

// mutateOutbound mengunci Outbound, menjalankan fn, menyimpan, mencatat audit, dan memanggil handler selesai.
func (s *Service) mutateOutbound(ctx context.Context, id uuid.UUID, version *int, action audit.Action, fn outboundMutation) (domain.Outbound, error) {
	var out domain.Outbound
	err := s.Tx.WithTx(ctx, func(ctx context.Context) error {
		o, err := s.Docs.GetOutbound(ctx, id, true)
		if err != nil {
			return err
		}
		if version != nil && *version != o.Version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := outboundSnapshot(o)
		w := postgres.OutboundWrite{}
		note, err := fn(ctx, &o, &w)
		if err != nil {
			return err
		}
		wasOpen := o.Status == domain.StatusInProgress
		if a, ok := actor.From(ctx); ok && wasOpen {
			o.TryComplete(a.UserID, actorName(a), s.now())
		}
		if out, err = s.Docs.SaveOutbound(ctx, o, w, actor.UserID(ctx)); err != nil {
			return err
		}
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: action, EntityType: EntityOutbound, EntityID: out.ID, EntityLabel: out.Number,
			Before: before, After: outboundSnapshot(out), Note: note,
		}); err != nil {
			return err
		}
		if wasOpen && out.Status == domain.StatusCompleted {
			if h := s.outboundHandler(out.SourceType); h.Completed != nil {
				return h.Completed(ctx, OutboundResult{Outbound: out})
			}
		}
		return nil
	})
	return out, err
}

// PickOutbound mencatat picking per batch oleh Staf Gudang (prepared_by). lineIDs kosong = semua baris.
func (s *Service) PickOutbound(ctx context.Context, id uuid.UUID, version int, lineIDs []uuid.UUID) (OutboundDetail, error) {
	a, err := actor.Require(ctx, PermOutboundWrite)
	if err != nil {
		return OutboundDetail{}, err
	}
	if _, err := s.mutateOutbound(ctx, id, &version, audit.ActionUpdate, func(_ context.Context, o *domain.Outbound, _ *postgres.OutboundWrite) (string, error) {
		return "Picking oleh " + actorName(a), o.Pick(lineIDs, a.UserID, actorName(a))
	}); err != nil {
		return OutboundDetail{}, err
	}
	return s.outboundDetail(ctx, id)
}

// PackOutbound mencatat koli saat packing (S-316) oleh Staf Gudang.
func (s *Service) PackOutbound(ctx context.Context, id uuid.UUID, version int, pkgs []domain.PackageInput) (OutboundDetail, error) {
	a, err := actor.Require(ctx, PermOutboundWrite)
	if err != nil {
		return OutboundDetail{}, err
	}
	if _, err := s.mutateOutbound(ctx, id, &version, audit.ActionUpdate, func(_ context.Context, o *domain.Outbound, w *postgres.OutboundWrite) (string, error) {
		w.Packages = true
		return "Packing " + itoaInt(len(pkgs)) + " koli oleh " + actorName(a), o.Pack(pkgs, a.UserID, actorName(a), s.now())
	}); err != nil {
		return OutboundDetail{}, err
	}
	return s.outboundDetail(ctx, id)
}

// RecordOutboundQC mencatat QC satu baris oleh Fulfillment Leader/Merchandiser yang bukan penyiap barang. Qty
// ditolak: dokumen asal memindahkannya ke Karantina dan langsung mengalokasikan stok pengganti FEFO (baris
// pengganti menunggu picking ulang); stok pengganti kurang → baris ditahan + notifikasi Sales/CS; barang dari
// pemasok/maklon langsung diretur tanpa approval (D-24). Satu transaksi (TG 5.7.2).
func (s *Service) RecordOutboundQC(ctx context.Context, id, lineID uuid.UUID, in domain.OutboundQCInput) (OutboundDetail, error) {
	a, err := actor.Require(ctx, PermOutboundQC)
	if err != nil {
		return OutboundDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return OutboundDetail{}, err
	}
	var held bool
	saved, err := s.mutateOutbound(ctx, id, nil, audit.ActionPost, func(ctx context.Context, o *domain.Outbound, w *postgres.OutboundWrite) (string, error) {
		if err := o.RequireChecker(a.UserID); err != nil {
			return "", err
		}
		line, ok := o.Line(lineID)
		if !ok {
			return "", apperr.New(apperr.CodeNotFound)
		}
		if in, err = line.ApplyQC(in, a.UserID, actorName(a), s.now()); err != nil {
			return "", err
		}
		note := "QC baris " + itoaInt(line.LineNo) + ": lulus " + line.PassedQty.String() + ", ditolak " + line.RejectedQty.String()
		w.Checks = domain.OutboundQualityChecks(*o, *line, in)
		if err := attachPhotos(ctx, d, in.PhotoFileIDs, FileOwnerOutboundQC, line.ID); err != nil {
			return "", err
		}
		if !line.RejectedQty.IsPositive() {
			return note, nil
		}
		h := s.outboundHandler(o.SourceType)
		if h.Replace == nil {
			return "", apperr.WithMessage(apperr.CodeProviderUnavailable,
				"Dokumen asal "+domain.OutboundSourceLabel(o.SourceType)+" belum mendukung stok pengganti QC Outbound.")
		}
		rejected := *line
		rep, err := h.Replace(ctx, OutboundRejection{
			OutboundID: o.ID, OutboundNumber: o.Number, SourceType: o.SourceType, SourceID: o.SourceID, LineID: rejected.ID,
			SourceLineID: rejected.SourceLineID, ProductID: rejected.ProductID, BatchID: rejected.BatchID, LocationID: o.LocationID,
			Qty: rejected.RejectedQty, Need: rejected.RejectedQty, Reason: rejected.RejectReason,
		})
		if err != nil {
			return "", err
		}
		if d.Returns != nil {
			loc := rep.QuarantineLocationID
			if loc == uuid.Nil {
				loc = o.LocationID
			}
			label, err := d.Returns.ReturnRejected(ctx, OutboundReturn{
				OutboundID: o.ID, OutboundNumber: o.Number, LineID: rejected.ID, ProductID: rejected.ProductID,
				BatchID: rejected.BatchID, LocationID: loc, Qty: rejected.RejectedQty, Reason: rejected.RejectReason,
			})
			if err != nil {
				return "", err
			}
			line, _ = o.Line(lineID)
			line.ReturnLabel = label
			if label != "" {
				note += "; diretur ke pemasok/maklon " + label
			}
		}
		o.AddReplacement(rejected, rep.Batches, rep.Shortfall)
		if rep.Shortfall.IsPositive() {
			held = true
			note += "; stok pengganti kurang " + rep.Shortfall.String() + " (baris ditahan)"
		}
		return note, nil
	})
	if err != nil {
		return OutboundDetail{}, err
	}
	if held {
		s.notify(ctx, d, s.heldRecipients(ctx, d), "outbound_held", "Baris Outbound ditahan: "+saved.Number,
			"Stok pengganti QC Outbound "+saved.SourceLabel+" tidak cukup. Baris ditahan menunggu stok; kirim sebagian hanya atas keputusan user.",
			"/warehouse/outbounds/"+saved.ID.String())
	}
	return s.outboundDetail(ctx, id)
}

func (s *Service) heldRecipients(ctx context.Context, d DocumentDeps) []uuid.UUID {
	var ids []uuid.UUID
	for _, r := range heldNotifyRoles {
		ids = append(ids, s.roleUsers(ctx, d, r)...)
	}
	return ids
}

// ReallocateOutboundLine mencoba lagi mengalokasikan stok untuk baris yang ditahan.
func (s *Service) ReallocateOutboundLine(ctx context.Context, id, lineID uuid.UUID) (OutboundDetail, error) {
	if _, err := actor.Require(ctx, PermOutboundWrite); err != nil {
		return OutboundDetail{}, err
	}
	if _, err := s.mutateOutbound(ctx, id, nil, audit.ActionUpdate, func(ctx context.Context, o *domain.Outbound, _ *postgres.OutboundWrite) (string, error) {
		if o.Status != domain.StatusInProgress {
			return "", apperr.InvalidStateTransition("outbound", string(o.Status), domain.ActionReallocate)
		}
		line, ok := o.Line(lineID)
		if !ok {
			return "", apperr.New(apperr.CodeNotFound)
		}
		if line.Status != domain.OutboundHeld {
			return "", apperr.InvalidStateTransition("outbound_line", string(line.Status), domain.ActionReallocate)
		}
		h := s.outboundHandler(o.SourceType)
		if h.Replace == nil {
			return "", apperr.New(apperr.CodeProviderUnavailable)
		}
		rep, err := h.Replace(ctx, OutboundRejection{
			OutboundID: o.ID, OutboundNumber: o.Number, SourceType: o.SourceType, SourceID: o.SourceID, LineID: line.ID,
			SourceLineID: line.SourceLineID, ProductID: line.ProductID, LocationID: o.LocationID, Qty: decimal.Zero,
			Need: line.AllocatedQty,
		})
		if err != nil {
			return "", err
		}
		got := decimal.Zero
		for _, b := range rep.Batches {
			got = got.Add(b.Qty)
		}
		if !got.IsPositive() {
			return "", apperr.WithMessage(apperr.CodeStockInsufficient, "Stok tersedia belum cukup untuk baris yang ditahan.")
		}
		return "Alokasi ulang baris " + itoaInt(line.LineNo) + ": " + got.String(), o.Refill(lineID, rep.Batches)
	}); err != nil {
		return OutboundDetail{}, err
	}
	return s.outboundDetail(ctx, id)
}

// DropOutboundLine: qty baris ditahan tidak ikut dikirim (pengiriman sebagian atas keputusan user).
func (s *Service) DropOutboundLine(ctx context.Context, id, lineID uuid.UUID, reason string) (OutboundDetail, error) {
	a, err := actor.Require(ctx, PermOutboundPost)
	if err != nil {
		return OutboundDetail{}, err
	}
	if _, err := s.mutateOutbound(ctx, id, nil, audit.ActionUpdate, func(_ context.Context, o *domain.Outbound, _ *postgres.OutboundWrite) (string, error) {
		return "Kirim sebagian oleh " + actorName(a) + ": " + reason, o.Drop(lineID, reason)
	}); err != nil {
		return OutboundDetail{}, err
	}
	return s.outboundDetail(ctx, id)
}
