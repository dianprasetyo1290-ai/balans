package service

import (
	"context"
	"strconv"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/tabular"
)

// Ekspor laporan ke Excel sebagai job dataexchange.export (TG 5.17, 7.3). Parameter disimpan di job.

// ReportParam membaca tanggal dari parameter ekspor; kosong = def.
func ReportParam(params map[string]string, key string, def clock.BusinessDate) (clock.BusinessDate, error) {
	v := params[key]
	if v == "" {
		return def, nil
	}
	d, err := clock.ParseBusinessDate(v)
	if err != nil {
		return clock.BusinessDate{}, apperr.Validation(map[string]string{key: "Format tanggal YYYY-MM-DD"})
	}
	return d, nil
}

// ReportUUIDParam membaca UUID opsional dari parameter ekspor.
func ReportUUIDParam(params map[string]string, key string) (*uuid.UUID, error) {
	v := params[key]
	if v == "" {
		return nil, nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return nil, apperr.Validation(map[string]string{key: "ID tidak valid"})
	}
	return &id, nil
}

// MonthStart adalah tanggal 1 bulan berjalan.
func MonthStart(d clock.BusinessDate) clock.BusinessDate { return clock.NewBusinessDate(d.Year(), d.Month(), 1) }

func (s *ReportService) periodParams(params map[string]string) (clock.BusinessDate, clock.BusinessDate, error) {
	today := clock.Today(s.Clock)
	to, err := ReportParam(params, "dateTo", today)
	if err != nil {
		return clock.BusinessDate{}, clock.BusinessDate{}, err
	}
	from, err := ReportParam(params, "dateFrom", MonthStart(to))
	if err != nil {
		return clock.BusinessDate{}, clock.BusinessDate{}, err
	}
	return from, to, nil
}

// StatementColumns adalah kolom ekspor laporan keuangan.
var StatementColumns = []tabular.Column{
	{Key: "row_kind", Header: "Jenis Baris", Kind: tabular.KindText},
	{Key: "account_code", Header: "Kode Akun", Kind: tabular.KindText},
	{Key: "label", Header: "Uraian", Kind: tabular.KindText},
	{Key: "amount", Header: "Nilai", Kind: tabular.KindDecimal},
}

func statementFilter(kindParam string, from, to clock.BusinessDate) (StatementFilter, error) {
	k := domain.StatementKind(kindParam)
	if !k.Valid() {
		return StatementFilter{}, apperr.Validation(map[string]string{"kind": "Jenis laporan tidak valid"})
	}
	return StatementFilter{Kind: k, DateFrom: from, DateTo: to}, nil
}

// ValidateStatementParams memeriksa parameter ekspor laporan keuangan (kind, dateFrom, dateTo).
func (s *ReportService) ValidateStatementParams(params map[string]string) error {
	from, to, err := s.periodParams(params)
	if err != nil {
		return err
	}
	f, err := statementFilter(params["kind"], from, to)
	if err != nil {
		return err
	}
	if f.Kind == domain.StatementBalanceSheet {
		return nil
	}
	return domain.CheckRange(from, to)
}

