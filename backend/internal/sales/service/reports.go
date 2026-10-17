package service

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/tabular"
	"balans/internal/sales/postgres"
)

// Laporan penjualan per channel/SKU/periode (RPT-05), margin kotor per produk/kategori (RPT-15), dan
// rekonsiliasi konsinyasi per mitra (RPT-06) dari read model rm_sales_lines (TG 5.17).

// Permission laporan; laporan keuangan (accounting.report.read) juga membuka laporan penjualan & margin.
const (
	PermReportRead           = "accounting.report.read"
	PermConsignmentRecRead   = "sales.consignment_reconciliation.read"
	maxSalesReportDays       = 732
	ReadModelSalesLines      = "sales.sales_lines"
	salesReportUncategorized = "Tanpa kategori"
)

// Pengelompokan laporan penjualan.
const (
	GroupChannel    = "channel"
	GroupSubChannel = "sub_channel"
	GroupProduct    = "product"
	GroupCategory   = "category"
	GroupMonth      = "month"
	GroupDay        = "day"
)

var salesGroups = []string{GroupChannel, GroupSubChannel, GroupProduct, GroupCategory, GroupMonth, GroupDay}

// SalesReportFilter adalah kriteria laporan penjualan.
type SalesReportFilter = postgres.SalesReportFilter

// SalesReportLine adalah satu baris laporan penjualan & margin.
type SalesReportLine struct {
	Key         string
	Code        string
	Name        string
	Qty         money.Amount
	Gross       money.Amount
	Discount    money.Amount
	Net         money.Amount
	COGS        money.Amount
	GrossProfit money.Amount
	MarginPct   decimal.Decimal
}

func (l *SalesReportLine) add(r postgres.SalesReportRow) {
	l.Qty, l.Gross, l.Discount = l.Qty.Add(r.Qty), l.Gross.Add(r.Gross), l.Discount.Add(r.Discount)
	l.Net, l.COGS = l.Net.Add(r.Net), l.COGS.Add(r.COGS)
}

func (l *SalesReportLine) finish() {
	l.GrossProfit = l.Net.Sub(l.COGS)
	l.MarginPct = decimal.Zero
	if !l.Net.IsZero() {
		l.MarginPct = l.GrossProfit.Decimal().Div(l.Net.Decimal()).Mul(decimal.NewFromInt(100)).Round(2)
	}
}

func newLine(key string) SalesReportLine {
	return SalesReportLine{Key: key, Qty: money.Zero, Gross: money.Zero, Discount: money.Zero, Net: money.Zero, COGS: money.Zero}
}

// SalesReport adalah laporan penjualan & margin.
type SalesReport struct {
	Filter SalesReportFilter
	Lines  []SalesReportLine
	Total  SalesReportLine
}

