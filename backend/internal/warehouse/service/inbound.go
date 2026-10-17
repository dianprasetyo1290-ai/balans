package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

func inboundSnapshot(ib domain.Inbound) map[string]any {
	return map[string]any{
		"number": ib.Number, "sourceType": ib.SourceType, "sourceLabel": ib.SourceLabel, "status": ib.Status,
		"lines": len(ib.Lines),
	}
}

// CreateInbound membuat Inbound di transaksi aktif tanpa memeriksa permission (API publik untuk modul asal).
func (s *Service) CreateInbound(ctx context.Context, in domain.InboundInput, warehouseID *uuid.UUID) (domain.Inbound, error) {
	d, err := s.deps()
	if err != nil {
		return domain.Inbound{}, err
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = clockToday(s)
	}
	var ib domain.Inbound
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		var loc domain.Location
		switch {
		case in.LocationID != uuid.Nil:
			if loc, err = s.activeLocation(ctx, in.LocationID, "locationId"); err != nil {
				return err
			}
		case warehouseID != nil:
			l, ok, err := s.standardLocation(ctx, *warehouseID, domain.LocationInbound)
			if err != nil {
				return err
			}
			if !ok {
				return apperr.Validation(map[string]string{"warehouseId": "Gudang tidak punya lokasi Inbound"})
			}
			loc, in.LocationID = l, l.ID
		default:
			return apperr.Validation(map[string]string{"locationId": "Lokasi Inbound wajib diisi"})
		}
		if loc.Type != domain.LocationInbound || loc.WarehouseID == nil {
			return apperr.Validation(map[string]string{"locationId": "Lokasi harus lokasi Inbound gudang"})
		}
		if err := s.defaultDestinations(ctx, d, *loc.WarehouseID, in.Lines); err != nil {
			return err
		}
		if ib, err = domain.NewInbound(in); err != nil {
			return err
		}
		ib.WarehouseID = *loc.WarehouseID
		if ib.Number, err = d.Numbers.Next(ctx, document.TypeInbound, ib.BusinessDate); err != nil {
			return err
		}
		if ib, err = s.Docs.InsertInbound(ctx, ib, actor.UserID(ctx)); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: EntityInbound, EntityID: ib.ID, EntityLabel: ib.Number,
			After: inboundSnapshot(ib), Note: "Dibuat dari " + domain.SourceTypeLabel(ib.SourceType) + " " + ib.SourceLabel,
		})
	})
	return ib, err
}

