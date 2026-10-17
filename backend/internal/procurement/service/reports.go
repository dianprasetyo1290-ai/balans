package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/tabular"
	"balans/internal/procurement/postgres"
)

// Laporan pengadaan (S-414, S-416): Outstanding PO (RPT-10), umur utang (RPT-11), komposisi HPP batch dengan
// rincian reject (RPT-04).

// AccountPayable adalah akun Utang Usaha (rekonsiliasi umur utang).
const AccountPayable = "2101"

type reportStore interface {
	OutstandingPurchaseOrders(ctx context.Context, supplierID *uuid.UUID) ([]postgres.OutstandingPOLine, error)
	OpenPayablesAsOf(ctx context.Context, asOf clock.BusinessDate) ([]postgres.OpenPayableRow, error)
	BatchCosts(ctx context.Context, from, to clock.BusinessDate, productID *uuid.UUID) ([]postgres.BatchCostRow, error)
}

var errNoReportStore = errors.New("procurement: store tidak mendukung laporan")

func (s *Service) reportStore() (reportStore, error) {
	rs, ok := s.Store.(reportStore)
	if !ok {
		return nil, errNoReportStore
	}
	return rs, nil
}

// OutstandingPOReport adalah baris PO terbuka beserta produk.
type OutstandingPOReport struct {
	AsOf     clock.BusinessDate
	Lines    []postgres.OutstandingPOLine
	Products map[uuid.UUID]ProductRef
}

// OutstandingPOReport menyusun Outstanding PO: qty belum diterima dan qty diterima belum ditagih per baris
// (RPT-10); nilai Barang Diterima Belum Ditagih ada di laporan GRNI.
func (s *Service) OutstandingPOReport(ctx context.Context, supplierID *uuid.UUID) (OutstandingPOReport, error) {
	if _, err := actor.RequireAny(ctx, PermOrderRead, PermBillRead, PermReportRead); err != nil {
		return OutstandingPOReport{}, err
	}
	rs, err := s.reportStore()
	if err != nil {
		return OutstandingPOReport{}, err
	}
	out := OutstandingPOReport{AsOf: s.today()}
	if out.Lines, err = rs.OutstandingPurchaseOrders(ctx, supplierID); err != nil {
		return OutstandingPOReport{}, err
	}
	var ids []uuid.UUID
	for _, l := range out.Lines {
		if l.ProductID != nil {
			ids = append(ids, *l.ProductID)
		}
	}
	out.Products, err = s.Catalog.Products(ctx, uniqueIDs(ids))
	return out, err
}

// Kelompok umur utang (hari lewat jatuh tempo).
var agingBuckets = []struct {
	Key  string
	Upto int
}{{"current", 0}, {"d1_30", 30}, {"d31_60", 60}, {"d61_90", 90}, {"over_90", 1 << 30}}

// PayableAgingBucket adalah satu kolom umur utang.
type PayableAgingBucket struct {
	Key    string
	Amount money.Amount
}

// PayableAgingRow adalah umur utang satu pemasok.
type PayableAgingRow struct {
	SupplierID   uuid.UUID
	SupplierCode string
	SupplierName string
	Buckets      []PayableAgingBucket
	Total        money.Amount
	Documents    []postgres.OpenPayableRow
}

// PayableAging adalah laporan umur utang beserta rekonsiliasi ke saldo akun 2101.
type PayableAging struct {
	AsOf          clock.BusinessDate
	Rows          []PayableAgingRow
	Totals        []PayableAgingBucket
	Total         money.Amount
	LedgerBalance money.Amount
}

func bucketOf(daysOverdue int) int {
	for i, b := range agingBuckets {
		if daysOverdue <= b.Upto {
			return i
		}
	}
	return len(agingBuckets) - 1
}

func newBuckets() []PayableAgingBucket {
	out := make([]PayableAgingBucket, len(agingBuckets))
	for i, b := range agingBuckets {
		out[i] = PayableAgingBucket{Key: b.Key, Amount: money.Zero}
	}
	return out
}

