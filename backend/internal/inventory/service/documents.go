package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/shopspring/decimal"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

// Approvals mengajukan dokumen ke engine approval (dipenuhi *approval.Module).
type Approvals interface {
	Request(ctx context.Context, s approval.Subject) (approval.Request, error)
}

// Poster memposting jurnal otomatis (dipenuhi *accounting.Module).
type Poster interface {
	Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error)
}

// Numberer mengambil nomor dokumen di transaksi aktif (dipenuhi *document.Module).
type Numberer interface {
	Next(ctx context.Context, t document.Type, businessDate clock.BusinessDate) (string, error)
}

// DimensionOption adalah divisi atau kampanye untuk dropdown dan tampilan dokumen.
type DimensionOption struct {
	ID        uuid.UUID
	Code      string
	Name      string
	IsActive  bool
	ValidFrom *clock.BusinessDate
	ValidTo   *clock.BusinessDate
}

// Jenis dimensi yang dipakai dokumen persediaan.
const (
	DimensionDivision = "division"
	DimensionCampaign = "campaign"
)

// DimensionDirectory membaca master divisi & kampanye modul accounting (adapter di app).
type DimensionDirectory interface {
	DimensionOptions(ctx context.Context, kind string, includeInactive bool) ([]DimensionOption, error)
}

// ReturnReason adalah master alasan retur (modul sales) beserta usulan disposisinya.
type ReturnReason struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	DefaultDisposition domain.Disposition
	// Usable nil bila alasan bisa dipakai dokumen baru.
	Usable error
}

// ReturnReasonDirectory membaca alasan retur (adapter di app ke *sales.Module).
type ReturnReasonDirectory interface {
	ReturnReason(ctx context.Context, id uuid.UUID) (ReturnReason, error)
}

// FileAttacher menautkan foto berita acara ke dokumen (dipenuhi *filestore.Module).
type FileAttacher interface {
	Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error
	Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Notifier mengirim notifikasi in-app (dipenuhi *notification.Module).
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
}

// RoleMembers membaca user aktif satu role (adapter di app ke modul identity).
type RoleMembers interface {
	UserIDsWithRole(ctx context.Context, role string) ([]uuid.UUID, error)
}

// JobEnqueuer memasukkan job di transaksi aktif (dipenuhi *jobqueue.Inserter).
type JobEnqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
}

// documentStore adalah repository dokumen persediaan dan recost.
type documentStore interface {
	InsertUsage(ctx context.Context, u domain.InternalUsage) (domain.InternalUsage, error)
	ReplaceUsageLines(ctx context.Context, u *domain.InternalUsage) error
	GetUsage(ctx context.Context, id uuid.UUID, lock bool) (domain.InternalUsage, error)
	SaveUsage(ctx context.Context, u *domain.InternalUsage, actorID uuid.UUID) error
	ListUsages(ctx context.Context, f postgres.DocFilter, page paging.Page) (paging.Result[domain.InternalUsage], error)
	SettlementDueUsages(ctx context.Context, today clock.BusinessDate) ([]uuid.UUID, error)

	InsertRepacking(ctx context.Context, r domain.Repacking) (domain.Repacking, error)
	ReplaceRepackLines(ctx context.Context, r *domain.Repacking) error
	GetRepacking(ctx context.Context, id uuid.UUID, lock bool) (domain.Repacking, error)
	SaveRepacking(ctx context.Context, r *domain.Repacking, actorID uuid.UUID) error
	ListRepackings(ctx context.Context, f postgres.DocFilter, page paging.Page) (paging.Result[domain.Repacking], error)

	InsertDisposal(ctx context.Context, d domain.Disposal) (domain.Disposal, error)
	ReplaceDisposalLines(ctx context.Context, d *domain.Disposal) error
	GetDisposal(ctx context.Context, id uuid.UUID, lock bool) (domain.Disposal, error)
	SaveDisposal(ctx context.Context, d *domain.Disposal, actorID uuid.UUID) error
	ListDisposals(ctx context.Context, f postgres.DocFilter, page paging.Page) (paging.Result[domain.Disposal], error)

	InsertDisposition(ctx context.Context, d domain.ReturnDisposition) (domain.ReturnDisposition, error)
	ReplaceDispositionLines(ctx context.Context, d *domain.ReturnDisposition) error
	GetDisposition(ctx context.Context, id uuid.UUID, lock bool) (domain.ReturnDisposition, error)
	SaveDisposition(ctx context.Context, d *domain.ReturnDisposition, actorID uuid.UUID) error
	ListDispositions(ctx context.Context, f postgres.DocFilter, page paging.Page) (paging.Result[domain.ReturnDisposition], error)

	SourceMovements(ctx context.Context, sourceType string, sourceID uuid.UUID, action string) ([]postgres.SourceMovement, error)
	OpenLayers(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) (domain.LayerSet, error)

	HasIssuesSince(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID, from clock.BusinessDate) (bool, error)
	InsertRecostRun(ctx context.Context, r postgres.RecostRun, actorID uuid.UUID) (uuid.UUID, error)
	GetRecostRun(ctx context.Context, id uuid.UUID, lock bool) (postgres.RecostRun, error)
	FinishRecostRun(ctx context.Context, id uuid.UUID, status string, amount decimal.Decimal, journalID *uuid.UUID, errText string) error
	ListRecostRuns(ctx context.Context, productID *uuid.UUID, page paging.Page) (paging.Result[postgres.RecostRun], error)
	LockAllLayers(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) ([]domain.ReplayLayer, error)
	BatchIssues(ctx context.Context, productID uuid.UUID, batchID *uuid.UUID) ([]postgres.BatchIssue, error)
	InsertRecostLine(ctx context.Context, l postgres.RecostLine) error
	RecostLines(ctx context.Context, runID uuid.UUID) ([]postgres.RecostLine, error)
}