// defaultDestinations mengisi lokasi tujuan kosong dengan lokasi standar gudang sesuai jenis produk dan
// memastikan lokasi tujuan berada di gudang yang sama.
func (s *Service) defaultDestinations(ctx context.Context, d DocumentDeps, warehouseID uuid.UUID, lines []domain.InboundLineInput) error {
	ids := make([]uuid.UUID, len(lines))
	for i, l := range lines {
		ids[i] = l.ProductID
	}
	products, err := d.Products.Products(ctx, uniq(ids))
	if err != nil {
		return err
	}
	cache := map[domain.LocationType]uuid.UUID{}
	fields := map[string]string{}
	for i := range lines {
		l := &lines[i]
		p, ok := products[l.ProductID]
		if !ok {
			fields[lineFieldName(i, "productId")] = "Produk tidak ditemukan"
			continue
		}
		if l.DestinationLocationID != nil {
			dest, err := s.activeLocation(ctx, *l.DestinationLocationID, lineFieldName(i, "destinationLocationId"))
			if err != nil {
				return err
			}
			if dest.WarehouseID == nil || *dest.WarehouseID != warehouseID || dest.Type == domain.LocationInbound {
				fields[lineFieldName(i, "destinationLocationId")] = "Lokasi tujuan harus lokasi lain di gudang yang sama"
			}
			continue
		}
		t := domain.DefaultDestinationType(string(p.Type))
		id, ok := cache[t]
		if !ok {
			loc, found, err := s.standardLocation(ctx, warehouseID, t)
			if err != nil {
				return err
			}
			if !found {
				fields[lineFieldName(i, "destinationLocationId")] = "Gudang tidak punya lokasi " + t.Label()
				continue
			}
			id, cache[t] = loc.ID, loc.ID
		}
		dest := id
		l.DestinationLocationID = &dest
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func lineFieldName(i int, f string) string { return "lines[" + itoaInt(i) + "]." + f }

// InboundFilter adalah kriteria daftar Inbound.
type InboundFilter = postgres.InboundFilter

// ListInbounds mengembalikan antrian Inbound.
func (s *Service) ListInbounds(ctx context.Context, f InboundFilter, page paging.Page) (paging.Result[postgres.InboundSummary], Refs, error) {
	if _, err := actor.Require(ctx, PermInboundRead); err != nil {
		return paging.Result[postgres.InboundSummary]{}, Refs{}, err
	}
	d, err := s.deps()
	if err != nil {
		return paging.Result[postgres.InboundSummary]{}, Refs{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Docs.ListInbounds(ctx, f, page)
	if err != nil {
		return res, Refs{}, err
	}
	locs := make([]uuid.UUID, len(res.Items))
	for i, it := range res.Items {
		locs[i] = it.Inbound.LocationID
	}
	refs, err := s.refs(ctx, d, nil, nil, locs)
	return res, refs, err
}

// InboundDetail adalah detail Inbound beserta catatan QC dan data tampilan.
type InboundDetail struct {
	Inbound domain.Inbound
	Checks  []domain.QualityCheck
	Refs    Refs
}

// GetInbound mengembalikan detail Inbound.
func (s *Service) GetInbound(ctx context.Context, id uuid.UUID) (InboundDetail, error) {
	if _, err := actor.Require(ctx, PermInboundRead); err != nil {
		return InboundDetail{}, err
	}
	return s.inboundDetail(ctx, id)
}

func (s *Service) inboundDetail(ctx context.Context, id uuid.UUID) (InboundDetail, error) {
	d, err := s.deps()
	if err != nil {
		return InboundDetail{}, err
	}
	ib, err := s.Docs.GetInbound(ctx, id, false)
	if err != nil {
		return InboundDetail{}, err
	}
	checks, err := s.Docs.QualityChecks(ctx, "inbound", id)
	if err != nil {
		return InboundDetail{}, err
	}
	var products, batches []uuid.UUID
	locs := []uuid.UUID{ib.LocationID}
	for _, l := range ib.Lines {
		products = append(products, l.ProductID)
		batches = append(batches, derefIDs(l.BatchID)...)
		locs = append(locs, l.DestinationLocationID)
		locs = append(locs, derefIDs(l.RejectLocationID)...)
	}
	refs, err := s.refs(ctx, d, products, batches, locs)
	return InboundDetail{Inbound: ib, Checks: checks, Refs: refs}, err
}

// InboundLineActions mengembalikan aksi baris Inbound untuk actor (allowedActions).
func InboundLineActions(ctx context.Context, l domain.InboundLine) []string {
	a, ok := actor.From(ctx)
	if !ok || l.Status != domain.LineQuarantine || !a.Has(PermInboundPost) {
		return []string{}
	}
	return []string{domain.ActionQC}
}

// RecordQC mencatat QC satu baris Inbound: qty lulus pindah Karantina → Tersedia di lokasi tujuan, qty ditolak
// tetap Karantina (ditahan di Inbound atau pindah ke Gudang Retur), catatan QC + foto disimpan, callback modul
// asal dipanggil, dan Inbound selesai bila semua baris sudah QC. Satu transaksi (TG 5.7.2).
func (s *Service) RecordQC(ctx context.Context, inboundID, lineID uuid.UUID, in domain.QCInput) (InboundDetail, error) {
	a, err := actor.Require(ctx, PermInboundPost)
	if err != nil {
		return InboundDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return InboundDetail{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		ib, err := s.Docs.GetInbound(ctx, inboundID, true)
		if err != nil {
			return err
		}
		line, ok := ib.Line(lineID)
		if !ok {
			return apperr.New(apperr.CodeNotFound)
		}
		if in.DestinationLocationID != nil && *in.DestinationLocationID != line.DestinationLocationID {
			dest, err := s.activeLocation(ctx, *in.DestinationLocationID, "destinationLocationId")
			if err != nil {
				return err
			}
			if dest.WarehouseID == nil || *dest.WarehouseID != ib.WarehouseID || dest.Type == domain.LocationInbound {
				return apperr.Validation(map[string]string{"destinationLocationId": "Lokasi tujuan harus lokasi lain di gudang yang sama"})
			}
		}
		now := s.now()
		if in, err = line.ApplyQC(in, a.UserID, actorName(a), now); err != nil {
			return err
		}
		src := StockSource{Type: EntityInbound, ID: ib.ID, Action: "qc:" + line.ID.String(), Label: ib.Number, BusinessDate: clockToday(s)}
		from := &StockPlace{LocationID: ib.LocationID, Status: StockQuarantine}
		var lines []StockLine
		if line.PassedQty.IsPositive() {
			lines = append(lines, StockLine{
				Kind: StockMove, SourceLineID: &line.ID, ProductID: line.ProductID, BatchID: line.BatchID, Qty: line.PassedQty,
				From: from, To: &StockPlace{LocationID: line.DestinationLocationID, Status: StockAvailable}, Note: "Lulus QC",
			})
		}
		if line.RejectedQty.IsPositive() {
			rejectAt := ib.LocationID
			if ib.RejectAction == domain.RejectToReturns {
				ret, ok, err := s.standardLocation(ctx, ib.WarehouseID, domain.LocationReturns)
				if err != nil {
					return err
				}
				if !ok {
					return apperr.Validation(map[string]string{"rejectedQty": "Gudang tidak punya lokasi Retur"})
				}
				rejectAt = ret.ID
				lines = append(lines, StockLine{
					Kind: StockMove, SourceLineID: &line.ID, ProductID: line.ProductID, BatchID: line.BatchID, Qty: line.RejectedQty,
					From: from, To: &StockPlace{LocationID: ret.ID, Status: StockQuarantine}, Note: "Ditolak QC: " + line.RejectReason,
				})
			}
			line.RejectLocationID = &rejectAt
		}
		if len(lines) > 0 {
			if _, err := d.Ledger.Post(ctx, src, lines); err != nil {
				return err
			}
		}
		completed := ib.Complete(now)
		if err := attachPhotos(ctx, d, in.PhotoFileIDs, FileOwnerQualityCheck, line.ID); err != nil {
			return err
		}
		saved, err := s.Docs.SaveInboundQC(ctx, ib, *line, domain.QualityChecks(ib, *line, in), a.UserID)
		if err != nil {
			return err
		}
		note := "QC baris " + itoaInt(line.LineNo) + ": lulus " + line.PassedQty.String() + ", ditolak " + line.RejectedQty.String()
		if err := s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionPost, EntityType: EntityInbound, EntityID: ib.ID, EntityLabel: ib.Number,
			Before: map[string]any{"line": line.LineNo, "status": domain.LineQuarantine},
			After: map[string]any{
				"line": line.LineNo, "status": line.Status, "passedQty": line.PassedQty.String(),
				"rejectedQty": line.RejectedQty.String(), "rejectReason": line.RejectReason, "inboundStatus": saved.Status,
			},
			Note: note,
		}); err != nil {
			return err
		}
		if h := s.inboundHandler(ib.SourceType); h != nil {
			return h(ctx, InboundOutcome{
				InboundID: ib.ID, InboundNumber: ib.Number, SourceType: ib.SourceType, SourceID: ib.SourceID, LineID: line.ID,
				SourceLineID: line.SourceLineID, ProductID: line.ProductID, BatchID: line.BatchID, PassedQty: line.PassedQty,
				RejectedQty: line.RejectedQty, RejectReason: line.RejectReason, DestinationLocationID: line.DestinationLocationID,
				RejectLocationID: line.RejectLocationID, Completed: completed,
			})
		}
		return nil
	})
	if err != nil {
		return InboundDetail{}, err
	}
	return s.inboundDetail(ctx, inboundID)
}

// InboundBySource membaca Inbound dokumen asal tanpa memeriksa permission (API publik).
func (s *Service) InboundBySource(ctx context.Context, sourceType string, sourceID uuid.UUID) (domain.Inbound, error) {
	if _, err := s.deps(); err != nil {
		return domain.Inbound{}, err
	}
	return s.Docs.InboundBySource(ctx, sourceType, sourceID)
}