// PayableAgingReport menyusun umur utang per pemasok per tanggal (RPT-11): belum jatuh tempo, 1–30, 31–60,
// 61–90, > 90 hari; total dibandingkan dengan saldo akun Utang Usaha 2101.
func (s *Service) PayableAgingReport(ctx context.Context, asOf clock.BusinessDate) (PayableAging, error) {
	if _, err := actor.RequireAny(ctx, PermBillRead, PermReportRead, "accounting.payment.read"); err != nil {
		return PayableAging{}, err
	}
	if asOf.IsZero() {
		asOf = s.today()
	}
	rs, err := s.reportStore()
	if err != nil {
		return PayableAging{}, err
	}
	docs, err := rs.OpenPayablesAsOf(ctx, asOf)
	if err != nil {
		return PayableAging{}, err
	}
	out := PayableAging{AsOf: asOf, Totals: newBuckets(), Total: money.Zero}
	idx := map[uuid.UUID]int{}
	for _, d := range docs {
		i, ok := idx[d.SupplierID]
		if !ok {
			i = len(out.Rows)
			idx[d.SupplierID] = i
			out.Rows = append(out.Rows, PayableAgingRow{
				SupplierID: d.SupplierID, SupplierCode: d.SupplierCode, SupplierName: d.SupplierName, Buckets: newBuckets(), Total: money.Zero,
			})
		}
		overdue := int(asOf.Time().Sub(d.DueDate.Time()).Hours() / 24)
		b := bucketOf(overdue)
		r := &out.Rows[i]
		r.Buckets[b].Amount = r.Buckets[b].Amount.Add(d.Open)
		r.Total = r.Total.Add(d.Open)
		r.Documents = append(r.Documents, d)
		out.Totals[b].Amount = out.Totals[b].Amount.Add(d.Open)
		out.Total = out.Total.Add(d.Open)
	}
	if out.LedgerBalance, err = s.Ledger.AccountBalance(ctx, AccountPayable, asOf); err != nil {
		return PayableAging{}, err
	}
	return out, nil
}

// BatchCostReport adalah komposisi HPP per batch dengan rincian reject (RPT-04).
type BatchCostReport struct {
	Rows     []postgres.BatchCostRow
	Products map[uuid.UUID]ProductRef
}

const maxProcurementReportDays = 732

// BatchCostReport membaca komposisi HPP batch dari Penerimaan Barang diposting pada rentang tanggal.
func (s *Service) BatchCostReport(ctx context.Context, from, to clock.BusinessDate, productID *uuid.UUID) (BatchCostReport, error) {
	if _, err := actor.RequireAny(ctx, PermReceiptRead, PermReportRead, "inventory.stock.read"); err != nil {
		return BatchCostReport{}, err
	}
	if from.IsZero() || to.IsZero() || to.Before(from) || to.Time().Sub(from.Time()) > maxProcurementReportDays*24*time.Hour {
		return BatchCostReport{}, apperr.Validation(map[string]string{"dateTo": "Rentang tanggal tidak valid (maks. 2 tahun)"})
	}
	rs, err := s.reportStore()
	if err != nil {
		return BatchCostReport{}, err
	}
	rows, err := rs.BatchCosts(ctx, from, to, productID)
	if err != nil {
		return BatchCostReport{}, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ProductID
	}
	prods, err := s.Catalog.Products(ctx, uniqueIDs(ids))
	return BatchCostReport{Rows: rows, Products: prods}, err
}

// --- Ekspor ----------------------------------------------------------------------------------

