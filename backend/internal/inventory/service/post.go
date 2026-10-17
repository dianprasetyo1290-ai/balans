package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/catalog"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/warehouse"
)

// ErrAlreadyPosted: aksi dokumen sumber (source type, id, action) sudah memutasi stok (errors.Is).
var ErrAlreadyPosted = postgres.ErrAlreadyPosted

// errLayerMismatch menandai cost layer batch lebih kecil dari stoknya (invariant ledger rusak).
var errLayerMismatch = errors.New("inventory: sisa cost layer lebih kecil dari stok batch")

// LineResult adalah hasil satu baris posting.
type LineResult struct {
	Index int
	// CostAmount positif: HPP FIFO barang keluar (issue), nilai barang yang dipindah (move), atau nilai
	// penerimaan (receive), dibulatkan 2 desimal.
	CostAmount decimal.Decimal
	// UnitCost adalah CostAmount ÷ qty (4 desimal).
	UnitCost  decimal.Decimal
	Movements []domain.Movement
}

// PostingResult adalah hasil posting: movement yang ditulis beserta biaya per baris.
type PostingResult struct {
	PostingID uuid.UUID
	Movements []domain.Movement
	Lines     []LineResult
}

// TotalCost adalah jumlah CostAmount semua baris.
func (r PostingResult) TotalCost() decimal.Decimal {
	total := decimal.Zero
	for _, l := range r.Lines {
		total = total.Add(l.CostAmount)
	}
	return total
}

// IssuedCostByLocation menjumlahkan HPP barang keluar (issue) per lokasi asal, mis. untuk A-06 yang mengkredit
// akun persediaan sesuai tipe lokasi.
func (r PostingResult) IssuedCostByLocation() map[uuid.UUID]decimal.Decimal {
	out := map[uuid.UUID]decimal.Decimal{}
	for _, m := range r.Movements {
		if m.Type == domain.MovementIssue {
			out[m.LocationID] = out[m.LocationID].Sub(m.CostAmount)
		}
	}
	return out
}

// part adalah satu potongan baris setelah alokasi batch: satu receive, satu issue, atau sepasang move.
type part struct {
	line        int
	kind        domain.LineKind
	out         *domain.Movement
	in          *domain.Movement
	reserveInto bool
	// release: pengambilan reservasi dihitung sebagai dilepas, bukan dipakai.
	release bool
}

func (pt part) qty() decimal.Decimal {
	if pt.in != nil {
		return pt.in.Qty
	}
	return pt.out.Qty.Neg()
}

// Post menjalankan satu posting stok di transaksi pemanggil (atau transaksi baru): validasi, alokasi batch
// FEFO bila batch kosong, kunci saldo berurutan, cek saldo tidak negatif, konsumsi cost layer FIFO, tulis
// ledger. Tidak memeriksa permission; modul pemanggil memeriksa permission dokumennya.
func (s *Service) Post(ctx context.Context, p domain.Posting) (PostingResult, error) {
	return s.post(ctx, p, false)
}

type masters struct {
	products  map[uuid.UUID]catalog.Product
	locations map[uuid.UUID]warehouse.Location
	batches   map[uuid.UUID]domain.Batch
}

func (m masters) productLabel(id uuid.UUID) string {
	if p, ok := m.products[id]; ok {
		return p.SKU
	}
	return id.String()
}

func (m masters) locationLabel(id uuid.UUID) string {
	if l, ok := m.locations[id]; ok {
		return l.Code
	}
	return id.String()
}

func lf(i int, f string) string { return fmt.Sprintf("lines[%d].%s", i, f) }

