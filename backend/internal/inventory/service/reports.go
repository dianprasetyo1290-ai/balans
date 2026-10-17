package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/tabular"
	"balans/internal/warehouse"
)

// Laporan persediaan (S-414, S-415): posisi & nilai stok (RPT-01), mutasi (RPT-02), kedaluwarsa & umur stok
// (RPT-19), pemakaian internal (RPT-08), dan retur & tindak lanjut (RPT-09). Posisi & nilai dari read model
// rm_stock_values (di-refresh job accounting.refresh_read_models), nama produk/lokasi lewat API modul pemiliknya.

// ReadModelStock adalah nama read model stok di status read model.
const ReadModelStock = "inventory.stock_values"

// Permission laporan yang juga membuka laporan persediaan.
const (
	permReportRead      = "accounting.report.read"
	permSalesReturnRead = "sales.sales_return.read"
	maxReportDays       = 732
)

type reportStore interface {
	RefreshStockReadModel(ctx context.Context) error
	StockValueRows(ctx context.Context, f postgres.StockReportFilter) ([]postgres.StockValueRow, error)
	MovementSummary(ctx context.Context, f postgres.StockReportFilter, from, to clock.BusinessDate) ([]postgres.MovementSummaryRow, error)
	UsageReport(ctx context.Context, f postgres.UsageReportFilter) ([]postgres.UsageReportRow, error)
	ReturnReport(ctx context.Context, from, to clock.BusinessDate) ([]postgres.ReturnReportRow, error)
}

var errNoReportStore = errors.New("inventory: store tidak mendukung laporan")

func (s *Service) reports() (reportStore, error) {
	rs, ok := s.Store.(reportStore)
	if !ok {
		return nil, errNoReportStore
	}
	return rs, nil
}

// RefreshReadModels me-refresh read model stok.
func (s *Service) RefreshReadModels(ctx context.Context) error {
	rs, err := s.reports()
	if err != nil {
		return err
	}
	return rs.RefreshStockReadModel(ctx)
}

// ReportRefs adalah master untuk tampilan laporan.
type ReportRefs struct {
	Products  map[uuid.UUID]ProductRef
	Locations map[uuid.UUID]ReportLocation
}

// ReportLocation adalah lokasi beserta jenisnya.
type ReportLocation struct {
	LocationRef
	Type string
}

func (s *Service) reportRefs(ctx context.Context, productIDs, locationIDs []uuid.UUID) (ReportRefs, error) {
	out := ReportRefs{Products: map[uuid.UUID]ProductRef{}, Locations: map[uuid.UUID]ReportLocation{}}
	if len(productIDs) > 0 && s.Products != nil {
		prods, err := s.Products.Products(ctx, uniq(productIDs))
		if err != nil {
			return ReportRefs{}, err
		}
		for id, p := range prods {
			out.Products[id] = ProductRef{ID: id, SKU: p.SKU, Name: p.DisplayName(), UOM: p.UOM, HasBatch: p.HasBatch}
		}
	}
	if len(locationIDs) > 0 && s.Locations != nil {
		list, err := s.Locations.Locations(ctx, uniq(locationIDs))
		if err != nil {
			return ReportRefs{}, err
		}
		for id, l := range list {
			out.Locations[id] = ReportLocation{
				LocationRef: LocationRef{ID: id, Code: l.Code, Name: l.Name, WarehouseName: l.WarehouseName}, Type: string(l.Type),
			}
		}
	}
	return out, nil
}

// Product mengembalikan ref produk.
func (r ReportRefs) Product(id uuid.UUID) ProductRef {
	if p, ok := r.Products[id]; ok {
		return p
	}
	return ProductRef{ID: id, SKU: id.String()}
}

// Location mengembalikan ref lokasi.
func (r ReportRefs) Location(id uuid.UUID) ReportLocation {
	if l, ok := r.Locations[id]; ok {
		return l
	}
	return ReportLocation{LocationRef: LocationRef{ID: id, Code: id.String()}}
}

