package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/accounting/domain"
	"balans/internal/accounting/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/platform/tabular"
)

// TrialBalance adalah Neraca Saldo satu rentang (ACC-18).
type TrialBalance struct {
	Filter postgres.TrialBalanceFilter
	Rows   []domain.TrialBalanceRow
	Totals domain.TrialBalanceTotals
}

func validateRange(from, to clock.BusinessDate) error {
	fields := map[string]string{}
	if from.IsZero() {
		fields["dateFrom"] = "Tanggal mulai wajib diisi"
	}
	if to.IsZero() {
		fields["dateTo"] = "Tanggal akhir wajib diisi"
	}
	if !from.IsZero() && !to.IsZero() {
		if to.Before(from) {
			fields["dateTo"] = "Tanggal akhir tidak boleh sebelum tanggal mulai"
		} else if from.AddDays(maxLedgerDays).Before(to) {
			fields["dateTo"] = "Rentang paling panjang 2 tahun"
		}
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// TrialBalance menghitung Neraca Saldo per akun: saldo awal, mutasi debit/kredit, dan saldo akhir untuk rentang
// tanggal dan dimensi; hanya jurnal terposting. Akun tanpa saldo dan mutasi tidak ditampilkan.
func (s *Service) TrialBalance(ctx context.Context, f postgres.TrialBalanceFilter) (TrialBalance, error) {
	if _, err := actor.RequireAny(ctx, PermReportRead, PermJournalRead); err != nil {
		return TrialBalance{}, err
	}
	if err := validateRange(f.DateFrom, f.DateTo); err != nil {
		return TrialBalance{}, err
	}
	rows, err := s.Store.TrialBalance(ctx, f)
	if err != nil {
		return TrialBalance{}, err
	}
	out := TrialBalance{Filter: f, Rows: make([]domain.TrialBalanceRow, 0, len(rows))}
	for _, r := range rows {
		if r.OpeningNet.IsZero() && r.PeriodDebit.IsZero() && r.PeriodCredit.IsZero() {
			continue
		}
		out.Rows = append(out.Rows, r)
	}
	out.Totals = domain.TrialBalanceTotalsOf(out.Rows)
	return out, nil
}

// --- Ekspor laporan -------------------------------------------------------------------------------

// TrialBalanceColumns adalah kolom ekspor Neraca Saldo.
var TrialBalanceColumns = []tabular.Column{
	{Key: "accountCode", Header: "Kode Akun", Kind: tabular.KindText},
	{Key: "accountName", Header: "Nama Akun", Kind: tabular.KindText},
	{Key: "group", Header: "Kelompok", Kind: tabular.KindText},
	{Key: "openingDebit", Header: "Saldo Awal Debit", Kind: tabular.KindDecimal},
	{Key: "openingCredit", Header: "Saldo Awal Kredit", Kind: tabular.KindDecimal},
	{Key: "periodDebit", Header: "Mutasi Debit", Kind: tabular.KindDecimal},
	{Key: "periodCredit", Header: "Mutasi Kredit", Kind: tabular.KindDecimal},
	{Key: "closingDebit", Header: "Saldo Akhir Debit", Kind: tabular.KindDecimal},
	{Key: "closingCredit", Header: "Saldo Akhir Kredit", Kind: tabular.KindDecimal},
}

// LedgerColumns adalah kolom ekspor Buku Besar detail.
var LedgerColumns = []tabular.Column{
	{Key: "accountCode", Header: "Kode Akun", Kind: tabular.KindText},
	{Key: "accountName", Header: "Nama Akun", Kind: tabular.KindText},
	{Key: "businessDate", Header: "Tanggal", Kind: tabular.KindDate},
	{Key: "number", Header: "Nomor Jurnal", Kind: tabular.KindText},
	{Key: "description", Header: "Keterangan", Kind: tabular.KindText},
	{Key: "source", Header: "Dokumen Sumber", Kind: tabular.KindText},
	{Key: "debit", Header: "Debit", Kind: tabular.KindDecimal},
	{Key: "credit", Header: "Kredit", Kind: tabular.KindDecimal},
	{Key: "channel", Header: "Channel", Kind: tabular.KindText},
	{Key: "subChannel", Header: "Sub-channel", Kind: tabular.KindText},
	{Key: "division", Header: "Divisi", Kind: tabular.KindText},
	{Key: "campaign", Header: "Kampanye", Kind: tabular.KindText},
	{Key: "memo", Header: "Keterangan Baris", Kind: tabular.KindText},
}

// maxLedgerExportRows membatasi ekspor Buku Besar (batas baris file impor/ekspor).
const maxLedgerExportRows = 20000

// ReportParams membaca parameter ekspor laporan: year+month (periode) atau dateFrom+dateTo, accountId, dan
// channelId/subChannelId/divisionId/campaignId. Tanpa rentang = bulan berjalan.
func (s *Service) reportParams(params map[string]string) (from, to clock.BusinessDate, account *uuid.UUID, dims domain.Dimensions, err error) {
	fields := map[string]string{}
	parseID := func(key string) *uuid.UUID {
		v := strings.TrimSpace(params[key])
		if v == "" {
			return nil
		}
		id, perr := uuid.Parse(v)
		if perr != nil {
			fields[key] = "ID tidak valid"
			return nil
		}
		return &id
	}
	parseDate := func(key string) clock.BusinessDate {
		v := strings.TrimSpace(params[key])
		if v == "" {
			return clock.BusinessDate{}
		}
		d, perr := clock.ParseBusinessDate(v)
		if perr != nil {
			fields[key] = "Format tanggal YYYY-MM-DD"
		}
		return d
	}
	from, to = parseDate("dateFrom"), parseDate("dateTo")
	if y, m := params["year"], params["month"]; y != "" && m != "" {
		var year, month int
		if _, perr := fmt.Sscanf(y+"-"+m, "%d-%d", &year, &month); perr != nil || month < 1 || month > 12 {
			fields["month"] = "Periode tidak valid"
		} else {
			from = clock.NewBusinessDate(year, time.Month(month), 1)
			to = clock.NewBusinessDate(year, time.Month(month)+1, 0)
		}
	}
	if from.IsZero() && to.IsZero() {
		today := clock.Today(s.Clock)
		from = clock.NewBusinessDate(today.Year(), today.Month(), 1)
		to = clock.NewBusinessDate(today.Year(), today.Month()+1, 0)
	}
	account = parseID("accountId")
	dims = domain.Dimensions{
		ChannelID: parseID("channelId"), SubChannelID: parseID("subChannelId"), DivisionID: parseID("divisionId"),
		CampaignID: parseID("campaignId"),
	}
	if len(fields) > 0 {
		return from, to, nil, dims, apperr.Validation(fields)
	}
	return from, to, account, dims, validateRange(from, to)
}

// ExportTrialBalance mengalirkan Neraca Saldo sesuai parameter ekspor.
func (s *Service) ExportTrialBalance(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	from, to, _, dims, err := s.reportParams(params)
	if err != nil {
		return err
	}
	tb, err := s.TrialBalance(ctx, postgres.TrialBalanceFilter{DateFrom: from, DateTo: to, Dimensions: dims})
	if err != nil {
		return err
	}
	write := func(r tabular.Row, od, oc, pd, pc, cd, cc money.Amount) error {
		r.Set("openingDebit", od.String()).Set("openingCredit", oc.String()).Set("periodDebit", pd.String())
		r.Set("periodCredit", pc.String()).Set("closingDebit", cd.String()).Set("closingCredit", cc.String())
		return emit(r)
	}
	for _, row := range tb.Rows {
		od, oc := domain.SplitNet(row.OpeningNet)
		cd, cc := domain.SplitNet(row.ClosingNet())
		r := tabular.NewRow(0).Set("accountCode", row.AccountCode).Set("accountName", row.AccountName).Set("group", row.Group.Label())
		if err := write(r, od, oc, row.PeriodDebit, row.PeriodCredit, cd, cc); err != nil {
			return err
		}
	}
	t := tb.Totals
	total := tabular.NewRow(0).Set("accountName", fmt.Sprintf("TOTAL %s s.d. %s", from, to))
	return write(total, t.OpeningDebit, t.OpeningCredit, t.PeriodDebit, t.PeriodCredit, t.ClosingDebit, t.ClosingCredit)
}

// ExportLedger mengalirkan Buku Besar detail (semua akun atau satu akun) sesuai parameter ekspor.
func (s *Service) ExportLedger(ctx context.Context, params map[string]string, emit func(tabular.Row) error) error {
	if _, err := actor.RequireAny(ctx, PermReportRead, PermJournalRead); err != nil {
		return err
	}
	from, to, account, dims, err := s.reportParams(params)
	if err != nil {
		return err
	}
	lines, err := s.Store.LedgerExportLines(ctx, postgres.LedgerExportFilter{
		AccountID: account, DateFrom: from, DateTo: to, Dimensions: dims, Limit: maxLedgerExportRows + 1,
	})
	if err != nil {
		return err
	}
	if len(lines) > maxLedgerExportRows {
		return apperr.WithMessage(apperr.CodeValidationFailed,
			fmt.Sprintf("Buku Besar berisi lebih dari %d baris. Persempit rentang tanggal atau pilih akun.", maxLedgerExportRows))
	}
	var ids []uuid.UUID
	for _, l := range lines {
		for _, id := range []*uuid.UUID{l.Dimensions.ChannelID, l.Dimensions.SubChannelID, l.Dimensions.DivisionID, l.Dimensions.CampaignID} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
	}
	names, err := s.Store.DimensionNames(ctx, ids)
	if err != nil {
		return err
	}
	code := func(id *uuid.UUID) string {
		if id == nil {
			return ""
		}
		return names[*id].Code
	}
	for _, l := range lines {
		r := tabular.NewRow(0).Set("accountCode", l.AccountCode).Set("accountName", l.AccountName)
		r.Set("businessDate", l.BusinessDate.String()).Set("number", l.Number).Set("description", l.Description)
		r.Set("source", strings.TrimSpace(l.SourceType+" "+l.SourceLabel)).Set("debit", l.Debit.String()).Set("credit", l.Credit.String())
		r.Set("channel", code(l.Dimensions.ChannelID)).Set("subChannel", code(l.Dimensions.SubChannelID))
		r.Set("division", code(l.Dimensions.DivisionID)).Set("campaign", code(l.Dimensions.CampaignID)).Set("memo", l.Memo)
		if err := emit(r); err != nil {
			return err
		}
	}
	return nil
}

// --- Impor Jurnal Umum ------------------------------------------------------------------------------

// JournalImportColumns adalah kolom template impor Jurnal Umum (S-413). Baris dengan Nomor Referensi yang sama
// di satu file menjadi satu draft Jurnal Umum; balance diperiksa saat draft diposting.
var JournalImportColumns = []tabular.Column{
	{Key: "reference", Header: "Nomor Referensi", Kind: tabular.KindText, Required: true, Example: "JU-001",
		Note: "Baris dengan nomor referensi sama di satu file menjadi satu jurnal."},
	{Key: "businessDate", Header: "Tanggal", Kind: tabular.KindDate, Required: true, Example: "2027-01-31"},
	{Key: "description", Header: "Keterangan", Kind: tabular.KindText, Required: true, Example: "Koreksi biaya sewa Januari"},
	{Key: "accountCode", Header: "Kode Akun", Kind: tabular.KindText, Required: true, Example: "6301"},
	{Key: "debit", Header: "Debit", Kind: tabular.KindDecimal, Example: "1500000", Note: "Isi salah satu Debit/Kredit."},
	{Key: "credit", Header: "Kredit", Kind: tabular.KindDecimal},
	{Key: "channelCode", Header: "Kode Channel", Kind: tabular.KindText, Example: "internal"},
	{Key: "subChannelCode", Header: "Kode Sub-channel", Kind: tabular.KindText},
	{Key: "divisionCode", Header: "Kode Divisi", Kind: tabular.KindText, Example: "FAT"},
	{Key: "campaignCode", Header: "Kode Kampanye", Kind: tabular.KindText},
	{Key: "memo", Header: "Keterangan Baris", Kind: tabular.KindText},
}

// dimensionByCode mencari ID dimensi dari kode (termasuk nonaktif; aktif/berlaku dicek saat validasi baris).
func (s *Service) dimensionByCode(ctx context.Context, kind domain.DimensionKind, code string) (*uuid.UUID, bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, true, nil
	}
	list, err := s.Store.ListDimensions(ctx, kind, postgres.DimensionFilter{IncludeInactive: true})
	if err != nil {
		return nil, false, err
	}
	for _, d := range list {
		if strings.EqualFold(d.Code, code) {
			id := d.ID
			return &id, true, nil
		}
	}
	return nil, false, nil
}

// importKey mengambil ID job impor dari kunci baris (`<job>:<baris>`), sehingga Nomor Referensi hanya
// menggabungkan baris di file yang sama.
func importKey(rowKey string) string {
	if i := strings.IndexByte(rowKey, ':'); i > 0 {
		return rowKey[:i]
	}
	return rowKey
}

// ImportJournalRow menambahkan satu baris ke draft Jurnal Umum hasil impor (dikelompokkan per Nomor Referensi).
// rowKey adalah kunci baris dataexchange (RowKey) yang menandai file impor.
func (s *Service) ImportJournalRow(ctx context.Context, rowKey string, row tabular.Row) error {
	a, err := actor.Require(ctx, PermJournalWrite)
	if err != nil {
		return err
	}
	p := tabular.Parse(row)
	ref := strings.TrimSpace(p.Text("reference"))
	if len(ref) > 60 {
		p.Fail("reference", "Nomor referensi paling panjang 60 karakter")
	}
	var date clock.BusinessDate
	if d := p.Date("businessDate"); d != nil {
		date = *d
	}
	description := strings.TrimSpace(p.Text("description"))
	line := domain.Line{Debit: parseAmount(p, "debit"), Credit: parseAmount(p, "credit"), Memo: p.Text("memo")}
	acc, ok, err := s.accountByCode(ctx, strings.TrimSpace(p.Text("accountCode")))
	if err != nil {
		return err
	}
	if !ok {
		p.Fail("accountCode", "Akun tidak ditemukan di Chart of Accounts")
	}
	line.AccountID = acc.ID
	for _, d := range []struct {
		kind domain.DimensionKind
		key  string
		dst  **uuid.UUID
	}{
		{domain.KindChannel, "channelCode", &line.Dimensions.ChannelID},
		{domain.KindSubChannel, "subChannelCode", &line.Dimensions.SubChannelID},
		{domain.KindDivision, "divisionCode", &line.Dimensions.DivisionID},
		{domain.KindCampaign, "campaignCode", &line.Dimensions.CampaignID},
	} {
		id, found, err := s.dimensionByCode(ctx, d.kind, p.Text(d.key))
		if err != nil {
			return err
		}
		if !found {
			p.Fail(d.key, "Kode tidak ditemukan di master dimensi")
		}
		*d.dst = id
	}
	if err := p.Err(); err != nil {
		return err
	}
	fields := domain.ValidateLines([]domain.Line{line, line})
	if msg := acc.ManualPostingError(); msg != "" {
		fields["lines.0.accountId"] = msg
	}
	if description == "" || len(description) > 500 {
		fields["description"] = "Keterangan jurnal wajib diisi (maks. 500 karakter)"
	}
	mapping := map[string]string{"lines.0.accountId": "accountCode", "lines.0.debit": "debit", "lines.0.memo": "memo",
		"lines.0.channelId": "channelCode", "lines.0.subChannelId": "subChannelCode", "lines.0.divisionId": "divisionCode",
		"lines.0.campaignId": "campaignCode"}
	delete(fields, "lines")
	if len(fields) > 0 {
		return remapFields(apperr.Validation(fields), mapping)
	}
	key := importKey(rowKey)
	return remapFields(s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.CheckPeriodOpen(ctx, date); err != nil {
			return err
		}
		dimErrs, err := s.checkDimensions(ctx, line.Dimensions, date, "lines.0.")
		if err != nil {
			return err
		}
		if len(dimErrs) > 0 {
			return apperr.Validation(dimErrs)
		}
		id, exists, err := s.Store.JournalImportRef(ctx, key, ref)
		if err != nil {
			return err
		}
		if !exists {
			e := domain.Entry{EntryType: domain.EntryManual, SourceType: domain.SourceManualJournal, Status: domain.EntryDraft,
				BusinessDate: date, Description: description}
			if e.ID, err = uuid.NewV7(); err != nil {
				return fmt.Errorf("new journal id: %w", err)
			}
			e.SourceID, e.SourceLabel = e.ID, "Impor "+ref
			line.LineNo, line.AccountCode, line.AccountName = 1, acc.Code, acc.Name
			e.Lines = []domain.Line{line}
			e.TotalAmount, _ = e.Totals()
			if err := s.Store.InsertEntry(ctx, e, a.UserID); err != nil {
				return err
			}
			if err := s.Store.InsertJournalImportRef(ctx, key, ref, e.ID); err != nil {
				return err
			}
			return s.Audit.Record(ctx, audit.Entry{
				Action: audit.ActionImport, EntityType: "journal_entry", EntityID: e.ID, EntityLabel: entryLabel(e), After: entrySnapshot(e),
			})
		}
		e, err := s.lockedEntry(ctx, id, 0)
		if err != nil {
			return err
		}
		if !e.BusinessDate.Equal(date) || e.Description != description {
			return apperr.Validation(map[string]string{"reference": "Tanggal dan keterangan harus sama untuk satu Nomor Referensi"})
		}
		if len(e.Lines) >= domain.MaxLines {
			return apperr.Validation(map[string]string{"reference": fmt.Sprintf("Satu jurnal paling banyak %d baris", domain.MaxLines)})
		}
		line.LineNo, line.AccountCode, line.AccountName = len(e.Lines)+1, acc.Code, acc.Name
		e.Lines = append(e.Lines, line)
		e.TotalAmount, _ = e.Totals()
		return s.Store.ReplaceDraft(ctx, e, e.Version, a.UserID)
	}), mapping)
}