// ExportStatement mengalirkan baris laporan keuangan.
func (s *ReportService) ExportStatement(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	from, to, err := s.periodParams(params)
	if err != nil {
		return err
	}
	f, err := statementFilter(params["kind"], from, to)
	if err != nil {
		return err
	}
	st, err := s.Statement(ctx, f)
	if err != nil {
		return err
	}
	header := tabular.NewRow(0)
	header.Values["row_kind"] = "judul"
	header.Values["label"] = st.Kind.Label() + " " + st.DateFrom.String() + " s.d. " + st.DateTo.String()
	if err := emit(header); err != nil {
		return err
	}
	for i, r := range st.Rows {
		row := tabular.NewRow(i + 1)
		row.Values["row_kind"] = string(r.Kind)
		row.Values["account_code"] = r.Code
		row.Values["label"] = r.Label
		if r.Kind != domain.RowSection && r.Kind != domain.RowGroup {
			row.Values["amount"] = r.Amount.String()
		}
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

// DimensionColumns adalah kolom ekspor laba rugi per dimensi (format panjang: akun × nilai dimensi).
var DimensionColumns = []tabular.Column{
	{Key: "dimension", Header: "Dimensi", Kind: tabular.KindText},
	{Key: "account_code", Header: "Kode Akun", Kind: tabular.KindText},
	{Key: "account_name", Header: "Nama Akun", Kind: tabular.KindText},
	{Key: "amount", Header: "Nilai", Kind: tabular.KindDecimal},
}

func dimensionParams(params map[string]string) domain.DimensionKind {
	if k := domain.DimensionKind(params["dimension"]); k != "" {
		return k
	}
	return domain.KindChannel
}

// ValidateDimensionParams memeriksa parameter ekspor laba rugi per dimensi.
func (s *ReportService) ValidateDimensionParams(params map[string]string) error {
	if !dimensionParams(params).Valid() {
		return apperr.Validation(map[string]string{"dimension": "Dimensi tidak valid"})
	}
	from, to, err := s.periodParams(params)
	if err != nil {
		return err
	}
	return domain.CheckRange(from, to)
}

// ExportDimensionProfitLoss mengalirkan laba rugi per dimensi.
func (s *ReportService) ExportDimensionProfitLoss(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	from, to, err := s.periodParams(params)
	if err != nil {
		return err
	}
	r, err := s.DimensionProfitLoss(ctx, DimensionReportFilter{Kind: dimensionParams(params), DateFrom: from, DateTo: to})
	if err != nil {
		return err
	}
	n := 0
	for _, a := range r.Accounts {
		for i, c := range r.Columns {
			if a.Amounts[i].IsZero() {
				continue
			}
			label := "Tanpa " + r.Kind.Label()
			if c.ID != uuid.Nil {
				label = r.Labels[c.ID].Code + " – " + r.Labels[c.ID].Name
			}
			n++
			row := tabular.NewRow(n)
			row.Values["dimension"] = label
			row.Values["account_code"] = a.Account.Code
			row.Values["account_name"] = a.Account.Name
			row.Values["amount"] = a.Amounts[i].String()
			if err := emit(row); err != nil {
				return err
			}
		}
	}
	return nil
}

// TaxColumns adalah kolom ekspor rekap pajak per masa.
var TaxColumns = []tabular.Column{
	{Key: "month", Header: "Masa", Kind: tabular.KindText},
	{Key: "pph21", Header: "PPh 21 Dipotong", Kind: tabular.KindDecimal},
	{Key: "pph23", Header: "PPh 23 Dipotong", Kind: tabular.KindDecimal},
	{Key: "pph42", Header: "PPh 4(2) Dipotong", Kind: tabular.KindDecimal},
	{Key: "paid", Header: "PPh Disetor", Kind: tabular.KindDecimal},
	{Key: "final_tax", Header: "PPh Final", Kind: tabular.KindDecimal},
	{Key: "turnover", Header: "Omzet Bruto", Kind: tabular.KindDecimal},
	{Key: "cumulative", Header: "Omzet Kumulatif", Kind: tabular.KindDecimal},
}

func taxYear(params map[string]string, def int) (int, error) {
	v := params["year"]
	if v == "" {
		return def, nil
	}
	y, err := strconv.Atoi(v)
	if err != nil || y < 2000 || y > 2100 {
		return 0, apperr.Validation(map[string]string{"year": "Tahun tidak valid"})
	}
	return y, nil
}

// ValidateTaxParams memeriksa parameter ekspor rekap pajak.
func (s *ReportService) ValidateTaxParams(params map[string]string) error {
	_, err := taxYear(params, clock.Today(s.Clock).Year())
	return err
}

// ExportTax mengalirkan rekap pajak 12 masa.
func (s *ReportService) ExportTax(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	y, err := taxYear(params, clock.Today(s.Clock).Year())
	if err != nil {
		return err
	}
	if _, err := actor.RequireAny(ctx, PermReportRead, "accounting.withholding.read"); err != nil {
		return err
	}
	r, err := s.TaxReport(ctx, y, 0)
	if err != nil {
		return err
	}
	for i, m := range r.Months {
		row := tabular.NewRow(i + 1)
		row.Values["month"] = m.Month.String()[:7]
		row.Values["pph21"] = m.PPh21Withheld.String()
		row.Values["pph23"] = m.PPh23Withheld.String()
		row.Values["pph42"] = m.PPh42Withheld.String()
		row.Values["paid"] = m.WithholdingPaid.String()
		row.Values["final_tax"] = m.FinalTax.String()
		row.Values["turnover"] = m.Turnover.String()
		row.Values["cumulative"] = m.Cumulative.String()
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}