// StockReportFilter adalah filter laporan stok; LocationType membatasi jenis lokasi (mis. consignment).
type StockReportFilter struct {
	postgres.StockReportFilter
	LocationType string
}

func (s *Service) resolveLocationType(ctx context.Context, f *StockReportFilter) (bool, error) {
	if f.LocationType == "" {
		return true, nil
	}
	if s.Locations == nil {
		return false, nil
	}
	list, err := s.Locations.FindLocations(ctx, warehouse.LocationQuery{Types: []warehouse.LocationType{warehouse.LocationType(f.LocationType)}})
	if err != nil {
		return false, err
	}
	var ids []uuid.UUID
	for _, l := range list {
		if len(f.LocationIDs) == 0 || containsID(f.LocationIDs, l.ID) {
			ids = append(ids, l.ID)
		}
	}
	f.LocationIDs = ids
	return len(ids) > 0, nil
}

func containsID(list []uuid.UUID, id uuid.UUID) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// TypeTotal adalah ringkasan stok per jenis lokasi.
type TypeTotal struct {
	Type  string
	Qty   decimal.Decimal
	Value decimal.Decimal
}

// StockPosition adalah posisi & nilai stok (RPT-01).
type StockPosition struct {
	Rows       []postgres.StockValueRow
	Refs       ReportRefs
	TotalQty   decimal.Decimal
	TotalValue decimal.Decimal
	ByType     []TypeTotal
}

// StockPositionReport membaca posisi & nilai stok per produk × lokasi × batch, termasuk lokasi konsinyasi,
// buffer vending, maklon, dan Dalam Pengiriman (status in_transit di lokasi tujuan/transit).
func (s *Service) StockPositionReport(ctx context.Context, f StockReportFilter) (StockPosition, error) {
	if _, err := actor.RequireAny(ctx, PermStockRead, permReportRead); err != nil {
		return StockPosition{}, err
	}
	rs, err := s.reports()
	if err != nil {
		return StockPosition{}, err
	}
	out := StockPosition{TotalQty: decimal.Zero, TotalValue: decimal.Zero}
	if ok, err := s.resolveLocationType(ctx, &f); err != nil || !ok {
		return out, err
	}
	rows, err := rs.StockValueRows(ctx, f.StockReportFilter)
	if err != nil {
		return StockPosition{}, err
	}
	var pids, lids []uuid.UUID
	for _, r := range rows {
		pids, lids = append(pids, r.ProductID), append(lids, r.LocationID)
	}
	if out.Refs, err = s.reportRefs(ctx, pids, lids); err != nil {
		return StockPosition{}, err
	}
	byType := map[string]*TypeTotal{}
	for _, r := range rows {
		out.TotalQty, out.TotalValue = out.TotalQty.Add(r.Qty), out.TotalValue.Add(r.Value)
		t := out.Refs.Location(r.LocationID).Type
		tt, ok := byType[t]
		if !ok {
			tt = &TypeTotal{Type: t, Qty: decimal.Zero, Value: decimal.Zero}
			byType[t] = tt
		}
		tt.Qty, tt.Value = tt.Qty.Add(r.Qty), tt.Value.Add(r.Value)
	}
	for _, t := range byType {
		out.ByType = append(out.ByType, *t)
	}
	sort.Slice(out.ByType, func(i, j int) bool { return out.ByType[i].Type < out.ByType[j].Type })
	out.Rows = rows
	return out, nil
}

