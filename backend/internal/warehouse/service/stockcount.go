package service

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

// Rule jurnal selisih stock count (Desain Finance & Accounting A-14) dan divisi pemiliknya.
const (
	RuleStockCountVariance = "A-14"
	AmountShortageValue    = "shortage_value"
	DivisionLogistics      = "LOG"
)

func countSnapshot(c domain.StockCount) map[string]any {
	return map[string]any{
		"number": c.Number, "status": c.Status, "countType": c.CountType, "locationId": c.LocationID,
		"businessDate": c.BusinessDate.String(), "varianceAmount": c.VarianceAmount.StringFixed(2),
		"netVariance": c.NetVariance.StringFixed(2), "lines": len(c.Lines),
	}
}

func countLabel(c domain.StockCount) string {
	if c.Number != "" {
		return c.Number
	}
	return "Draft Stock Count"
}

func countLink(id uuid.UUID) string { return "/warehouse/stock-counts/" + id.String() }

// countScope mengembalikan lokasi yang boleh dilihat actor: Merchandiser hanya lokasi konsinyasi mitra yang
// ditugaskan kepadanya, Fulfillment Leader hanya lokasi Petty Stock (Desain Role & Approval 3.5). scoped false =
// semua lokasi.
func (s *Service) countScope(ctx context.Context, d DocumentDeps, a actor.Actor) (bool, []uuid.UUID, error) {
	only := func(role string) bool { return a.HasRole(role) && len(a.Roles) == 1 }
	switch {
	case only(RoleMerchandiser):
		if d.Partners == nil {
			return true, nil, nil
		}
		partners, err := d.Partners.PartnersForMerchandiser(ctx, a.UserID)
		if err != nil {
			return true, nil, err
		}
		var ids []uuid.UUID
		for _, p := range partners {
			pid := p.ID
			locs, err := s.FindLocations(ctx, LocationFilter{
				Types: []domain.LocationType{domain.LocationConsignment}, PartyType: domain.PartyPartner, PartyID: &pid,
			})
			if err != nil {
				return true, nil, err
			}
			for _, l := range locs {
				ids = append(ids, l.ID)
			}
		}
		return true, ids, nil
	case only(RoleFulfillmentLeader):
		locs, err := s.FindLocations(ctx, LocationFilter{Types: []domain.LocationType{domain.LocationPettyStock}})
		if err != nil {
			return true, nil, err
		}
		ids := make([]uuid.UUID, len(locs))
		for i, l := range locs {
			ids[i] = l.ID
		}
		return true, ids, nil
	}
	return false, nil, nil
}

func (s *Service) requireCountScope(ctx context.Context, d DocumentDeps, a actor.Actor, locationID uuid.UUID) error {
	scoped, allowed, err := s.countScope(ctx, d, a)
	if err != nil {
		return err
	}
	if scoped && !slices.Contains(allowed, locationID) {
		return apperr.New(apperr.CodeForbidden)
	}
	return nil
}

// StockCountCreate adalah isian pembuatan stock count; satu dokumen per lokasi.
type StockCountCreate struct {
	CountType      domain.CountType
	LocationIDs    []uuid.UUID
	BusinessDate   clock.BusinessDate
	AssignedTo     *uuid.UUID
	AssignedToName string
	Note           string
}

// CreateStockCounts membuat stock count draft untuk setiap lokasi.
func (s *Service) CreateStockCounts(ctx context.Context, in StockCountCreate) ([]domain.StockCount, error) {
	a, err := actor.Require(ctx, PermCountWrite)
	if err != nil {
		return nil, err
	}
	d, err := s.deps()
	if err != nil {
		return nil, err
	}
	if len(in.LocationIDs) == 0 {
		return nil, apperr.Validation(map[string]string{"locationIds": "Pilih minimal satu lokasi"})
	}
	if len(in.LocationIDs) > 100 {
		return nil, apperr.Validation(map[string]string{"locationIds": "Maksimal 100 lokasi sekali buat"})
	}
	if in.BusinessDate.IsZero() {
		in.BusinessDate = clockToday(s)
	}
	var out []domain.StockCount
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		for _, locID := range uniq(in.LocationIDs) {
			if err := s.requireCountScope(ctx, d, a, locID); err != nil {
				return err
			}
			c, err := s.createCount(ctx, domain.StockCountInput{
				CountType: in.CountType, LocationID: locID, BusinessDate: in.BusinessDate, AssignedTo: in.AssignedTo,
				AssignedToName: in.AssignedToName, Note: in.Note,
			}, a.UserID)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return nil
	})
	return out, err
}