// loadMasters membaca dan memeriksa produk, lokasi, dan batch yang dirujuk posting.
func (s *Service) loadMasters(ctx context.Context, p domain.Posting) (masters, error) {
	var productIDs, locationIDs, batchIDs []uuid.UUID
	for _, l := range p.Lines {
		productIDs = append(productIDs, l.ProductID)
		for _, pl := range []*domain.Place{l.From, l.To} {
			if pl != nil {
				locationIDs = append(locationIDs, pl.LocationID)
			}
		}
		if l.BatchID != nil {
			batchIDs = append(batchIDs, *l.BatchID)
		}
	}
	var m masters
	var err error
	if m.products, err = s.Products.Products(ctx, uniq(productIDs)); err != nil {
		return masters{}, err
	}
	if m.locations, err = s.Locations.Locations(ctx, uniq(locationIDs)); err != nil {
		return masters{}, err
	}
	if m.batches, err = s.Store.BatchesByIDs(ctx, uniq(batchIDs)); err != nil {
		return masters{}, err
	}
	fields := map[string]string{}
	for i, l := range p.Lines {
		prod, ok := m.products[l.ProductID]
		switch {
		case !ok:
			fields[lf(i, "productId")] = "Produk tidak ditemukan"
			continue
		case !prod.HasStock():
			fields[lf(i, "productId")] = "Bundel virtual tidak punya stok; pecah menjadi komponen terlebih dahulu"
			continue
		}
		hasBatch := l.BatchID != nil || l.Batch != nil
		switch {
		case !prod.HasBatch && hasBatch:
			fields[lf(i, "batchId")] = "Produk " + prod.SKU + " tidak dilacak per batch"
		case prod.HasBatch && l.Kind == domain.KindReceive && !hasBatch:
			fields[lf(i, "batchId")] = "Batch wajib diisi untuk " + prod.SKU
		}
		if l.BatchID != nil {
			if b, ok := m.batches[*l.BatchID]; !ok || b.ProductID != l.ProductID {
				fields[lf(i, "batchId")] = "Batch tidak ditemukan untuk produk " + prod.SKU
			}
		}
		if l.From != nil {
			if _, ok := m.locations[l.From.LocationID]; !ok {
				fields[lf(i, "from.locationId")] = "Lokasi asal tidak ditemukan"
			}
		}
		if l.To != nil {
			loc, ok := m.locations[l.To.LocationID]
			switch {
			case !ok:
				fields[lf(i, "to.locationId")] = "Lokasi tujuan tidak ditemukan"
			case !loc.IsActive || loc.Deleted():
				fields[lf(i, "to.locationId")] = "Lokasi tujuan " + loc.Code + " nonaktif atau dihapus"
			}
		}
	}
	if len(fields) > 0 {
		return masters{}, apperr.Validation(fields)
	}
	return m, nil
}

func uniq(ids []uuid.UUID) []uuid.UUID {
	out := slices.Clone(ids)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(out)
}

func (s *Service) post(ctx context.Context, p domain.Posting, release bool) (PostingResult, error) {
	p, err := p.Validate()
	if err != nil {
		return PostingResult{}, err
	}
	m, err := s.loadMasters(ctx, p)
	if err != nil {
		return PostingResult{}, err
	}
	if s.Periods != nil {
		if err := s.Periods.CheckPeriodOpen(ctx, p.Source.BusinessDate); err != nil {
			return PostingResult{}, err
		}
	}
	actorID := actor.UserID(ctx)
	var res PostingResult
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		postingID, err := s.Store.InsertPosting(ctx, p.Source, actorID)
		if err != nil {
			return err
		}
		pl := &planner{s: s, p: p, m: m, actorID: actorID, used: map[string]decimal.Decimal{}, resUsed: map[uuid.UUID]decimal.Decimal{}}
		parts, err := pl.plan(ctx, release)
		if err != nil {
			return err
		}
		if err := s.applyBalances(ctx, parts, m); err != nil {
			return err
		}
		if err := s.applyReservations(ctx, parts, m, actorID); err != nil {
			return err
		}
		res, err = s.persist(ctx, postingID, p, parts, actorID)
		return err
	})
	if err != nil {
		return PostingResult{}, err
	}
	return res, nil
}

