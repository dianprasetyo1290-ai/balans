package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/catalog"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

// ProductRef dan LocationRef adalah ringkasan master untuk tampilan.
type ProductRef struct {
	ID       uuid.UUID
	SKU      string
	Name     string
	UOM      string
	HasBatch bool
}

// LocationRef adalah ringkasan lokasi untuk tampilan.
type LocationRef struct {
	ID            uuid.UUID
	Code          string
	Name          string
	WarehouseName string
}

// Refs adalah peta master yang dirujuk hasil query.
type Refs struct {
	Products  map[uuid.UUID]ProductRef
	Locations map[uuid.UUID]LocationRef
}

// Product mengembalikan ref produk (ID saja bila tidak ditemukan).
func (r Refs) Product(id uuid.UUID) ProductRef {
	if p, ok := r.Products[id]; ok {
		return p
	}
	return ProductRef{ID: id, SKU: id.String()}
}

// Location mengembalikan ref lokasi (ID saja bila tidak ditemukan).
func (r Refs) Location(id uuid.UUID) LocationRef {
	if l, ok := r.Locations[id]; ok {
		return l
	}
	return LocationRef{ID: id, Code: id.String()}
}

func (s *Service) refs(ctx context.Context, productIDs, locationIDs []uuid.UUID) (Refs, error) {
	out := Refs{Products: map[uuid.UUID]ProductRef{}, Locations: map[uuid.UUID]LocationRef{}}
	if len(productIDs) > 0 {
		prods, err := s.Products.Products(ctx, uniq(productIDs))
		if err != nil {
			return Refs{}, err
		}
		for id, p := range prods {
			out.Products[id] = ProductRef{ID: id, SKU: p.SKU, Name: p.DisplayName(), UOM: p.UOM, HasBatch: p.HasBatch}
		}
	}
	if len(locationIDs) > 0 {
		locs, err := s.Locations.Locations(ctx, uniq(locationIDs))
		if err != nil {
			return Refs{}, err
		}
		for id, l := range locs {
			out.Locations[id] = LocationRef{ID: id, Code: l.Code, Name: l.Name, WarehouseName: l.WarehouseName}
		}
	}
	return out, nil
}

// --- Stok per Lokasi --------------------------------------------------------------------------

// BalanceFilter adalah kriteria Stok per Lokasi.
type BalanceFilter = postgres.BalanceFilter

// BalancePage adalah satu halaman Stok per Lokasi.
type BalancePage struct {
	Items    []postgres.BalanceRow
	Page     paging.Page
	Totals   postgres.BalanceTotals
	Refs     Refs
	Today    clock.BusinessDate
	NearDays int
}

// ListBalances mengembalikan saldo > 0 per produk × lokasi × batch × status.
func (s *Service) ListBalances(ctx context.Context, f BalanceFilter, page paging.Page) (BalancePage, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return BalancePage{}, err
	}
	for _, st := range f.Statuses {
		if !st.Valid() {
			return BalancePage{}, apperr.Validation(map[string]string{"status": "Status stok tidak valid"})
		}
	}
	rows, totals, err := s.Store.ListBalances(ctx, f, page)
	if err != nil {
		return BalancePage{}, err
	}
	var pids, lids []uuid.UUID
	for _, r := range rows {
		pids, lids = append(pids, r.Key.ProductID), append(lids, r.Key.LocationID)
	}
	refs, err := s.refs(ctx, pids, lids)
	if err != nil {
		return BalancePage{}, err
	}
	return BalancePage{Items: rows, Page: page, Totals: totals, Refs: refs, Today: s.today(), NearDays: domain.DefaultNearExpiryDays}, nil
}

// --- Kartu Stok -------------------------------------------------------------------------------

// CardFilter adalah kriteria Kartu Stok.
type CardFilter = postgres.CardFilter

// StockCard adalah kartu stok satu SKU (opsional per lokasi/batch/status) dengan saldo berjalan.
type StockCard struct {
	Summary postgres.CardSummary
	Lines   []postgres.CardLine
	Page    paging.Page
	Product ProductRef
	Refs    Refs
}