func (s *Service) createCount(ctx context.Context, in domain.StockCountInput, actorID uuid.UUID) (domain.StockCount, error) {
	c, err := domain.NewStockCount(in)
	if err != nil {
		return c, err
	}
	loc, err := s.activeLocation(ctx, in.LocationID, "locationIds")
	if err != nil {
		return c, err
	}
	if !slices.Contains(in.CountType.LocationTypes(), loc.Type) {
		return c, apperr.Validation(map[string]string{"locationIds": "Lokasi " + loc.Code + " tidak sesuai jenis stock count"})
	}
	if c, err = s.Docs.InsertStockCount(ctx, c, actorID); err != nil {
		return c, err
	}
	return c, s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionCreate, EntityType: EntityStockCount, EntityID: c.ID, EntityLabel: countLabel(c),
		After: countSnapshot(c), Note: "Lokasi " + loc.Code,
	})
}

// StockCountFilter adalah kriteria daftar stock count.
type StockCountFilter = postgres.StockCountFilter

// ListStockCounts mengembalikan daftar stock count sesuai scope actor.
func (s *Service) ListStockCounts(ctx context.Context, f StockCountFilter, page paging.Page) (paging.Result[postgres.StockCountSummary], Refs, error) {
	a, err := actor.Require(ctx, PermCountRead)
	if err != nil {
		return paging.Result[postgres.StockCountSummary]{}, Refs{}, err
	}
	d, err := s.deps()
	if err != nil {
		return paging.Result[postgres.StockCountSummary]{}, Refs{}, err
	}
	if f.Scoped, f.AllowedLocationIDs, err = s.countScope(ctx, d, a); err != nil {
		return paging.Result[postgres.StockCountSummary]{}, Refs{}, err
	}
	f.Search = strings.TrimSpace(f.Search)
	res, err := s.Docs.ListStockCounts(ctx, f, page)
	if err != nil {
		return res, Refs{}, err
	}
	locs := make([]uuid.UUID, len(res.Items))
	for i, it := range res.Items {
		locs[i] = it.Count.LocationID
	}
	refs, err := s.refs(ctx, d, nil, nil, locs)
	return res, refs, err
}

// StockCountDetail adalah detail stock count beserta data tampilan.
type StockCountDetail struct {
	Count domain.StockCount
	Refs  Refs
}

// GetStockCount mengembalikan detail stock count (qty sistem disembunyikan handler saat hitung buta).
func (s *Service) GetStockCount(ctx context.Context, id uuid.UUID) (StockCountDetail, error) {
	a, err := actor.Require(ctx, PermCountRead)
	if err != nil {
		return StockCountDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return StockCountDetail{}, err
	}
	det, err := s.countDetail(ctx, d, id)
	if err != nil {
		return det, err
	}
	return det, s.requireCountScope(ctx, d, a, det.Count.LocationID)
}

func (s *Service) countDetail(ctx context.Context, d DocumentDeps, id uuid.UUID) (StockCountDetail, error) {
	c, err := s.Docs.GetStockCount(ctx, id, false)
	if err != nil {
		return StockCountDetail{}, err
	}
	var products, batches []uuid.UUID
	for _, l := range c.Lines {
		products = append(products, l.ProductID)
		batches = append(batches, derefIDs(l.BatchID)...)
	}
	refs, err := s.refs(ctx, d, products, batches, []uuid.UUID{c.LocationID})
	return StockCountDetail{Count: c, Refs: refs}, err
}