func checkRange(from, to clock.BusinessDate) error {
	fields := map[string]string{}
	switch {
	case from.IsZero():
		fields["dateFrom"] = "Tanggal awal wajib diisi"
	case to.IsZero():
		fields["dateTo"] = "Tanggal akhir wajib diisi"
	case to.Before(from):
		fields["dateTo"] = "Tanggal akhir tidak boleh sebelum tanggal awal"
	case to.Time().Sub(from.Time()) > maxReportDays*24*time.Hour:
		fields["dateTo"] = "Rentang laporan paling panjang 2 tahun"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// MovementReport adalah mutasi stok per produk × lokasi pada periode (RPT-02).
type MovementReport struct {
	Rows []postgres.MovementSummaryRow
	Refs ReportRefs
}

// StockMovementReport menyusun saldo awal, masuk, keluar, pindah, dan saldo akhir per produk × lokasi.
func (s *Service) StockMovementReport(ctx context.Context, f StockReportFilter, from, to clock.BusinessDate) (MovementReport, error) {
	if _, err := actor.RequireAny(ctx, PermStockRead, permReportRead); err != nil {
		return MovementReport{}, err
	}
	if err := checkRange(from, to); err != nil {
		return MovementReport{}, err
	}
	rs, err := s.reports()
	if err != nil {
		return MovementReport{}, err
	}
	if ok, err := s.resolveLocationType(ctx, &f); err != nil || !ok {
		return MovementReport{}, err
	}
	rows, err := rs.MovementSummary(ctx, f.StockReportFilter, from, to)
	if err != nil {
		return MovementReport{}, err
	}
	var pids, lids []uuid.UUID
	for _, r := range rows {
		pids, lids = append(pids, r.ProductID), append(lids, r.LocationID)
	}
	refs, err := s.reportRefs(ctx, pids, lids)
	return MovementReport{Rows: rows, Refs: refs}, err
}

// ExpiryRow adalah satu batch × lokasi dengan sisa umur dan umur stok.
type ExpiryRow struct {
	postgres.StockValueRow
	DaysToExpiry *int
	AgeDays      *int
	Bucket       string
}

// Kelompok sisa umur simpan (RPT-19).
const (
	BucketExpired  = "expired"
	BucketNear     = "near"
	BucketUpTo90   = "d31_90"
	BucketUpTo180  = "d91_180"
	BucketOver180  = "over_180"
	BucketNoExpiry = "no_expiry"
)

func expiryBucket(days *int, near int) string {
	switch {
	case days == nil:
		return BucketNoExpiry
	case *days < 0:
		return BucketExpired
	case *days <= near:
		return BucketNear
	case *days <= 90:
		return BucketUpTo90
	case *days <= 180:
		return BucketUpTo180
	}
	return BucketOver180
}

func daysBetween(from, to clock.BusinessDate) int {
	return int(to.Time().Sub(from.Time()).Hours() / 24)
}

// ExpiryReport adalah stok mendekati kedaluwarsa & umur stok (RPT-19).
type ExpiryReport struct {
	Rows     []ExpiryRow
	Refs     ReportRefs
	Today    clock.BusinessDate
	NearDays int
}

// StockExpiryReport menyusun sisa umur simpan dan umur stok per batch per lokasi (qty > 0). withinDays > 0
// hanya menampilkan batch yang kedaluwarsa dalam N hari (termasuk yang sudah lewat).
func (s *Service) StockExpiryReport(ctx context.Context, f StockReportFilter, withinDays int) (ExpiryReport, error) {
	pos, err := s.StockPositionReport(ctx, f)
	if err != nil {
		return ExpiryReport{}, err
	}
	today := s.today()
	out := ExpiryReport{Refs: pos.Refs, Today: today, NearDays: domain.DefaultNearExpiryDays}
	for _, r := range pos.Rows {
		if !r.Qty.IsPositive() {
			continue
		}
		row := ExpiryRow{StockValueRow: r}
		if r.ExpiryDate != nil {
			d := daysBetween(today, *r.ExpiryDate)
			row.DaysToExpiry = &d
		}
		if r.FirstInDate != nil {
			a := daysBetween(*r.FirstInDate, today)
			row.AgeDays = &a
		}
		row.Bucket = expiryBucket(row.DaysToExpiry, out.NearDays)
		if withinDays > 0 && (row.DaysToExpiry == nil || *row.DaysToExpiry > withinDays) {
			continue
		}
		out.Rows = append(out.Rows, row)
	}
	sort.SliceStable(out.Rows, func(i, j int) bool {
		a, b := out.Rows[i].DaysToExpiry, out.Rows[j].DaysToExpiry
		if a == nil || b == nil {
			return b == nil && a != nil
		}
		return *a < *b
	})
	return out, nil
}

// UsageReport adalah pemakaian internal diposting (RPT-08).
type UsageReport struct {
	Rows       []postgres.UsageReportRow
	Refs       ReportRefs
	Dimensions map[uuid.UUID]DimensionOption
}

// InternalUsageReport membaca pemakaian internal diposting per kampanye, tujuan, divisi, dan produk.
func (s *Service) InternalUsageReport(ctx context.Context, f postgres.UsageReportFilter) (UsageReport, error) {
	if _, err := actor.RequireAny(ctx, domain.PermUsageRead, permReportRead); err != nil {
		return UsageReport{}, err
	}
	if err := checkRange(f.DateFrom, f.DateTo); err != nil {
		return UsageReport{}, err
	}
	rs, err := s.reports()
	if err != nil {
		return UsageReport{}, err
	}
	rows, err := rs.UsageReport(ctx, f)
	if err != nil {
		return UsageReport{}, err
	}
	var pids []uuid.UUID
	for _, r := range rows {
		pids = append(pids, r.ProductID)
	}
	out := UsageReport{Rows: rows, Dimensions: map[uuid.UUID]DimensionOption{}}
	if out.Refs, err = s.reportRefs(ctx, pids, nil); err != nil {
		return UsageReport{}, err
	}
	if s.Dimensions != nil && len(rows) > 0 {
		for _, kind := range []string{"division", "campaign"} {
			opts, err := s.Dimensions.DimensionOptions(ctx, kind, true)
			if err != nil {
				return UsageReport{}, err
			}
			for _, o := range opts {
				out.Dimensions[o.ID] = o
			}
		}
	}
	return out, nil
}

// ReturnReport adalah barang retur per alasan & tindak lanjut (RPT-09).
type ReturnReport struct {
	Rows    []postgres.ReturnReportRow
	Refs    ReportRefs
	Reasons map[uuid.UUID]ReturnReason
}

// ReturnDispositionReport membaca baris Disposisi Retur diposting beserta alasan retur dan disposisinya.
func (s *Service) ReturnDispositionReport(ctx context.Context, from, to clock.BusinessDate) (ReturnReport, error) {
	if _, err := actor.RequireAny(ctx, PermStockRead, permSalesReturnRead, permReportRead); err != nil {
		return ReturnReport{}, err
	}
	if err := checkRange(from, to); err != nil {
		return ReturnReport{}, err
	}
	rs, err := s.reports()
	if err != nil {
		return ReturnReport{}, err
	}
	rows, err := rs.ReturnReport(ctx, from, to)
	if err != nil {
		return ReturnReport{}, err
	}
	var pids, lids []uuid.UUID
	out := ReturnReport{Rows: rows, Reasons: map[uuid.UUID]ReturnReason{}}
	for _, r := range rows {
		pids, lids = append(pids, r.ProductID), append(lids, r.LocationID)
		if r.ReasonID != nil && s.ReturnReasons != nil {
			if _, ok := out.Reasons[*r.ReasonID]; !ok {
				rr, err := s.ReturnReasons.ReturnReason(ctx, *r.ReasonID)
				if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
					return ReturnReport{}, err
				}
				out.Reasons[*r.ReasonID] = rr
			}
		}
	}
	out.Refs, err = s.reportRefs(ctx, pids, lids)
	return out, err
}

// --- Ekspor (job dataexchange.export) --------------------------------------------------------

// ReportFilterFromParams membaca filter laporan stok dari parameter ekspor.
func ReportFilterFromParams(params map[string]string) (StockReportFilter, error) {
	f := StockReportFilter{LocationType: params["locationType"]}
	for key, dst := range map[string]**uuid.UUID{"productId": &f.ProductID, "batchId": &f.BatchID} {
		if params[key] == "" {
			continue
		}
		id, err := uuid.Parse(params[key])
		if err != nil {
			return f, apperr.Validation(map[string]string{key: "ID tidak valid"})
		}
		*dst = &id
	}
	if v := params["locationId"]; v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, apperr.Validation(map[string]string{"locationId": "ID tidak valid"})
		}
		f.LocationIDs = []uuid.UUID{id}
	}
	return f, nil
}