// applyBalances mengunci saldo dalam urutan (product, location, batch, status) lalu menolak saldo negatif.
func (s *Service) applyBalances(ctx context.Context, parts []part, m masters) error {
	var all []domain.Movement
	for _, pt := range parts {
		for _, mv := range []*domain.Movement{pt.out, pt.in} {
			if mv != nil {
				all = append(all, *mv)
			}
		}
	}
	for _, d := range domain.NetDeltas(all) {
		lb, err := s.Store.LockBalance(ctx, d.Key)
		if err != nil {
			return err
		}
		next := lb.Qty.Add(d.Delta)
		if next.IsNegative() {
			return domain.InsufficientStock(m.productLabel(d.Key.ProductID), m.locationLabel(d.Key.LocationID), d.Key.Status, lb.Qty)
		}
		if !d.Delta.IsZero() {
			if err := s.Store.SetBalanceQty(ctx, lb.ID, next); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyReservations mengunci reservasi yang diambil (urut id), memeriksa qty terbuka, dan mencatat pemakaian.
func (s *Service) applyReservations(ctx context.Context, parts []part, m masters, actorID uuid.UUID) error {
	draws := map[uuid.UUID]decimal.Decimal{}
	released := map[uuid.UUID]bool{}
	labels := map[uuid.UUID]domain.Movement{}
	for _, pt := range parts {
		if pt.out == nil || pt.out.ReservationID == nil {
			continue
		}
		id := *pt.out.ReservationID
		draws[id] = draws[id].Add(pt.qty())
		released[id] = pt.release
		labels[id] = *pt.out
	}
	if len(draws) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(draws))
	for id := range draws {
		ids = append(ids, id)
	}
	open, err := s.Store.LockReservations(ctx, uniq(ids))
	if err != nil {
		return err
	}
	for _, id := range uniq(ids) {
		if open[id].LessThan(draws[id]) {
			mv := labels[id]
			return domain.InsufficientStock(m.productLabel(mv.ProductID), m.locationLabel(mv.LocationID), domain.StatusReserved, open[id])
		}
		consumed, rel := draws[id], decimal.Zero
		if released[id] {
			consumed, rel = decimal.Zero, draws[id]
		}
		if err := s.Store.AddReservationUsage(ctx, id, consumed, rel, actorID); err != nil {
			return err
		}
	}
	return nil
}

type layerGroup struct {
	productID uuid.UUID
	batchID   *uuid.UUID
}

func (g layerGroup) key() string { return g.productID.String() + "/" + batchString(g.batchID) }

func batchString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// persist mengunci cost layer per (produk, batch) berurutan, menghitung HPP FIFO, lalu menulis movement,
// cost layer, jejak konsumsi, dan reservasi baru.
func (s *Service) persist(ctx context.Context, postingID uuid.UUID, p domain.Posting, parts []part, actorID uuid.UUID) (PostingResult, error) {
	groups := map[string]layerGroup{}
	for _, pt := range parts {
		mv := pt.out
		if mv == nil {
			mv = pt.in
		}
		g := layerGroup{productID: mv.ProductID, batchID: mv.BatchID}
		groups[g.key()] = g
	}
	ordered := make([]layerGroup, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	slices.SortFunc(ordered, func(a, b layerGroup) int {
		ka := domain.BalanceKey{ProductID: a.productID, BatchID: a.batchID}
		kb := domain.BalanceKey{ProductID: b.productID, BatchID: b.batchID}
		return ka.Compare(kb)
	})
	sets := map[string]*domain.LayerSet{}
	before := map[uuid.UUID]decimal.Decimal{}
	for _, g := range ordered {
		set, err := s.Store.LockLayers(ctx, g.productID, g.batchID)
		if err != nil {
			return PostingResult{}, err
		}
		for _, l := range set.Layers {
			before[l.ID] = l.Remaining
		}
		sets[g.key()] = &set
	}

	res := PostingResult{PostingID: postingID, Lines: make([]LineResult, len(p.Lines))}
	for i := range res.Lines {
		res.Lines[i] = LineResult{Index: i, CostAmount: decimal.Zero}
	}
	lineNo := 0
	write := func(mv *domain.Movement, line domain.Line) error {
		lineNo++
		id, err := s.Store.InsertMovement(ctx, postgres.MovementRow{
			PostingID: postingID, LineNo: lineNo, Source: p.Source, SourceLineID: line.SourceLineID, Movement: *mv,
		}, actorID)
		if err != nil {
			return err
		}
		mv.ID = id
		for _, c := range mv.Consumptions {
			if err := s.Store.InsertConsumption(ctx, id, c, actorID); err != nil {
				return err
			}
		}
		return nil
	}
	for _, pt := range parts {
		line := p.Lines[pt.line]
		lr := &res.Lines[pt.line]
		switch pt.kind {
		case domain.KindReceive:
			mv := pt.in
			if err := write(mv, line); err != nil {
				return PostingResult{}, err
			}
			layerID, err := s.Store.InsertLayer(ctx, postgres.NewLayer{
				ProductID: mv.ProductID, BatchID: mv.BatchID, Source: p.Source, MovementID: mv.ID, Qty: mv.Qty, UnitCost: mv.UnitCost,
			})
			if err != nil {
				return PostingResult{}, err
			}
			sets[layerGroup{productID: mv.ProductID, batchID: mv.BatchID}.key()].Add(domain.Layer{ID: layerID, Remaining: mv.Qty, UnitCost: mv.UnitCost})
			before[layerID] = mv.Qty
			lr.CostAmount = lr.CostAmount.Add(mv.CostAmount)
			lr.Movements = append(lr.Movements, *mv)
		case domain.KindIssue:
			mv := pt.out
			set := sets[layerGroup{productID: mv.ProductID, batchID: mv.BatchID}.key()]
			cons, ok := set.Consume(pt.qty())
			if !ok {
				return PostingResult{}, fmt.Errorf("issue %s: %w", mv.Key(), errLayerMismatch)
			}
			mv.Consumptions = cons
			mv.UnitCost = domain.AverageUnitCost(cons)
			mv.CostAmount = domain.CostAmount(cons).Neg()
			if err := write(mv, line); err != nil {
				return PostingResult{}, err
			}
			lr.CostAmount = lr.CostAmount.Sub(mv.CostAmount)
			lr.Movements = append(lr.Movements, *mv)
		case domain.KindMove:
			set := sets[layerGroup{productID: pt.out.ProductID, batchID: pt.out.BatchID}.key()]
			cons, ok := set.Peek(pt.qty())
			if !ok {
				return PostingResult{}, fmt.Errorf("move %s: %w", pt.out.Key(), errLayerMismatch)
			}
			amount, unit := domain.CostAmount(cons), domain.AverageUnitCost(cons)
			pt.out.UnitCost, pt.out.CostAmount = unit, amount.Neg()
			pt.in.UnitCost, pt.in.CostAmount = unit, amount
			if pt.reserveInto {
				resID, err := s.Store.InsertReservation(ctx, postgres.NewReservation{
					Source: p.Source, SourceLineID: line.SourceLineID, Key: pt.in.Key(), Qty: pt.in.Qty, ExpiresAt: p.ReservationExpiresAt,
				}, actorID)
				if err != nil {
					return PostingResult{}, err
				}
				pt.in.ReservationID = &resID
			}
			if err := write(pt.out, line); err != nil {
				return PostingResult{}, err
			}
			if err := write(pt.in, line); err != nil {
				return PostingResult{}, err
			}
			lr.CostAmount = lr.CostAmount.Add(amount)
			lr.Movements = append(lr.Movements, *pt.out, *pt.in)
		}
	}
	for _, set := range sets {
		for _, l := range set.Changed(before) {
			if err := s.Store.SetLayerRemaining(ctx, l.ID, l.Remaining); err != nil {
				return PostingResult{}, err
			}
		}
	}
	for i := range res.Lines {
		lr := &res.Lines[i]
		if q := p.Lines[i].Qty; q.IsPositive() {
			lr.UnitCost = lr.CostAmount.Div(q).Round(domain.UnitCostScale)
		}
		res.Movements = append(res.Movements, lr.Movements...)
	}
	return res, nil
}

// --- Perencanaan alokasi ----------------------------------------------------------------------

type planner struct {
	s       *Service
	p       domain.Posting
	m       masters
	actorID uuid.UUID
	// used adalah qty yang sudah direncanakan diambil per kunci saldo di posting ini.
	used map[string]decimal.Decimal
	// resUsed adalah qty yang sudah direncanakan diambil per reservasi.
	resUsed map[uuid.UUID]decimal.Decimal
}

func (pl *planner) plan(ctx context.Context, release bool) ([]part, error) {
	var parts []part
	for i, l := range pl.p.Lines {
		var lineParts []part
		var err error
		if l.Kind == domain.KindReceive {
			lineParts, err = pl.planReceive(ctx, i, l)
		} else {
			lineParts, err = pl.planOutgoing(ctx, i, l, release)
		}
		if err != nil {
			return nil, err
		}
		parts = append(parts, lineParts...)
	}
	return parts, nil
}

func (pl *planner) planReceive(ctx context.Context, i int, l domain.Line) ([]part, error) {
	prod := pl.m.products[l.ProductID]
	batchID := l.BatchID
	if l.Batch != nil {
		nb, err := domain.ResolveNewBatch(l.ProductID, *l.Batch, prod.ShelfLifeDays, lf(i, "batch"))
		if err != nil {
			return nil, err
		}
		saved, created, err := pl.s.Store.EnsureBatch(ctx, nb, pl.actorID)
		if err != nil {
			return nil, err
		}
		if err := checkSavedBatch(saved, created, nb, prod.ShelfLifeDays, lf(i, "batch")); err != nil {
			return nil, err
		}
		batchID = &saved.ID
	}
	var unitCost decimal.Decimal
	if l.UnitCost != nil {
		unitCost = *l.UnitCost
	} else {
		c, ok, err := pl.s.Store.LatestUnitCost(ctx, l.ProductID, batchID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperr.Validation(map[string]string{lf(i, "unitCost"): "HPP per unit wajib diisi karena batch belum punya cost layer"})
		}
		unitCost = c
	}
	mv := &domain.Movement{
		LineIndex: i, Type: domain.MovementReceive, ProductID: l.ProductID, LocationID: l.To.LocationID, BatchID: batchID,
		Status: l.To.Status, Qty: l.Qty, UnitCost: unitCost, CostAmount: domain.LineAmount(l.Qty, unitCost),
		BatchReason: l.BatchReason, Note: l.Note,
	}
	return []part{{line: i, kind: domain.KindReceive, in: mv}}, nil
}

func usedKey(productID, locationID uuid.UUID, batchID *uuid.UUID, status domain.StockStatus) string {
	return domain.BalanceKey{ProductID: productID, LocationID: locationID, BatchID: batchID, Status: status}.String()
}

// balanceCandidates membaca stok per batch dikurangi yang sudah direncanakan baris sebelumnya.
func (pl *planner) balanceCandidates(ctx context.Context, productID uuid.UUID, from domain.Place) ([]domain.Candidate, error) {
	cands, err := pl.s.Store.BalanceCandidates(ctx, productID, from.LocationID, from.Status)
	if err != nil {
		return nil, err
	}
	for i := range cands {
		cands[i].Qty = cands[i].Qty.Sub(pl.used[usedKey(productID, from.LocationID, cands[i].BatchID, from.Status)])
	}
	return cands, nil
}

// reservationCandidates membaca reservasi terbuka dokumen rujukan dikurangi yang sudah direncanakan.
func (pl *planner) reservationCandidates(ctx context.Context, l domain.Line) ([]domain.Candidate, error) {
	ref := l.Reservation
	rows, err := pl.s.Store.OpenReservations(ctx, postgres.ReservationQuery{
		SourceType: ref.SourceType, SourceID: ref.SourceID, SourceLineID: ref.SourceLineID,
		ProductID: &l.ProductID, LocationID: &l.From.LocationID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Candidate, 0, len(rows))
	for _, r := range rows {
		if ref.ReservationID != nil && r.ID != *ref.ReservationID {
			continue
		}
		if l.BatchID != nil && !domain.SameBatch(r.Candidate.BatchID, l.BatchID) {
			continue
		}
		c := r.Candidate
		c.Qty = c.Qty.Sub(pl.resUsed[r.ID])
		if c.Qty.IsPositive() {
			out = append(out, c)
		}
	}
	domain.SortFEFO(out)
	return out, nil
}

func sumQty(cands []domain.Candidate) decimal.Decimal {
	total := decimal.Zero
	for _, c := range cands {
		if c.Qty.IsPositive() {
			total = total.Add(c.Qty)
		}
	}
	return total
}

func (pl *planner) planOutgoing(ctx context.Context, i int, l domain.Line, release bool) ([]part, error) {
	prod := pl.m.products[l.ProductID]
	today := pl.p.Source.BusinessDate
	var allocs []domain.Allocation
	switch {
	case l.From.Status == domain.StatusReserved:
		cands, err := pl.reservationCandidates(ctx, l)
		if err != nil {
			return nil, err
		}
		remaining := l.Qty
		for _, c := range cands {
			if !remaining.IsPositive() {
				break
			}
			take := decimal.Min(c.Qty, remaining)
			allocs = append(allocs, domain.Allocation{Candidate: c, Qty: take})
			remaining = remaining.Sub(take)
		}
		if remaining.IsPositive() {
			return nil, domain.InsufficientStock(prod.SKU, pl.m.locationLabel(l.From.LocationID), domain.StatusReserved, sumQty(cands))
		}
	case prod.HasBatch && l.BatchID == nil:
		cands, err := pl.balanceCandidates(ctx, l.ProductID, *l.From)
		if err != nil {
			return nil, err
		}
		var shortfall decimal.Decimal
		allocs, shortfall = pl.s.Allocator.Allocate(cands, l.Qty, today)
		if shortfall.IsPositive() {
			return nil, domain.InsufficientStock(prod.SKU, pl.m.locationLabel(l.From.LocationID), l.From.Status, sumQty(domain.Eligible(cands, today)))
		}
	default:
		if prod.HasBatch && l.From.Status == domain.StatusAvailable && l.BatchReason == "" {
			cands, err := pl.balanceCandidates(ctx, l.ProductID, *l.From)
			if err != nil {
				return nil, err
			}
			if !domain.IsFEFOChoice(pl.s.Allocator, cands, l.BatchID, l.Qty, today) {
				msg := "Batch yang dipilih bukan saran FEFO"
				if el := domain.Eligible(cands, today); len(el) > 0 {
					msg += " (saran: " + el[0].BatchNumber + ")"
				}
				return nil, apperr.Validation(map[string]string{lf(i, "batchReason"): msg + ". Isi alasan pemilihan batch manual"})
			}
		}
		allocs = []domain.Allocation{{Candidate: domain.Candidate{BatchID: l.BatchID}, Qty: l.Qty}}
	}

	parts := make([]part, 0, len(allocs))
	for _, a := range allocs {
		batchID := a.Candidate.BatchID
		out := &domain.Movement{
			LineIndex: i, Type: domain.MovementIssue, ProductID: l.ProductID, LocationID: l.From.LocationID, BatchID: batchID,
			Status: l.From.Status, Qty: a.Qty.Neg(), ReservationID: a.Candidate.ReservationID, BatchReason: l.BatchReason, Note: l.Note,
		}
		pl.used[usedKey(l.ProductID, l.From.LocationID, batchID, l.From.Status)] =
			pl.used[usedKey(l.ProductID, l.From.LocationID, batchID, l.From.Status)].Add(a.Qty)
		if a.Candidate.ReservationID != nil {
			pl.resUsed[*a.Candidate.ReservationID] = pl.resUsed[*a.Candidate.ReservationID].Add(a.Qty)
		}
		pt := part{line: i, kind: l.Kind, out: out, release: release}
		if l.Kind == domain.KindMove {
			out.Type = domain.MovementMoveOut
			pt.in = &domain.Movement{
				LineIndex: i, Type: domain.MovementMoveIn, ProductID: l.ProductID, LocationID: l.To.LocationID, BatchID: batchID,
				Status: l.To.Status, Qty: a.Qty, BatchReason: l.BatchReason, Note: l.Note,
			}
			pt.reserveInto = l.IntoReserved()
		}
		parts = append(parts, pt)
	}
	return parts, nil
}

// --- Reservasi --------------------------------------------------------------------------------

// ReserveLine adalah satu baris reservasi stok available → reserved di lokasi yang sama.
type ReserveLine struct {
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	LocationID   uuid.UUID
	// BatchID nil = dialokasikan FEFO.
	BatchID     *uuid.UUID
	Qty         decimal.Decimal
	BatchReason string
}

// ReserveRequest adalah permintaan reservasi stok untuk satu dokumen (TG 5.7.5).
type ReserveRequest struct {
	// Source.Action kosong = "reserve".
	Source    domain.Source
	ExpiresAt *time.Time
	Lines     []ReserveLine
}

// ActionReserve dan ActionRelease adalah aksi default reservasi.
const (
	ActionReserve = "reserve"
	ActionRelease = "release"
)

// Reserve memindah stok available → reserved (FEFO bila batch kosong) dan mencatat reservasi per baris.
func (s *Service) Reserve(ctx context.Context, r ReserveRequest) (PostingResult, error) {
	if r.Source.Action == "" {
		r.Source.Action = ActionReserve
	}
	lines := make([]domain.Line, len(r.Lines))
	for i, l := range r.Lines {
		lines[i] = domain.Line{
			Kind: domain.KindMove, SourceLineID: l.SourceLineID, ProductID: l.ProductID, Qty: l.Qty, BatchID: l.BatchID,
			From:        &domain.Place{LocationID: l.LocationID, Status: domain.StatusAvailable},
			To:          &domain.Place{LocationID: l.LocationID, Status: domain.StatusReserved},
			BatchReason: l.BatchReason,
		}
	}
	return s.Post(ctx, domain.Posting{Source: r.Source, Lines: lines, ReservationExpiresAt: r.ExpiresAt})
}

// ReleaseRequest melepas seluruh qty terbuka reservasi dokumen kembali ke available.
type ReleaseRequest struct {
	// Source mencatat aksi pelepasan (Action kosong = "release").
	Source      domain.Source
	Reservation domain.ReservationRef
	// ExpiredAt: hanya reservasi dengan expires_at <= nilai ini (job kedaluwarsa).
	ExpiredAt *time.Time
}

// Release melepas reservasi terbuka; tanpa reservasi terbuka mengembalikan hasil kosong.
func (s *Service) Release(ctx context.Context, r ReleaseRequest) (PostingResult, error) {
	if r.Source.Action == "" {
		r.Source.Action = ActionRelease
	}
	ref := r.Reservation
	if ref.SourceID == uuid.Nil || ref.SourceType == "" {
		return PostingResult{}, apperr.Validation(map[string]string{"reservation": "Referensi reservasi wajib diisi"})
	}
	open, err := s.Store.OpenReservations(ctx, postgres.ReservationQuery{
		SourceType: ref.SourceType, SourceID: ref.SourceID, SourceLineID: ref.SourceLineID,
	})
	if err != nil {
		return PostingResult{}, err
	}
	var lines []domain.Line
	for _, o := range open {
		if ref.ReservationID != nil && o.ID != *ref.ReservationID {
			continue
		}
		if r.ExpiredAt != nil && (o.ExpiresAt == nil || o.ExpiresAt.After(*r.ExpiredAt)) {
			continue
		}
		id := o.ID
		lines = append(lines, domain.Line{
			Kind: domain.KindMove, SourceLineID: o.SourceLineID, ProductID: o.ProductID, Qty: o.Candidate.Qty,
			BatchID:     o.Candidate.BatchID,
			From:        &domain.Place{LocationID: o.LocationID, Status: domain.StatusReserved},
			To:          &domain.Place{LocationID: o.LocationID, Status: domain.StatusAvailable},
			Reservation: &domain.ReservationRef{SourceType: ref.SourceType, SourceID: ref.SourceID, ReservationID: &id},
		})
	}
	if len(lines) == 0 {
		return PostingResult{}, nil
	}
	return s.post(ctx, domain.Posting{Source: r.Source, Lines: lines}, true)
}

// ReleaseExpired melepas reservasi yang melewati expires_at (job stock_reservation.expire). Mengembalikan
// jumlah dokumen yang dilepas; kegagalan satu dokumen dicatat di log dan tidak menghentikan yang lain.
func (s *Service) ReleaseExpired(ctx context.Context) (int, error) {
	now := s.Clock.Now()
	sources, err := s.Store.ExpiredReservationSources(ctx, now, 100)
	if err != nil {
		return 0, err
	}
	ctx = actor.System(ctx)
	n := 0
	var firstErr error
	for _, src := range sources {
		_, err := s.Release(ctx, ReleaseRequest{
			Source: domain.Source{
				Type: src.SourceType, ID: src.SourceID, Action: "expire:" + now.UTC().Format("20060102T150405.000000"),
				Label: "Reservasi kedaluwarsa", BusinessDate: clock.BusinessDateOf(now),
			},
			Reservation: domain.ReservationRef{SourceType: src.SourceType, SourceID: src.SourceID},
			ExpiredAt:   &now,
		})
		if err != nil {
			if s.Log != nil {
				s.Log.WarnContext(ctx, "release expired reservation", "source_type", src.SourceType, "source_id", src.SourceID, "error", err)
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n++
	}
	return n, firstErr
}

// --- Batch ------------------------------------------------------------------------------------

// BatchInput adalah batch yang dicari/dibuat modul lain sebelum posting (mis. form Penerimaan Barang).
type BatchInput struct {
	ProductID      uuid.UUID
	Number         string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
}

// EnsureBatch mencari batch (produk + nomor) atau membuatnya; tanggal yang bertentangan ditolak.
func (s *Service) EnsureBatch(ctx context.Context, in BatchInput) (domain.Batch, error) {
	prods, err := s.Products.Products(ctx, []uuid.UUID{in.ProductID})
	if err != nil {
		return domain.Batch{}, err
	}
	prod, ok := prods[in.ProductID]
	switch {
	case !ok:
		return domain.Batch{}, apperr.Validation(map[string]string{"productId": "Produk tidak ditemukan"})
	case !prod.HasBatch || !prod.HasStock():
		return domain.Batch{}, apperr.Validation(map[string]string{"productId": "Produk " + prod.SKU + " tidak dilacak per batch"})
	}
	nb, err := domain.ResolveNewBatch(in.ProductID, domain.NewBatch{Number: in.Number, ProductionDate: in.ProductionDate, ExpiryDate: in.ExpiryDate}, prod.ShelfLifeDays, "batch")
	if err != nil {
		return domain.Batch{}, err
	}
	var out domain.Batch
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		saved, created, err := s.Store.EnsureBatch(ctx, nb, actor.UserID(ctx))
		if err != nil {
			return err
		}
		if err := checkSavedBatch(saved, created, nb, prod.ShelfLifeDays, "batch"); err != nil {
			return err
		}
		out = saved
		return nil
	})
	return out, err
}

func checkSavedBatch(saved domain.Batch, created bool, in domain.Batch, shelfLifeDays *int, field string) error {
	if created {
		return domain.RequireExpiry(saved, shelfLifeDays, field)
	}
	return saved.Matches(in, field)
}

// Batch membaca batch berdasarkan ID.
func (s *Service) Batch(ctx context.Context, id uuid.UUID) (domain.Batch, error) {
	return s.Store.GetBatch(ctx, id)
}