// StockCountActions mengembalikan aksi yang boleh dilakukan actor (allowedActions).
func StockCountActions(ctx context.Context, c domain.StockCount) []string {
	a, ok := actor.From(ctx)
	out := []string{}
	if !ok {
		return out
	}
	for _, x := range []struct{ action, perm string }{
		{domain.ActionStart, PermCountWrite}, {domain.ActionCount, PermCountWrite}, {domain.ActionSubmit, PermCountSubmit},
		{domain.ActionRecount, PermCountWrite}, {domain.ActionCancel, PermCountWrite},
	} {
		if c.Can(x.action) && a.Has(x.perm) {
			out = append(out, x.action)
		}
	}
	return out
}

// lockCount mengunci stock count dan memeriksa versi serta scope.
func (s *Service) lockCount(ctx context.Context, d DocumentDeps, a actor.Actor, id uuid.UUID, version int) (domain.StockCount, error) {
	c, err := s.Docs.GetStockCount(ctx, id, true)
	if err != nil {
		return c, err
	}
	if version != 0 && c.Version != version {
		return c, apperr.New(apperr.CodeVersionConflict)
	}
	return c, s.requireCountScope(ctx, d, a, c.LocationID)
}

// snapshotRows membaca saldo sistem lokasi (Tersedia + Dialokasikan + Karantina) per produk × batch.
func snapshotRows(balances []StockBalance) []domain.SnapshotRow {
	type key struct {
		product uuid.UUID
		batch   uuid.UUID
	}
	agg := map[key]*domain.SnapshotRow{}
	var order []key
	for _, b := range balances {
		k := key{product: b.ProductID}
		if b.BatchID != nil {
			k.batch = *b.BatchID
		}
		row, ok := agg[k]
		if !ok {
			row = &domain.SnapshotRow{ProductID: b.ProductID, BatchID: b.BatchID, Qty: decimal.Zero, UnitCost: b.UnitCost}
			agg[k] = row
			order = append(order, k)
		}
		row.Qty = row.Qty.Add(b.Qty)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].product != order[j].product {
			return order[i].product.String() < order[j].product.String()
		}
		return order[i].batch.String() < order[j].batch.String()
	})
	out := make([]domain.SnapshotRow, len(order))
	for i, k := range order {
		out[i] = *agg[k]
	}
	return out
}

// StartCount memberi nomor SC, mengambil snapshot saldo sistem, dan membuka lembar hitung.
func (s *Service) StartCount(ctx context.Context, id uuid.UUID, version int) (StockCountDetail, error) {
	a, err := actor.Require(ctx, PermCountWrite)
	if err != nil {
		return StockCountDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return StockCountDetail{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.lockCount(ctx, d, a, id, version)
		if err != nil {
			return err
		}
		before := countSnapshot(c)
		balances, err := d.Ledger.Balances(ctx, StockBalanceQuery{
			LocationIDs: []uuid.UUID{c.LocationID}, Statuses: []string{StockAvailable, StockReserved, StockQuarantine},
		})
		if err != nil {
			return err
		}
		if err := c.Start(snapshotRows(balances), a.UserID, actorName(a), s.now()); err != nil {
			return err
		}
		if c.Number, err = d.Numbers.Next(ctx, document.TypeStockCount, c.BusinessDate); err != nil {
			return err
		}
		if c, err = s.Docs.UpdateStockCount(ctx, c, postgres.LinesReplace, nil, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionStatusChange, EntityType: EntityStockCount, EntityID: c.ID, EntityLabel: countLabel(c),
			Before: before, After: countSnapshot(c), Note: "Mulai hitung; snapshot saldo sistem diambil",
		})
	})
	if err != nil {
		return StockCountDetail{}, err
	}
	return s.countDetail(ctx, d, id)
}

// CountEntries adalah isian autosave form hitung.
type CountEntries struct {
	Lines  []domain.CountEntry
	Extras []domain.ExtraEntry
	// PhotoFileIDs nil = foto tidak diubah.
	PhotoFileIDs []uuid.UUID
	SetPhotos    bool
}