// StockCard membaca kartu stok dari stock_movements (INV-01).
func (s *Service) StockCard(ctx context.Context, f CardFilter, page paging.Page) (StockCard, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return StockCard{}, err
	}
	fields := map[string]string{}
	if f.ProductID == uuid.Nil {
		fields["productId"] = "Produk wajib dipilih"
	}
	if f.Status != "" && !f.Status.Valid() {
		fields["status"] = "Status stok tidak valid"
	}
	if f.DateFrom.IsZero() || f.DateTo.IsZero() || f.DateTo.Before(f.DateFrom) {
		fields["dateTo"] = "Rentang tanggal tidak valid"
	}
	if len(fields) > 0 {
		return StockCard{}, apperr.Validation(fields)
	}
	sum, lines, err := s.Store.StockCard(ctx, f, page)
	if err != nil {
		return StockCard{}, err
	}
	lids := make([]uuid.UUID, len(lines))
	for i, l := range lines {
		lids[i] = l.LocationID
	}
	refs, err := s.refs(ctx, []uuid.UUID{f.ProductID}, lids)
	if err != nil {
		return StockCard{}, err
	}
	return StockCard{Summary: sum, Lines: lines, Page: page, Product: refs.Product(f.ProductID), Refs: refs}, nil
}

// --- Batch & Kedaluwarsa ----------------------------------------------------------------------

// BatchQuery adalah kriteria halaman Batch & Kedaluwarsa.
type BatchQuery struct {
	ProductID *uuid.UUID
	Search    string
	// State kosong = semua; expired / near_expiry / ok / no_expiry.
	State     domain.ExpiryState
	NearDays  int
	WithStock bool
}

// BatchPage adalah satu halaman batch.
type BatchPage struct {
	Items    []postgres.BatchSummary
	Page     paging.Page
	Total    int64
	Refs     Refs
	Today    clock.BusinessDate
	NearDays int
}

// ListBatches mengembalikan batch berurutan FEFO beserta qty per status.
func (s *Service) ListBatches(ctx context.Context, q BatchQuery, page paging.Page) (BatchPage, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return BatchPage{}, err
	}
	today := s.today()
	near := q.NearDays
	if near <= 0 {
		near = domain.DefaultNearExpiryDays
	}
	f := postgres.BatchFilter{ProductID: q.ProductID, Search: strings.ToUpper(strings.TrimSpace(q.Search)), WithStock: q.WithStock}
	yesterday, nearEnd, afterNear := today.AddDays(-1), today.AddDays(near), today.AddDays(near+1)
	switch q.State {
	case "":
	case domain.ExpiryExpired:
		f.ExpiryOnly, f.ExpiryTo = true, &yesterday
	case domain.ExpiryNear:
		f.ExpiryOnly, f.ExpiryFrom, f.ExpiryTo = true, &today, &nearEnd
	case domain.ExpiryOK:
		f.ExpiryOnly, f.ExpiryFrom = true, &afterNear
	case domain.ExpiryNone:
		f.NoExpiryOnly = true
	default:
		return BatchPage{}, apperr.Validation(map[string]string{"state": "Kondisi kedaluwarsa tidak valid"})
	}
	res, err := s.Store.ListBatches(ctx, f, page)
	if err != nil {
		return BatchPage{}, err
	}
	return s.batchPage(ctx, res, today, near)
}

func (s *Service) batchPage(ctx context.Context, res paging.Result[postgres.BatchSummary], today clock.BusinessDate, near int) (BatchPage, error) {
	pids := make([]uuid.UUID, len(res.Items))
	for i, b := range res.Items {
		pids[i] = b.ProductID
	}
	refs, err := s.refs(ctx, pids, nil)
	if err != nil {
		return BatchPage{}, err
	}
	return BatchPage{Items: res.Items, Page: res.Page, Total: res.Total, Refs: refs, Today: today, NearDays: near}, nil
}

// --- Saran FEFO -------------------------------------------------------------------------------

// SuggestedBatch adalah satu batch di saran FEFO.
type SuggestedBatch struct {
	Candidate    domain.Candidate
	SuggestedQty decimal.Decimal
}

// Suggestion adalah saran batch FEFO untuk mengambil qty dari satu lokasi (S-215).
type Suggestion struct {
	Product      ProductRef
	Location     LocationRef
	RequestedQty decimal.Decimal
	AllocatedQty decimal.Decimal
	Shortfall    decimal.Decimal
	// Items: batch available yang belum kedaluwarsa dalam urutan FEFO; SuggestedQty 0 = tidak perlu diambil.
	Items []SuggestedBatch
	// Expired: batch available yang sudah kedaluwarsa (tidak diusulkan).
	Expired []domain.Candidate
	// Quarantine: total stok Karantina di lokasi (tidak bisa dialokasikan).
	Quarantine decimal.Decimal
}