var _ documentStore = (*postgres.Store)(nil)

// Jenis dokumen sumber mutasi stok, jurnal, dan audit trail.
const (
	SourceInternalUsage     = "internal_usage"
	SourceRepacking         = "repacking"
	SourceDisposal          = "disposal"
	SourceReturnDisposition = "return_disposition"
	SourceRecost            = "recost_run"
)

// FileOwnerDisposal adalah jenis pemilik foto berita acara pemusnahan di filestore.
const FileOwnerDisposal = "disposal"

// Kode divisi master dimensi (Desain Finance & Accounting 3.2).
const (
	DivisionLogistics = "LOG"
	DivisionMarketing = "MKT"
)

// Aksi posting stok dokumen (Source.Action).
const (
	stockReserve  = "reserve"
	stockIssue    = "issue"
	stockDispatch = "dispatch"
	stockSettle   = "settle"
	stockExecute  = "post"
	stockRelease  = "release"
)

// errDependency menandai dependensi modul yang belum diwiring (konfigurasi ENABLED_MODULES).
var errDependency = errors.New("inventory: dependensi modul tidak aktif")

func requireDep(ok bool, name string) error {
	if !ok {
		return apperr.Wrap(apperr.CodeProviderUnavailable, fmt.Errorf("%w: %s", errDependency, name))
	}
	return nil
}

func docActor(a actor.Actor) domain.Actor { return domain.Actor{ID: a.UserID, Name: a.FullName} }

func (s *Service) record(ctx context.Context, e audit.Entry) error {
	if s.Audit == nil {
		return nil
	}
	return s.Audit.Record(ctx, e)
}

func (s *Service) number(ctx context.Context, t document.Type, d clock.BusinessDate) (string, error) {
	if err := requireDep(s.Numberer != nil, "document"); err != nil {
		return "", err
	}
	return s.Numberer.Next(ctx, t, d)
}

func (s *Service) postJournal(ctx context.Context, req accounting.PostingRequest) (*uuid.UUID, error) {
	if err := requireDep(s.Poster != nil, "accounting"); err != nil {
		return nil, err
	}
	e, err := s.Poster.Post(ctx, req)
	if err != nil {
		return nil, err
	}
	return &e.ID, nil
}

func (s *Service) requestApproval(ctx context.Context, subj approval.Subject) error {
	if err := requireDep(s.Approvals != nil, "approval"); err != nil {
		return err
	}
	_, err := s.Approvals.Request(ctx, subj)
	return err
}

// divisionByCode mencari ID divisi aktif dari kodenya (LOG untuk jurnal persediaan, MKT untuk sampel retur).
func (s *Service) divisionByCode(ctx context.Context, code string) (uuid.UUID, error) {
	if err := requireDep(s.Dimensions != nil, "accounting"); err != nil {
		return uuid.Nil, err
	}
	opts, err := s.Dimensions.DimensionOptions(ctx, DimensionDivision, false)
	if err != nil {
		return uuid.Nil, err
	}
	for _, o := range opts {
		if o.Code == code {
			return o.ID, nil
		}
	}
	return uuid.Nil, apperr.Validation(map[string]string{"divisionId": "Divisi " + code + " tidak ditemukan atau nonaktif"})
}

func amountOf(d decimal.Decimal) money.Amount { return money.FromDecimal(d).Round() }

// locationsOf membaca lokasi dan menolak lokasi yang tidak ditemukan.
func (s *Service) locationsOf(ctx context.Context, ids ...uuid.UUID) (map[uuid.UUID]warehouse.Location, error) {
	return s.Locations.Locations(ctx, uniq(ids))
}