// SaveCounts menyimpan isian hitung (autosave form HP). Tidak menaikkan pemeriksaan versi agar autosave dari
// beberapa perangkat tidak saling menolak; baris terakhir yang disimpan menang.
func (s *Service) SaveCounts(ctx context.Context, id uuid.UUID, in CountEntries) (StockCountDetail, error) {
	a, err := actor.Require(ctx, PermCountWrite)
	if err != nil {
		return StockCountDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return StockCountDetail{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.lockCount(ctx, d, a, id, 0)
		if err != nil {
			return err
		}
		for i := range in.Extras {
			cost, err := s.batchUnitCost(ctx, d, in.Extras[i].ProductID, in.Extras[i].BatchID)
			if err != nil {
				return err
			}
			in.Extras[i].UnitCost = cost
		}
		changed, err := c.RecordCounts(in.Lines, in.Extras, a.UserID, s.now())
		if err != nil {
			return err
		}
		if in.SetPhotos {
			if err := c.SetPhotos(uniq(in.PhotoFileIDs)); err != nil {
				return err
			}
		}
		var photos []uuid.UUID
		photos = append(photos, c.PhotoFileIDs...)
		for _, e := range in.Lines {
			photos = append(photos, derefIDs(e.PhotoFileID)...)
		}
		if err := attachPhotos(ctx, d, uniq(photos), FileOwnerStockCount, c.ID); err != nil {
			return err
		}
		_, err = s.Docs.UpdateStockCount(ctx, c, postgres.LinesChanged, changed, a.UserID)
		return err
	})
	if err != nil {
		return StockCountDetail{}, err
	}
	return s.countDetail(ctx, d, id)
}

// batchUnitCost mengambil HPP rata-rata batch dari saldo di lokasi mana pun (cost layer per batch).
func (s *Service) batchUnitCost(ctx context.Context, d DocumentDeps, productID uuid.UUID, batchID *uuid.UUID) (decimal.Decimal, error) {
	pid := productID
	rows, err := d.Ledger.Balances(ctx, StockBalanceQuery{ProductID: &pid, BatchID: batchID})
	if err != nil {
		return decimal.Zero, err
	}
	for _, r := range rows {
		if (batchID == nil && r.BatchID == nil) || (batchID != nil && r.BatchID != nil && *r.BatchID == *batchID) {
			return r.UnitCost, nil
		}
	}
	return decimal.Zero, nil
}

// SubmitCount mengirim hasil hitung: selisih dihitung dari snapshot; ada selisih pada stock count gudang →
// approval `stock_count` dengan nominal nilai selisih absolut; tanpa selisih atau konsinyasi/vending → Selesai.
func (s *Service) SubmitCount(ctx context.Context, id uuid.UUID, version int) (StockCountDetail, error) {
	a, err := actor.Require(ctx, PermCountSubmit)
	if err != nil {
		return StockCountDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return StockCountDetail{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.lockCount(ctx, d, a, id, version)
		if err != nil {
			return err
		}
		before := countSnapshot(c)
		if err := c.Submit(a.UserID, actorName(a), s.now()); err != nil {
			return err
		}
		if c.Status == domain.StatusPendingApproval {
			if d.Approvals == nil {
				return apperr.WithMessage(apperr.CodeProviderUnavailable, "Modul approval belum aktif untuk penyesuaian stok.")
			}
			loc, err := s.Store.GetLocation(ctx, c.LocationID, false)
			if err != nil {
				return err
			}
			n := 0
			for _, l := range c.Lines {
				if !l.VarianceQty.IsZero() {
					n++
				}
			}
			if _, err := d.Approvals.Request(ctx, approval.Subject{
				Type: approval.SubjectStockCount, ID: c.ID, Label: c.Number, Link: countLink(c.ID),
				Summary: fmt.Sprintf("%s · %d baris selisih · bersih Rp%s", loc.Code, n, c.NetVariance.StringFixed(2)),
				Amount:  money.FromDecimal(c.VarianceAmount),
			}); err != nil {
				return err
			}
		}
		if c, err = s.Docs.UpdateStockCount(ctx, c, postgres.LinesAll, nil, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionSubmit, EntityType: EntityStockCount, EntityID: c.ID, EntityLabel: countLabel(c),
			Before: before, After: countSnapshot(c),
		})
	})
	if err != nil {
		return StockCountDetail{}, err
	}
	return s.countDetail(ctx, d, id)
}