func checkReportRange(from, to clock.BusinessDate) error {
	fields := map[string]string{}
	switch {
	case from.IsZero():
		fields["dateFrom"] = "Tanggal awal wajib diisi"
	case to.IsZero():
		fields["dateTo"] = "Tanggal akhir wajib diisi"
	case to.Before(from):
		fields["dateTo"] = "Tanggal akhir tidak boleh sebelum tanggal awal"
	case to.Time().Sub(from.Time()) > maxSalesReportDays*24*time.Hour:
		fields["dateTo"] = "Rentang laporan paling panjang 2 tahun"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// SalesReport menyusun laporan penjualan per channel, sub-channel, produk, kategori, bulan, atau hari: qty,
// bruto, diskon, neto (= jurnal 4101 − 4102), HPP, laba kotor, dan margin.
func (s *DocumentService) SalesReport(ctx context.Context, f SalesReportFilter) (SalesReport, error) {
	if _, err := actor.RequireAny(ctx, PermOrderRead, PermMarketplaceRead, PermInvoiceRead, PermReportRead); err != nil {
		return SalesReport{}, err
	}
	if f.GroupBy == "" {
		f.GroupBy = GroupChannel
	}
	valid := false
	for _, g := range salesGroups {
		valid = valid || g == f.GroupBy
	}
	if !valid {
		return SalesReport{}, apperr.Validation(map[string]string{"groupBy": "Pengelompokan tidak valid"})
	}
	if err := checkReportRange(f.DateFrom, f.DateTo); err != nil {
		return SalesReport{}, err
	}
	q := f
	if f.GroupBy == GroupCategory {
		q.GroupBy = GroupProduct
	}
	rows, err := s.Store.SalesReport(ctx, q)
	if err != nil {
		return SalesReport{}, err
	}
	out := SalesReport{Filter: f, Total: newLine("")}
	byKey := map[string]*SalesReportLine{}
	var order []string
	keyOf := func(r postgres.SalesReportRow) string { return r.Key }
	labels := map[string][2]string{}
	switch f.GroupBy {
	case GroupChannel, GroupSubChannel:
		ids := parseIDs(rows)
		refs, err := s.Ledger.ChannelLabels(ctx, ids)
		if err != nil {
			return SalesReport{}, err
		}
		for id, r := range refs {
			labels[id.String()] = [2]string{r.Code, r.Name}
		}
	case GroupProduct, GroupCategory:
		ids := parseIDs(rows)
		prods, err := s.Catalog.Products(ctx, ids)
		if err != nil {
			return SalesReport{}, err
		}
		catOf := map[string]string{}
		for id, p := range prods {
			if f.GroupBy == GroupProduct {
				labels[id.String()] = [2]string{p.SKU, p.Name}
				continue
			}
			key, name := "", salesReportUncategorized
			if p.CategoryID != nil {
				key, name = p.CategoryID.String(), p.CategoryName
			}
			catOf[id.String()] = key
			labels[key] = [2]string{"", name}
		}
		if f.GroupBy == GroupCategory {
			labels[""] = [2]string{"", salesReportUncategorized}
			keyOf = func(r postgres.SalesReportRow) string { return catOf[r.Key] }
		}
	}
	for _, r := range rows {
		k := keyOf(r)
		l, ok := byKey[k]
		if !ok {
			nl := newLine(k)
			l = &nl
			byKey[k] = l
			order = append(order, k)
		}
		l.add(r)
		out.Total.add(r)
	}
	for _, k := range order {
		l := byKey[k]
		l.Code, l.Name = labels[k][0], labels[k][1]
		if l.Name == "" {
			l.Name = "Lainnya"
			if f.GroupBy == GroupMonth || f.GroupBy == GroupDay {
				l.Name = k
			}
		}
		l.finish()
		out.Lines = append(out.Lines, *l)
	}
	if f.GroupBy != GroupMonth && f.GroupBy != GroupDay {
		sort.SliceStable(out.Lines, func(i, j int) bool { return out.Lines[i].Net.Cmp(out.Lines[j].Net) > 0 })
	}
	out.Total.finish()
	return out, nil
}

func parseIDs(rows []postgres.SalesReportRow) []uuid.UUID {
	var ids []uuid.UUID
	for _, r := range rows {
		if id, err := uuid.Parse(r.Key); err == nil && id != uuid.Nil {
			ids = append(ids, id)
		}
	}
	return uniqueIDs(ids)
}

// ConsignmentReport menyusun invoice konsinyasi & selisih per mitra (RPT-06). Kosong selama rekonsiliasi
// konsinyasi (S-303 – S-305) belum menerbitkan invoice.
func (s *DocumentService) ConsignmentReport(ctx context.Context, from, to clock.BusinessDate) ([]postgres.ConsignmentRow, error) {
	if _, err := actor.RequireAny(ctx, PermInvoiceRead, PermConsignmentRecRead, PermReportRead); err != nil {
		return nil, err
	}
	if err := checkReportRange(from, to); err != nil {
		return nil, err
	}
	return s.Store.ConsignmentReport(ctx, from, to)
}

// RefreshReadModels me-refresh read model penjualan (dipanggil job accounting.refresh_read_models).
func (s *DocumentService) RefreshReadModels(ctx context.Context) error {
	return s.Store.RefreshSalesReadModel(ctx)
}

// --- Ekspor ----------------------------------------------------------------------------------

// SalesReportColumns adalah kolom ekspor laporan penjualan & margin.
var SalesReportColumns = []tabular.Column{
	{Key: "key", Header: "Kelompok", Kind: tabular.KindText},
	{Key: "code", Header: "Kode", Kind: tabular.KindText},
	{Key: "name", Header: "Nama", Kind: tabular.KindText},
	{Key: "qty", Header: "Qty", Kind: tabular.KindDecimal},
	{Key: "gross", Header: "Penjualan Bruto", Kind: tabular.KindDecimal},
	{Key: "discount", Header: "Diskon", Kind: tabular.KindDecimal},
	{Key: "net", Header: "Penjualan Neto", Kind: tabular.KindDecimal},
	{Key: "cogs", Header: "HPP", Kind: tabular.KindDecimal},
	{Key: "gross_profit", Header: "Laba Kotor", Kind: tabular.KindDecimal},
	{Key: "margin", Header: "Margin %", Kind: tabular.KindDecimal},
}

func optID(params map[string]string, key string) (*uuid.UUID, error) {
	if params[key] == "" {
		return nil, nil
	}
	id, err := uuid.Parse(params[key])
	if err != nil {
		return nil, apperr.Validation(map[string]string{key: "ID tidak valid"})
	}
	return &id, nil
}

// SalesReportFilterFromParams membaca parameter ekspor (groupBy, dateFrom, dateTo, channelId, subChannelId,
// productId, orderType); periode default bulan berjalan.
func (s *DocumentService) SalesReportFilterFromParams(params map[string]string) (SalesReportFilter, error) {
	today := s.today()
	f := SalesReportFilter{GroupBy: params["groupBy"], OrderType: params["orderType"], DateTo: today,
		DateFrom: clock.NewBusinessDate(today.Year(), today.Month(), 1)}
	var err error
	for key, dst := range map[string]*clock.BusinessDate{"dateFrom": &f.DateFrom, "dateTo": &f.DateTo} {
		if params[key] == "" {
			continue
		}
		if *dst, err = clock.ParseBusinessDate(params[key]); err != nil {
			return f, apperr.Validation(map[string]string{key: "Format tanggal YYYY-MM-DD"})
		}
	}
	if f.ChannelID, err = optID(params, "channelId"); err != nil {
		return f, err
	}
	if f.SubChannelID, err = optID(params, "subChannelId"); err != nil {
		return f, err
	}
	if f.ProductID, err = optID(params, "productId"); err != nil {
		return f, err
	}
	return f, checkReportRange(f.DateFrom, f.DateTo)
}

// ExportSalesReport mengalirkan laporan penjualan & margin.
func (s *DocumentService) ExportSalesReport(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	f, err := s.SalesReportFilterFromParams(params)
	if err != nil {
		return err
	}
	r, err := s.SalesReport(ctx, f)
	if err != nil {
		return err
	}
	for i, l := range append(r.Lines, func() SalesReportLine { t := r.Total; t.Key, t.Name = "", "TOTAL"; return t }()) {
		row := tabular.NewRow(i + 1)
		row.Values["key"], row.Values["code"], row.Values["name"] = l.Key, l.Code, l.Name
		row.Values["qty"], row.Values["gross"], row.Values["discount"] = l.Qty.Decimal().String(), l.Gross.String(), l.Discount.String()
		row.Values["net"], row.Values["cogs"], row.Values["gross_profit"] = l.Net.String(), l.COGS.String(), l.GrossProfit.String()
		row.Values["margin"] = l.MarginPct.StringFixed(2)
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}