func (s *Service) productsOf(ctx context.Context, ids ...uuid.UUID) (map[uuid.UUID]catalog.Product, error) {
	return s.Products.Products(ctx, uniq(ids))
}

// siblingLocation mencari lokasi standar berjenis t di gudang yang sama dengan lokasi asal.
func (s *Service) siblingLocation(ctx context.Context, of warehouse.Location, t warehouse.LocationType, field string) (warehouse.Location, error) {
	if of.WarehouseID == nil {
		return warehouse.Location{}, apperr.Validation(map[string]string{field: "Lokasi " + of.Code + " tidak berada di gudang"})
	}
	locs, err := s.Locations.FindLocations(ctx, warehouse.LocationQuery{WarehouseID: of.WarehouseID, Types: []warehouse.LocationType{t}})
	if err != nil {
		return warehouse.Location{}, err
	}
	var pick *warehouse.Location
	for i := range locs {
		l := locs[i]
		if !l.IsActive || l.Deleted() {
			continue
		}
		if pick == nil || (l.IsStandard && !pick.IsStandard) {
			pick = &l
		}
	}
	if pick == nil {
		return warehouse.Location{}, apperr.Validation(map[string]string{
			field: fmt.Sprintf("Gudang lokasi %s tidak punya lokasi %s aktif", of.Code, t.Label()),
		})
	}
	return *pick, nil
}

// requireLocation memeriksa lokasi ada, aktif, dan berjenis salah satu types.
func requireLocation(locs map[uuid.UUID]warehouse.Location, id uuid.UUID, field string, types ...warehouse.LocationType) (warehouse.Location, error) {
	l, ok := locs[id]
	switch {
	case !ok:
		return warehouse.Location{}, apperr.Validation(map[string]string{field: "Lokasi tidak ditemukan"})
	case !l.IsActive || l.Deleted():
		return warehouse.Location{}, apperr.Validation(map[string]string{field: "Lokasi " + l.Code + " nonaktif atau dihapus"})
	}
	for _, t := range types {
		if l.Type == t {
			return l, nil
		}
	}
	labels := ""
	for i, t := range types {
		if i > 0 {
			labels += "/"
		}
		labels += t.Label()
	}
	return warehouse.Location{}, apperr.Validation(map[string]string{field: "Lokasi " + l.Code + " harus berjenis " + labels})
}