// RecountCount membuka kembali stock count yang ditolak.
func (s *Service) RecountCount(ctx context.Context, id uuid.UUID, version int) (StockCountDetail, error) {
	return s.simpleCountAction(ctx, id, version, domain.ActionRecount, "")
}

// CancelCount membatalkan stock count draft atau yang sedang dihitung.
func (s *Service) CancelCount(ctx context.Context, id uuid.UUID, version int, reason string) (StockCountDetail, error) {
	return s.simpleCountAction(ctx, id, version, domain.ActionCancel, reason)
}

func (s *Service) simpleCountAction(ctx context.Context, id uuid.UUID, version int, action, reason string) (StockCountDetail, error) {
	a, err := actor.Require(ctx, PermCountWrite)
	if err != nil {
		return StockCountDetail{}, err
	}
	d, err := s.deps()
	if err != nil {
		return StockCountDetail{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		c, err := s.lockCount(ctx, d, a, id, version)
		if err != nil {
			return err
		}
		before := countSnapshot(c)
		mode := postgres.LinesNone
		auditAction := audit.ActionCancel
		if action == domain.ActionRecount {
			if err := c.Recount(); err != nil {
				return err
			}
			mode, auditAction = postgres.LinesAll, audit.ActionStatusChange
		} else if err := c.Transition(action); err != nil {
			return err
		}
		if c, err = s.Docs.UpdateStockCount(ctx, c, mode, nil, a.UserID); err != nil {
			return err
		}
		return s.Audit.Record(ctx, audit.Entry{
			Action: auditAction, EntityType: EntityStockCount, EntityID: c.ID, EntityLabel: countLabel(c),
			Before: before, After: countSnapshot(c), Note: strings.TrimSpace(reason),
		})
	})
	if err != nil {
		return StockCountDetail{}, err
	}
	return s.countDetail(ctx, d, id)
}

// OnStockCountApproved adalah handler approval `stock_count` (level terakhir): memposting penyesuaian stok
// (kurang = keluar FIFO dari Tersedia, lebih = masuk Tersedia dengan HPP snapshot) dan jurnal A-14 per akun
// persediaan di transaksi keputusan, lalu status menjadi Diposting.
func (s *Service) OnStockCountApproved(ctx context.Context, dec approval.Decided) error {
	d, err := s.deps()
	if err != nil {
		return err
	}
	c, err := s.Docs.GetStockCount(ctx, dec.Subject.ID, true)
	if err != nil {
		return err
	}
	before := countSnapshot(c)
	if err := c.Decide(true, dec.Note, s.now()); err != nil {
		return err
	}
	loc, err := s.Store.GetLocation(ctx, c.LocationID, false)
	if err != nil {
		return err
	}
	adj := c.Adjustments()
	date := c.BusinessDate
	if c.SnapshotAt != nil {
		date = clock.BusinessDateOf(*c.SnapshotAt)
	}
	if len(adj) > 0 {
		lines := make([]StockLine, len(adj))
		place := &StockPlace{LocationID: c.LocationID, Status: StockAvailable}
		for i, l := range adj {
			lineID := l.LineID
			sl := StockLine{SourceLineID: &lineID, ProductID: l.ProductID, BatchID: l.BatchID, Qty: l.Qty.Abs(), Note: "Selisih stock count " + c.Number}
			if l.Qty.IsNegative() {
				sl.Kind, sl.From, sl.BatchReason = StockIssue, place, "Selisih kurang stock count "+c.Number
			} else {
				sl.Kind, sl.To = StockReceive, place
				if l.UnitCost.IsPositive() {
					cost := l.UnitCost
					sl.UnitCost = &cost
				}
			}
			lines[i] = sl
		}
		res, err := d.Ledger.Post(ctx, StockSource{Type: EntityStockCount, ID: c.ID, Action: "adjust", Label: c.Number, BusinessDate: date}, lines)
		if err != nil {
			return err
		}
		if c.JournalEntryID, err = s.postVariance(ctx, d, c, loc, adj, res, date); err != nil {
			return err
		}
	}
	if c, err = s.Docs.UpdateStockCount(ctx, c, postgres.LinesNone, nil, dec.DecidedBy); err != nil {
		return err
	}
	if c.SubmittedBy != nil {
		s.notify(ctx, d, []uuid.UUID{*c.SubmittedBy}, "warehouse.stock_count_posted", "Stock count "+c.Number+" disetujui",
			"Penyesuaian stok diposting.", countLink(c.ID))
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionApprove, EntityType: EntityStockCount, EntityID: c.ID, EntityLabel: countLabel(c),
		Before: before, After: countSnapshot(c), Note: "Disetujui; penyesuaian stok dan jurnal A-14 diposting",
	})
}