// SuggestBatches mengusulkan batch FEFO stok available satu produk di satu lokasi. Batch kedaluwarsa dan stok
// Karantina tidak diusulkan. Tidak memeriksa permission (dipakai modul dokumen).
func (s *Service) SuggestBatches(ctx context.Context, productID, locationID uuid.UUID, qty decimal.Decimal) (Suggestion, error) {
	if msg := domain.ValidateQty(qty); msg != "" {
		return Suggestion{}, apperr.Validation(map[string]string{"qty": msg})
	}
	refs, err := s.refs(ctx, []uuid.UUID{productID}, []uuid.UUID{locationID})
	if err != nil {
		return Suggestion{}, err
	}
	if _, ok := refs.Products[productID]; !ok {
		return Suggestion{}, apperr.Validation(map[string]string{"productId": "Produk tidak ditemukan"})
	}
	if _, ok := refs.Locations[locationID]; !ok {
		return Suggestion{}, apperr.Validation(map[string]string{"locationId": "Lokasi tidak ditemukan"})
	}
	today := s.today()
	cands, err := s.Store.BalanceCandidates(ctx, productID, locationID, domain.StatusAvailable)
	if err != nil {
		return Suggestion{}, err
	}
	quarantine, err := s.Store.BalanceCandidates(ctx, productID, locationID, domain.StatusQuarantine)
	if err != nil {
		return Suggestion{}, err
	}
	allocs, shortfall := s.Allocator.Allocate(cands, qty, today)
	taken := map[string]decimal.Decimal{}
	for _, a := range allocs {
		taken[batchString(a.Candidate.BatchID)] = a.Qty
	}
	out := Suggestion{
		Product: refs.Product(productID), Location: refs.Location(locationID), RequestedQty: qty,
		AllocatedQty: qty.Sub(shortfall), Shortfall: shortfall, Quarantine: sumQty(quarantine),
	}
	for _, c := range domain.Eligible(cands, today) {
		out.Items = append(out.Items, SuggestedBatch{Candidate: c, SuggestedQty: taken[batchString(c.BatchID)]})
	}
	for _, c := range cands {
		if c.Expired(today) {
			out.Expired = append(out.Expired, c)
		}
	}
	domain.SortFEFO(out.Expired)
	return out, nil
}

// SuggestBatchesForUser seperti SuggestBatches dengan cek permission inventory.stock.read (halaman ERP).
func (s *Service) SuggestBatchesForUser(ctx context.Context, productID, locationID uuid.UUID, qty decimal.Decimal) (Suggestion, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return Suggestion{}, err
	}
	return s.SuggestBatches(ctx, productID, locationID, qty)
}

// --- HPP per Batch ----------------------------------------------------------------------------

// LayerFilter adalah kriteria HPP per Batch.
type LayerFilter = postgres.LayerFilter

// LayerPage adalah satu halaman cost layer.
type LayerPage struct {
	Items []postgres.LayerRow
	Page  paging.Page
	Total int64
	Refs  Refs
}

// ListCostLayers mengembalikan cost layer FIFO per batch (INV-03).
func (s *Service) ListCostLayers(ctx context.Context, f LayerFilter, page paging.Page) (LayerPage, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return LayerPage{}, err
	}
	res, err := s.Store.ListLayers(ctx, f, page)
	if err != nil {
		return LayerPage{}, err
	}
	pids := make([]uuid.UUID, len(res.Items))
	for i, l := range res.Items {
		pids[i] = l.ProductID
	}
	refs, err := s.refs(ctx, pids, nil)
	if err != nil {
		return LayerPage{}, err
	}
	return LayerPage{Items: res.Items, Page: page, Total: res.Total, Refs: refs}, nil
}

// LayerConsumptions adalah jejak konsumsi satu cost layer.
type LayerConsumptions struct {
	Layer postgres.LayerRow
	Items []postgres.ConsumptionRow
	Refs  Refs
}