// requireProducts memeriksa produk ada, aktif, punya stok, dan berjenis salah satu types (kosong = semua).
func requireProducts(prods map[uuid.UUID]catalog.Product, ids []uuid.UUID, field func(i int) string, types ...catalog.ProductType) error {
	fields := map[string]string{}
	for i, id := range ids {
		p, ok := prods[id]
		switch {
		case !ok:
			fields[field(i)] = "Produk tidak ditemukan"
		case !p.HasStock():
			fields[field(i)] = "Bundel virtual tidak punya stok"
		case len(types) > 0 && !containsType(types, p.Type):
			fields[field(i)] = "Jenis produk " + p.SKU + " tidak sesuai dokumen ini"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func containsType(types []catalog.ProductType, t catalog.ProductType) bool {
	for _, x := range types {
		if x == t {
			return true
		}
	}
	return false
}

func lineField(i int, f string) string { return fmt.Sprintf("lines[%d].%s", i, f) }

// estimateIssue menaksir HPP barang keluar (FEFO bila batch kosong, FIFO di dalam batch) tanpa mengunci,
// untuk nominal approval. Stok kurang ditolak STOCK_INSUFFICIENT agar dokumen tidak diajukan tanpa barang.
func (s *Service) estimateIssue(ctx context.Context, productID uuid.UUID, from domain.Place, batchID *uuid.UUID, qty decimal.Decimal, prod catalog.Product, loc warehouse.Location) (decimal.Decimal, error) {
	var allocs []domain.Allocation
	if batchID != nil || !prod.HasBatch {
		allocs = []domain.Allocation{{Candidate: domain.Candidate{BatchID: batchID}, Qty: qty}}
		cands, err := s.Store.BalanceCandidates(ctx, productID, from.LocationID, from.Status)
		if err != nil {
			return decimal.Zero, err
		}
		have := decimal.Zero
		for _, c := range cands {
			if domain.SameBatch(c.BatchID, batchID) {
				have = have.Add(c.Qty)
			}
		}
		if have.LessThan(qty) {
			return decimal.Zero, domain.InsufficientStock(prod.SKU, loc.Code, from.Status, have)
		}
	} else {
		cands, err := s.Store.BalanceCandidates(ctx, productID, from.LocationID, from.Status)
		if err != nil {
			return decimal.Zero, err
		}
		var short decimal.Decimal
		allocs, short = s.Allocator.Allocate(cands, qty, s.today())
		if short.IsPositive() {
			return decimal.Zero, domain.InsufficientStock(prod.SKU, loc.Code, from.Status, qty.Sub(short))
		}
	}
	total := decimal.Zero
	for _, a := range allocs {
		set, err := s.Store.OpenLayers(ctx, productID, a.Candidate.BatchID)
		if err != nil {
			return decimal.Zero, err
		}
		cons, ok := set.Peek(a.Qty)
		if !ok {
			return decimal.Zero, fmt.Errorf("estimate %s: %w", prod.SKU, errLayerMismatch)
		}
		total = total.Add(domain.CostAmount(cons))
	}
	return total, nil
}

// --- Tampilan ---------------------------------------------------------------------------------

// BatchRef adalah ringkasan batch untuk tampilan.
type BatchRef struct {
	ID         uuid.UUID
	Number     string
	ExpiryDate *clock.BusinessDate
}

// DocRefs adalah master yang dirujuk dokumen: produk, lokasi, batch, dimensi, alasan retur, file.
type DocRefs struct {
	Refs
	Batches    map[uuid.UUID]BatchRef
	Dimensions map[uuid.UUID]DimensionOption
	Reasons    map[uuid.UUID]ReturnReason
	Files      map[uuid.UUID]string
}

// Batch mengembalikan ref batch.
func (r DocRefs) Batch(id *uuid.UUID) *BatchRef {
	if id == nil {
		return nil
	}
	if b, ok := r.Batches[*id]; ok {
		return &b
	}
	return &BatchRef{ID: *id, Number: id.String()}
}

// Dimension mengembalikan ref dimensi.
func (r DocRefs) Dimension(id *uuid.UUID) *DimensionOption {
	if id == nil {
		return nil
	}
	if d, ok := r.Dimensions[*id]; ok {
		return &d
	}
	return &DimensionOption{ID: *id, Code: id.String()}
}

type refIDs struct {
	products, locations, batches, dimensions, reasons, files []uuid.UUID
}

func (s *Service) docRefs(ctx context.Context, ids refIDs) (DocRefs, error) {
	base, err := s.refs(ctx, ids.products, ids.locations)
	if err != nil {
		return DocRefs{}, err
	}
	out := DocRefs{
		Refs: base, Batches: map[uuid.UUID]BatchRef{}, Dimensions: map[uuid.UUID]DimensionOption{},
		Reasons: map[uuid.UUID]ReturnReason{}, Files: map[uuid.UUID]string{},
	}
	if len(ids.batches) > 0 {
		bs, err := s.Store.BatchesByIDs(ctx, uniq(ids.batches))
		if err != nil {
			return DocRefs{}, err
		}
		for id, b := range bs {
			out.Batches[id] = BatchRef{ID: id, Number: b.Number, ExpiryDate: b.ExpiryDate}
		}
	}
	if len(ids.dimensions) > 0 && s.Dimensions != nil {
		for _, kind := range []string{DimensionDivision, DimensionCampaign} {
			opts, err := s.Dimensions.DimensionOptions(ctx, kind, true)
			if err != nil {
				return DocRefs{}, err
			}
			for _, o := range opts {
				out.Dimensions[o.ID] = o
			}
		}
	}
	if s.ReturnReasons != nil {
		for _, id := range uniq(ids.reasons) {
			r, err := s.ReturnReasons.ReturnReason(ctx, id)
			if apperr.HasCode(err, apperr.CodeNotFound) {
				continue
			}
			if err != nil {
				return DocRefs{}, err
			}
			out.Reasons[id] = r
		}
	}
	if len(ids.files) > 0 && s.Files != nil {
		names, err := s.Files.Names(ctx, uniq(ids.files))
		if err != nil {
			return DocRefs{}, err
		}
		out.Files = names
	}
	return out, nil
}

// SourceMovements membaca batch yang keluar/dipindah satu dokumen untuk tab detail (permission dokumen
// diperiksa pemanggil).
func (s *Service) sourceMovements(ctx context.Context, sourceType string, id uuid.UUID) ([]postgres.SourceMovement, error) {
	return s.Store.SourceMovements(ctx, sourceType, id, "")
}

// DimensionOptions mengembalikan divisi atau kampanye aktif untuk form Pemakaian Internal.
func (s *Service) DimensionOptions(ctx context.Context, kind string) ([]DimensionOption, error) {
	if _, err := actor.RequireAny(ctx, domain.PermUsageRead, domain.PermUsageWrite); err != nil {
		return nil, err
	}
	if kind != DimensionDivision && kind != DimensionCampaign {
		return nil, apperr.Validation(map[string]string{"kind": "Jenis dimensi tidak valid"})
	}
	if err := requireDep(s.Dimensions != nil, "accounting"); err != nil {
		return nil, err
	}
	return s.Dimensions.DimensionOptions(ctx, kind, false)
}