// OutstandingPOColumns adalah kolom ekspor Outstanding PO.
var OutstandingPOColumns = []tabular.Column{
	{Key: "number", Header: "Nomor PO", Kind: tabular.KindText},
	{Key: "supplier", Header: "Pemasok", Kind: tabular.KindText},
	{Key: "expected_date", Header: "Tanggal Diharapkan", Kind: tabular.KindText},
	{Key: "item", Header: "Barang", Kind: tabular.KindText},
	{Key: "ordered_qty", Header: "Qty PO", Kind: tabular.KindDecimal},
	{Key: "received_qty", Header: "Qty Diterima", Kind: tabular.KindDecimal},
	{Key: "billed_qty", Header: "Qty Ditagih", Kind: tabular.KindDecimal},
	{Key: "open_qty", Header: "Belum Diterima", Kind: tabular.KindDecimal},
	{Key: "unbilled_qty", Header: "Diterima Belum Ditagih", Kind: tabular.KindDecimal},
	{Key: "open_value", Header: "Nilai Belum Diterima", Kind: tabular.KindDecimal},
}

// ExportOutstandingPO mengalirkan Outstanding PO.
func (s *Service) ExportOutstandingPO(ctx context.Context, _ map[string]string, emit func(tabular.Row) error) error {
	rep, err := s.OutstandingPOReport(ctx, nil)
	if err != nil {
		return err
	}
	for i, l := range rep.Lines {
		item := l.Description
		if l.ProductID != nil {
			if p, ok := rep.Products[*l.ProductID]; ok {
				item = p.SKU + " – " + p.Name
			}
		}
		open := l.OrderedQty.Sub(l.ReceivedQty)
		row := tabular.NewRow(i + 1)
		row.Values["number"], row.Values["supplier"] = l.Number, l.SupplierName
		row.Values["expected_date"], row.Values["item"] = l.ExpectedDate.String(), item
		row.Values["ordered_qty"], row.Values["received_qty"] = l.OrderedQty.String(), l.ReceivedQty.String()
		row.Values["billed_qty"], row.Values["open_qty"] = l.BilledQty.String(), open.String()
		row.Values["unbilled_qty"] = l.ReceivedQty.Sub(l.BilledQty).String()
		row.Values["open_value"] = l.UnitPrice.Mul(open).Round().String()
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

// PayableAgingColumns adalah kolom ekspor umur utang (per dokumen).
var PayableAgingColumns = []tabular.Column{
	{Key: "supplier", Header: "Pemasok", Kind: tabular.KindText},
	{Key: "document", Header: "Dokumen", Kind: tabular.KindText},
	{Key: "date", Header: "Tanggal", Kind: tabular.KindText},
	{Key: "due_date", Header: "Jatuh Tempo", Kind: tabular.KindText},
	{Key: "days_overdue", Header: "Hari Lewat Jatuh Tempo", Kind: tabular.KindInteger},
	{Key: "open", Header: "Sisa Utang", Kind: tabular.KindDecimal},
}

// ExportPayableAging mengalirkan umur utang per dokumen (params asOf).
func (s *Service) ExportPayableAging(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	asOf := s.today()
	if v := params["asOf"]; v != "" {
		d, err := clock.ParseBusinessDate(v)
		if err != nil {
			return apperr.Validation(map[string]string{"asOf": "Format tanggal YYYY-MM-DD"})
		}
		asOf = d
	}
	rep, err := s.PayableAgingReport(ctx, asOf)
	if err != nil {
		return err
	}
	n := 0
	for _, r := range rep.Rows {
		for _, d := range r.Documents {
			n++
			row := tabular.NewRow(n)
			row.Values["supplier"], row.Values["document"] = r.SupplierName, d.DocumentNumber
			row.Values["date"], row.Values["due_date"] = d.BusinessDate.String(), d.DueDate.String()
			row.Values["days_overdue"] = daysText(int(asOf.Time().Sub(d.DueDate.Time()).Hours() / 24))
			row.Values["open"] = d.Open.String()
			if err := emit(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func daysText(n int) string {
	if n < 0 {
		n = 0
	}
	return money.FromInt(int64(n)).Decimal().StringFixed(0)
}

// Today adalah tanggal bisnis hari ini (WIB) menurut jam service.
func (s *Service) Today() clock.BusinessDate { return s.today() }