// CostLayerConsumptions mengembalikan jejak HPP satu cost layer (RPT-04).
func (s *Service) CostLayerConsumptions(ctx context.Context, layerID uuid.UUID) (LayerConsumptions, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return LayerConsumptions{}, err
	}
	layer, err := s.Store.GetLayer(ctx, layerID)
	if err != nil {
		return LayerConsumptions{}, err
	}
	items, err := s.Store.LayerConsumptions(ctx, layerID)
	if err != nil {
		return LayerConsumptions{}, err
	}
	lids := make([]uuid.UUID, len(items))
	for i, c := range items {
		lids[i] = c.LocationID
	}
	refs, err := s.refs(ctx, []uuid.UUID{layer.ProductID}, lids)
	if err != nil {
		return LayerConsumptions{}, err
	}
	return LayerConsumptions{Layer: layer, Items: items, Refs: refs}, nil
}

// --- Stok tersedia & bundel -------------------------------------------------------------------

// AvailableQty menjumlahkan stok available per produk: di lokasi tertentu, atau (locationID nil) di semua
// lokasi yang bisa dijual (is_sellable).
func (s *Service) AvailableQty(ctx context.Context, productIDs []uuid.UUID, locationID *uuid.UUID) (map[uuid.UUID]decimal.Decimal, error) {
	byLoc, err := s.Store.StatusQty(ctx, uniq(productIDs), domain.StatusAvailable, locationID)
	if err != nil {
		return nil, err
	}
	var sellable map[uuid.UUID]warehouse.Location
	if locationID == nil {
		var lids []uuid.UUID
		for _, m := range byLoc {
			for lid := range m {
				lids = append(lids, lid)
			}
		}
		if len(lids) > 0 {
			if sellable, err = s.Locations.Locations(ctx, uniq(lids)); err != nil {
				return nil, err
			}
		}
	}
	out := map[uuid.UUID]decimal.Decimal{}
	for pid, m := range byLoc {
		for lid, q := range m {
			if locationID == nil {
				if l, ok := sellable[lid]; !ok || !l.IsSellable || l.Deleted() {
					continue
				}
			}
			out[pid] = out[pid].Add(q)
		}
	}
	return out, nil
}

// ComponentAvailability adalah stok tersedia satu komponen bundel.
type ComponentAvailability struct {
	ProductID    uuid.UUID
	QtyPerBundle decimal.Decimal
	AvailableQty decimal.Decimal
}

// BundleAvailability adalah stok tersedia bundel virtual = min(floor(stok komponen ÷ qty)) (S-115, MD-03).
type BundleAvailability struct {
	BundleID     uuid.UUID
	AvailableQty decimal.Decimal
	Components   []ComponentAvailability
}

// BundleAvailability menghitung stok tersedia bundel virtual dari stok available komponennya
// (catalog.AvailableBundleQty). ID yang bukan bundel dilewati. Tidak memeriksa permission.
func (s *Service) BundleAvailability(ctx context.Context, bundleIDs []uuid.UUID, locationID *uuid.UUID) ([]BundleAvailability, error) {
	if len(bundleIDs) == 0 {
		return nil, nil
	}
	prods, err := s.Products.Products(ctx, uniq(bundleIDs))
	if err != nil {
		return nil, err
	}
	var comps []uuid.UUID
	for _, p := range prods {
		for _, c := range p.Components {
			comps = append(comps, c.ComponentID)
		}
	}
	stock, err := s.AvailableQty(ctx, comps, locationID)
	if err != nil {
		return nil, err
	}
	var out []BundleAvailability
	for _, id := range bundleIDs {
		p, ok := prods[id]
		if !ok || !p.IsBundle {
			continue
		}
		ba := BundleAvailability{BundleID: id, AvailableQty: catalog.AvailableBundleQty(p.Components, stock)}
		for _, c := range p.Components {
			ba.Components = append(ba.Components, ComponentAvailability{ProductID: c.ComponentID, QtyPerBundle: c.Qty, AvailableQty: stock[c.ComponentID]})
		}
		out = append(out, ba)
	}
	return out, nil
}

// BundleAvailabilityForUser seperti BundleAvailability dengan cek permission inventory.stock.read.
func (s *Service) BundleAvailabilityForUser(ctx context.Context, bundleIDs []uuid.UUID, locationID *uuid.UUID) ([]BundleAvailability, error) {
	if _, err := actor.Require(ctx, PermStockRead); err != nil {
		return nil, err
	}
	if len(bundleIDs) > paging.MaxSize {
		return nil, apperr.Validation(map[string]string{"bundleId": "Maksimal 200 bundel per permintaan"})
	}
	return s.BundleAvailability(ctx, bundleIDs, locationID)
}
