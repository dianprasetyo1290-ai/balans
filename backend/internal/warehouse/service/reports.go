package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/tabular"
	"balans/internal/warehouse/domain"
	"balans/internal/warehouse/postgres"
)

// Laporan Stock Count: hasil hitung & selisih per baris (RPT-03, S-414).

type countReportStore interface {
	StockCountVariances(ctx context.Context, f postgres.CountVarianceFilter) ([]postgres.CountVarianceRow, error)
}

var errNoCountReport = errors.New("warehouse: store tidak mendukung laporan stock count")

const maxCountReportDays = 732

// CountVarianceFilter adalah filter laporan stock count.
type CountVarianceFilter = postgres.CountVarianceFilter

// CountVarianceReport adalah hasil stock count beserta nama produk, batch, dan lokasi.
type CountVarianceReport struct {
	Rows      []postgres.CountVarianceRow
	Products  map[uuid.UUID][2]string
	Batches   map[uuid.UUID]string
	Locations map[uuid.UUID]domain.Location
}

// StockCountVarianceReport membaca hasil stock count & selisih (qty dan nilai HPP snapshot).
func (s *Service) StockCountVarianceReport(ctx context.Context, f CountVarianceFilter) (CountVarianceReport, error) {
	if _, err := actor.RequireAny(ctx, PermCountRead, "accounting.report.read"); err != nil {
		return CountVarianceReport{}, err
	}
	switch {
	case f.DateFrom.IsZero() || f.DateTo.IsZero() || f.DateTo.Before(f.DateFrom):
		return CountVarianceReport{}, apperr.Validation(map[string]string{"dateTo": "Rentang tanggal tidak valid"})
	case f.DateTo.Time().Sub(f.DateFrom.Time()) > maxCountReportDays*24*time.Hour:
		return CountVarianceReport{}, apperr.Validation(map[string]string{"dateTo": "Rentang laporan paling panjang 2 tahun"})
	}
	st, ok := s.Docs.(countReportStore)
	if !ok {
		return CountVarianceReport{}, errNoCountReport
	}
	rows, err := st.StockCountVariances(ctx, f)
	if err != nil {
		return CountVarianceReport{}, err
	}
	out := CountVarianceReport{Rows: rows, Products: map[uuid.UUID][2]string{}, Batches: map[uuid.UUID]string{}}
	var pids, bids, lids []uuid.UUID
	for _, r := range rows {
		pids, lids = append(pids, r.ProductID), append(lids, r.LocationID)
		if r.BatchID != nil {
			bids = append(bids, *r.BatchID)
		}
	}
	if out.Locations, err = s.LocationsByIDs(ctx, uniq(lids)); err != nil {
		return CountVarianceReport{}, err
	}
	d, err := s.deps()
	if err != nil {
		return out, nil //nolint:nilerr // tanpa modul inventory/catalog laporan tetap tampil dengan ID
	}
	if d.Products != nil && len(pids) > 0 {
		prods, err := d.Products.Products(ctx, uniq(pids))
		if err != nil {
			return CountVarianceReport{}, err
		}
		for id, p := range prods {
			out.Products[id] = [2]string{p.SKU, p.DisplayName()}
		}
	}
	if d.Ledger != nil && len(bids) > 0 {
		batches, err := d.Ledger.Batches(ctx, uniq(bids))
		if err != nil {
			return CountVarianceReport{}, err
		}
		for id, b := range batches {
			out.Batches[id] = b.Number
		}
	}
	return out, nil
}

// CountVarianceColumns adalah kolom ekspor laporan stock count.
var CountVarianceColumns = []tabular.Column{
	{Key: "number", Header: "Nomor SC", Kind: tabular.KindText},
	{Key: "date", Header: "Tanggal", Kind: tabular.KindText},
	{Key: "location", Header: "Lokasi", Kind: tabular.KindText},
	{Key: "sku", Header: "SKU", Kind: tabular.KindText},
	{Key: "product", Header: "Produk", Kind: tabular.KindText},
	{Key: "batch", Header: "Batch", Kind: tabular.KindText},
	{Key: "system_qty", Header: "Qty Sistem", Kind: tabular.KindDecimal},
	{Key: "counted_qty", Header: "Qty Hitung", Kind: tabular.KindDecimal},
	{Key: "variance_qty", Header: "Selisih Qty", Kind: tabular.KindDecimal},
	{Key: "variance_amount", Header: "Selisih Nilai", Kind: tabular.KindDecimal},
	{Key: "status", Header: "Status", Kind: tabular.KindText},
}

// ExportStockCountVariances mengalirkan laporan stock count (params dateFrom, dateTo, locationId, varianceOnly).
func (s *Service) ExportStockCountVariances(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	today := clock.Today(s.Clock)
	f := CountVarianceFilter{DateFrom: clock.NewBusinessDate(today.Year(), today.Month(), 1), DateTo: today,
		VarianceOnly: params["varianceOnly"] == "true"}
	for key, dst := range map[string]*clock.BusinessDate{"dateFrom": &f.DateFrom, "dateTo": &f.DateTo} {
		if params[key] == "" {
			continue
		}
		d, err := clock.ParseBusinessDate(params[key])
		if err != nil {
			return apperr.Validation(map[string]string{key: "Format tanggal YYYY-MM-DD"})
		}
		*dst = d
	}
	if v := params["locationId"]; v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return apperr.Validation(map[string]string{"locationId": "ID tidak valid"})
		}
		f.LocationID = &id
	}
	rep, err := s.StockCountVarianceReport(ctx, f)
	if err != nil {
		return err
	}
	for i, r := range rep.Rows {
		row := tabular.NewRow(i + 1)
		loc := rep.Locations[r.LocationID]
		row.Values["number"], row.Values["date"], row.Values["location"] = r.Number, r.BusinessDate.String(), loc.Code+" – "+loc.Name
		row.Values["sku"], row.Values["product"] = rep.Products[r.ProductID][0], rep.Products[r.ProductID][1]
		if r.BatchID != nil {
			row.Values["batch"] = rep.Batches[*r.BatchID]
		}
		row.Values["system_qty"], row.Values["variance_qty"] = r.SystemQty.String(), r.VarianceQty.String()
		if r.CountedQty != nil {
			row.Values["counted_qty"] = r.CountedQty.String()
		}
		row.Values["variance_amount"], row.Values["status"] = r.VarianceAmount.StringFixed(2), r.Status
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}