// PeriodFromParams membaca dateFrom/dateTo (default bulan berjalan).
func (s *Service) PeriodFromParams(params map[string]string) (clock.BusinessDate, clock.BusinessDate, error) {
	today := s.today()
	from, to := clock.NewBusinessDate(today.Year(), today.Month(), 1), today
	for key, dst := range map[string]*clock.BusinessDate{"dateFrom": &from, "dateTo": &to} {
		if params[key] == "" {
			continue
		}
		d, err := clock.ParseBusinessDate(params[key])
		if err != nil {
			return from, to, apperr.Validation(map[string]string{key: "Format tanggal YYYY-MM-DD"})
		}
		*dst = d
	}
	return from, to, checkRange(from, to)
}

func dateText(d *clock.BusinessDate) string {
	if d == nil {
		return ""
	}
	return d.String()
}

func intText(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

// StockPositionColumns adalah kolom ekspor posisi stok.
var StockPositionColumns = []tabular.Column{
	{Key: "sku", Header: "SKU", Kind: tabular.KindText},
	{Key: "product", Header: "Produk", Kind: tabular.KindText},
	{Key: "location", Header: "Lokasi", Kind: tabular.KindText},
	{Key: "location_type", Header: "Jenis Lokasi", Kind: tabular.KindText},
	{Key: "batch", Header: "Batch", Kind: tabular.KindText},
	{Key: "expiry_date", Header: "Kedaluwarsa", Kind: tabular.KindText},
	{Key: "qty", Header: "Qty", Kind: tabular.KindDecimal},
	{Key: "value", Header: "Nilai (HPP)", Kind: tabular.KindDecimal},
}

// ExportStockPosition mengalirkan posisi stok.
func (s *Service) ExportStockPosition(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	f, err := ReportFilterFromParams(params)
	if err != nil {
		return err
	}
	pos, err := s.StockPositionReport(ctx, f)
	if err != nil {
		return err
	}
	for i, r := range pos.Rows {
		p, l := pos.Refs.Product(r.ProductID), pos.Refs.Location(r.LocationID)
		row := tabular.NewRow(i + 1)
		row.Values["sku"], row.Values["product"] = p.SKU, p.Name
		row.Values["location"], row.Values["location_type"] = l.Code+" – "+l.Name, l.Type
		row.Values["batch"], row.Values["expiry_date"] = r.BatchNumber, dateText(r.ExpiryDate)
		row.Values["qty"], row.Values["value"] = r.Qty.String(), r.Value.StringFixed(2)
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

// MovementColumns adalah kolom ekspor mutasi stok.
var MovementColumns = []tabular.Column{
	{Key: "sku", Header: "SKU", Kind: tabular.KindText},
	{Key: "product", Header: "Produk", Kind: tabular.KindText},
	{Key: "location", Header: "Lokasi", Kind: tabular.KindText},
	{Key: "opening_qty", Header: "Saldo Awal", Kind: tabular.KindDecimal},
	{Key: "receive_qty", Header: "Masuk", Kind: tabular.KindDecimal},
	{Key: "issue_qty", Header: "Keluar", Kind: tabular.KindDecimal},
	{Key: "move_in_qty", Header: "Pindah Masuk", Kind: tabular.KindDecimal},
	{Key: "move_out_qty", Header: "Pindah Keluar", Kind: tabular.KindDecimal},
	{Key: "closing_qty", Header: "Saldo Akhir", Kind: tabular.KindDecimal},
	{Key: "closing_value", Header: "Nilai Akhir", Kind: tabular.KindDecimal},
}

// ExportStockMovements mengalirkan mutasi stok periode.
func (s *Service) ExportStockMovements(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	f, err := ReportFilterFromParams(params)
	if err != nil {
		return err
	}
	from, to, err := s.PeriodFromParams(params)
	if err != nil {
		return err
	}
	rep, err := s.StockMovementReport(ctx, f, from, to)
	if err != nil {
		return err
	}
	for i, r := range rep.Rows {
		p, l := rep.Refs.Product(r.ProductID), rep.Refs.Location(r.LocationID)
		row := tabular.NewRow(i + 1)
		row.Values["sku"], row.Values["product"], row.Values["location"] = p.SKU, p.Name, l.Code+" – "+l.Name
		row.Values["opening_qty"], row.Values["receive_qty"] = r.OpeningQty().String(), r.ReceiveQty.String()
		row.Values["issue_qty"], row.Values["move_in_qty"] = r.IssueQty.Neg().String(), r.MoveInQty.String()
		row.Values["move_out_qty"], row.Values["closing_qty"] = r.MoveOutQty.Neg().String(), r.ClosingQty.String()
		row.Values["closing_value"] = r.ClosingValue.StringFixed(2)
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

// ExpiryColumns adalah kolom ekspor kedaluwarsa & umur stok.
var ExpiryColumns = []tabular.Column{
	{Key: "sku", Header: "SKU", Kind: tabular.KindText},
	{Key: "product", Header: "Produk", Kind: tabular.KindText},
	{Key: "location", Header: "Lokasi", Kind: tabular.KindText},
	{Key: "batch", Header: "Batch", Kind: tabular.KindText},
	{Key: "expiry_date", Header: "Kedaluwarsa", Kind: tabular.KindText},
	{Key: "days_to_expiry", Header: "Sisa Hari", Kind: tabular.KindInteger},
	{Key: "age_days", Header: "Umur Stok (Hari)", Kind: tabular.KindInteger},
	{Key: "qty", Header: "Qty", Kind: tabular.KindDecimal},
	{Key: "value", Header: "Nilai (HPP)", Kind: tabular.KindDecimal},
}

// ExportStockExpiry mengalirkan laporan kedaluwarsa (params withinDays opsional).
func (s *Service) ExportStockExpiry(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	f, err := ReportFilterFromParams(params)
	if err != nil {
		return err
	}
	within := 0
	if v := params["withinDays"]; v != "" {
		if within, err = strconv.Atoi(v); err != nil || within < 0 {
			return apperr.Validation(map[string]string{"withinDays": "Harus angka ≥ 0"})
		}
	}
	rep, err := s.StockExpiryReport(ctx, f, within)
	if err != nil {
		return err
	}
	for i, r := range rep.Rows {
		p, l := rep.Refs.Product(r.ProductID), rep.Refs.Location(r.LocationID)
		row := tabular.NewRow(i + 1)
		row.Values["sku"], row.Values["product"], row.Values["location"] = p.SKU, p.Name, l.Code+" – "+l.Name
		row.Values["batch"], row.Values["expiry_date"] = r.BatchNumber, dateText(r.ExpiryDate)
		row.Values["days_to_expiry"], row.Values["age_days"] = intText(r.DaysToExpiry), intText(r.AgeDays)
		row.Values["qty"], row.Values["value"] = r.Qty.String(), r.Value.StringFixed(2)
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}