// postVariance memposting jurnal A-14: nilai bersih per akun persediaan (kurang = beban 6401, lebih = dibalik).
func (s *Service) postVariance(ctx context.Context, d DocumentDeps, c domain.StockCount, loc domain.Location, adj []domain.AdjustmentLine, res StockResult, date clock.BusinessDate) (*uuid.UUID, error) {
	if d.Poster == nil {
		return nil, nil
	}
	ids := make([]uuid.UUID, len(adj))
	for i, l := range adj {
		ids[i] = l.ProductID
	}
	products, err := d.Products.Products(ctx, uniq(ids))
	if err != nil {
		return nil, err
	}
	values := map[string]decimal.Decimal{}
	for i, l := range adj {
		if i >= len(res.Lines) {
			break
		}
		acc := domain.InventoryAccount(string(products[l.ProductID].Type), loc.Type)
		cost := res.Lines[i].CostAmount.Abs()
		if l.Qty.IsNegative() {
			values[acc] = values[acc].Add(cost)
		} else {
			values[acc] = values[acc].Sub(cost)
		}
	}
	splits := domain.ShortageSplits(values)
	if len(splits) == 0 {
		return nil, nil
	}
	req := accounting.PostingRequest{
		RuleCode: RuleStockCountVariance, SourceType: EntityStockCount, SourceID: c.ID, SourceLabel: c.Number,
		BusinessDate: date, Description: "Selisih stock count " + c.Number + " · " + loc.Code,
	}
	for _, sp := range splits {
		req.Splits = append(req.Splits, accounting.PostingSplit{
			AmountKey: AmountShortageValue, AccountCode: sp.Account, Amount: money.FromDecimal(sp.Amount),
		})
	}
	if d.Divisions != nil {
		div, err := d.Divisions.DivisionIDByCode(ctx, DivisionLogistics)
		if err != nil {
			return nil, err
		}
		req.Dimensions.DivisionID = &div
	}
	entry, err := d.Poster.Post(ctx, req)
	if err != nil {
		return nil, err
	}
	return &entry.ID, nil
}

// OnStockCountRejected adalah handler approval `stock_count` saat ditolak: dokumen bisa dihitung ulang.
func (s *Service) OnStockCountRejected(ctx context.Context, dec approval.Decided) error {
	d, err := s.deps()
	if err != nil {
		return err
	}
	c, err := s.Docs.GetStockCount(ctx, dec.Subject.ID, true)
	if err != nil {
		return err
	}
	before := countSnapshot(c)
	if err := c.Decide(false, dec.Note, s.now()); err != nil {
		return err
	}
	if c, err = s.Docs.UpdateStockCount(ctx, c, postgres.LinesNone, nil, dec.DecidedBy); err != nil {
		return err
	}
	if c.SubmittedBy != nil {
		s.notify(ctx, d, []uuid.UUID{*c.SubmittedBy}, "warehouse.stock_count_rejected", "Stock count "+c.Number+" ditolak",
			"Hitung ulang: "+dec.Note, countLink(c.ID))
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: audit.ActionReject, EntityType: EntityStockCount, EntityID: c.ID, EntityLabel: countLabel(c),
		Before: before, After: countSnapshot(c), Note: dec.Note,
	})
}

// StockCountResult membaca hasil stock count tanpa cek permission (API publik, mis. rekonsiliasi konsinyasi).
func (s *Service) StockCountResult(ctx context.Context, id uuid.UUID) (domain.StockCount, error) {
	if _, err := s.deps(); err != nil {
		return domain.StockCount{}, err
	}
	return s.Docs.GetStockCount(ctx, id, false)
}
