package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func day(d int) clock.BusinessDate { return clock.NewBusinessDate(2026, time.October, d) }

func TestParseStatementRowsDebitCredit(t *testing.T) {
	rows := [][]string{
		{"Laporan Mutasi Rekening"},
		{"No. Rekening", "1234567890"},
		{},
		{"Tanggal", "Keterangan", "No. Referensi", "Mutasi Debet", "Mutasi Kredit", "Saldo"},
		{"", "SALDO AWAL", "", "", "", "10.000.000,00"},
		{"01/10/2026", "TRSF E-BANKING CR INV/2026/10/00031", "FT26274001", "", "1.500.000,00", "11.500.000,00"},
		{"02/10/2026", "BIAYA ADM", "", "15.000,00", "", "11.485.000,00"},
		{"3 Okt 2026", "TRF KE VENDOR", "FT26276002", "(2.000.000)", "", ""},
		{"2026-10-04", "BUNGA", "", "", "1,234.56", ""},
		{"45934", "SERI EXCEL", "", "", "100", ""},
		{"05/10/2026", "NOL", "", "", "0", ""},
		{"Total", "", "", "2.015.000,00", "1.501.334,56", ""},
	}
	got, err := domain.ParseStatementRows(rows)
	require.NoError(t, err)
	require.Len(t, got.Rows, 5)
	assert.Equal(t, 3, got.Skipped)

	assert.Equal(t, "2026-10-01", got.Rows[0].Date.String())
	assert.Equal(t, "1500000.00", got.Rows[0].Amount.String())
	assert.Equal(t, "FT26274001", got.Rows[0].Reference)
	require.NotNil(t, got.Rows[0].Balance)
	assert.Equal(t, "11500000.00", got.Rows[0].Balance.String())
	assert.Equal(t, 6, got.Rows[0].Row)

	assert.Equal(t, "-15000.00", got.Rows[1].Amount.String())
	assert.Equal(t, "2026-10-03", got.Rows[2].Date.String())
	assert.Equal(t, "-2000000.00", got.Rows[2].Amount.String(), "debit dalam kurung tetap keluar")
	assert.Equal(t, "1234.56", got.Rows[3].Amount.String())
	assert.Equal(t, "2025-10-04", got.Rows[4].Date.String(), "seri tanggal Excel")

	sum := domain.NewStatementSummary(got.Rows)
	assert.Equal(t, "2025-10-04", sum.PeriodFrom.String())
	assert.Equal(t, "2026-10-04", sum.PeriodTo.String())
	assert.Equal(t, 5, sum.LineCount)
	assert.Equal(t, "1501334.56", sum.TotalIn.String())
	assert.Equal(t, "2015000.00", sum.TotalOut.String())
}

func TestParseStatementRowsAmountWithIndicator(t *testing.T) {
	rows := [][]string{
		{"Date", "Description", "Amount", "DB/CR", "Balance"},
		{"19/10/2026", "TRANSFER IN", "Rp 2.500.000", "CR", "5,000,000.00"},
		{"19/10/2026", "TRANSFER OUT", "750,000.00", "DB", ""},
		{"20/10/2026", "NO INDICATOR", "-125.000", "", ""},
		{"21/10/2026", "SUFFIX", "1,000.00 CR", "", ""},
		{"22/10/2026", "SUFFIX NOSPACE", "300.00DB", "", ""},
		{"23/10/2026", "PLUS", "+5000", "K", ""},
	}
	got, err := domain.ParseStatementRows(rows)
	require.NoError(t, err)
	require.Len(t, got.Rows, 6)
	want := []string{"2500000.00", "-750000.00", "-125000.00", "1000.00", "-300.00", "5000.00"}
	for i, w := range want {
		assert.Equal(t, w, got.Rows[i].Amount.String(), got.Rows[i].Description)
	}
	assert.Empty(t, got.Rows[0].Reference)
}

func TestParseStatementRowsErrors(t *testing.T) {
	_, err := domain.ParseStatementRows([][]string{{"a", "b"}, {"1", "2"}})
	assert.Contains(t, fieldsOf(t, err), "fileId")

	_, err = domain.ParseStatementRows([][]string{{"Tanggal", "Debit", "Kredit"}, {"01/10/2026", "abc", ""}})
	var ae *apperr.Error
	require.ErrorAs(t, err, &ae)
	assert.Contains(t, ae.Fields["fileId"], "Baris 2")

	_, err = domain.ParseStatementRows([][]string{{"Tanggal", "Debit", "Kredit"}, {"01/10/2026", "", "1.234,567"}})
	assert.Error(t, err)

	_, err = domain.ParseStatementRows([][]string{{"Tanggal", "Debit", "Kredit"}, {"bukan tanggal", "1", ""}})
	assert.Contains(t, fieldsOf(t, err), "fileId", "tanpa baris transaksi")

	_, err = domain.ParseStatementRows([][]string{{"Tanggal", "Nominal"}, {"01/10/2026", "Rp"}})
	assert.Error(t, err)

	big := [][]string{{"Tanggal", "Nominal"}}
	for i := 0; i <= domain.MaxStatementLines; i++ {
		big = append(big, []string{"01/10/2026", "1"})
	}
	_, err = domain.ParseStatementRows(big)
	assert.Error(t, err)

	// Tanggal di luar 2000–2099 dan seri Excel di luar rentang dilewati.
	got, err := domain.ParseStatementRows([][]string{{"Tanggal", "Nominal"}, {"01/10/1999", "1"}, {"12", "1"}, {"'01/10/2026", "2"}})
	require.NoError(t, err)
	assert.Len(t, got.Rows, 1)
	assert.Equal(t, 2, got.Skipped)
}

func stLine(n, d int, amount int64, ref, desc string) domain.StatementLine {
	return domain.StatementLine{
		ID: uuid.New(), LineNo: n, TransactionDate: day(d), Amount: rp(amount), Reference: ref, Description: desc,
		Status: domain.LineUnmatched,
	}
}

func jCand(d int, amount int64, refs ...string) domain.MatchCandidate {
	return domain.MatchCandidate{
		Kind: domain.MatchJournalLine, JournalLineID: uuid.New(), JournalEntryID: uuid.New(), Date: day(d), Amount: rp(amount),
		References: refs, Label: "JV",
	}
}

func dCand(d int, amount int64, refs ...string) domain.MatchCandidate {
	return domain.MatchCandidate{
		Kind: domain.MatchDocument, Source: "sales.invoice", DocumentID: uuid.New(), Date: day(d), Amount: rp(amount),
		References: refs, Label: "INV",
	}
}

func TestSuggestMatches(t *testing.T) {
	byRef := stLine(1, 5, 1_500_000, "", "TRSF CR INV/2026/10/00031 PT ABC")
	byRefExact := stLine(2, 6, -250_000, "BYR/2026/10/00007", "")
	byAmount := stLine(3, 7, -15_000, "", "BIAYA ADM")
	tie := stLine(4, 8, 99_000, "", "")
	none := stLine(5, 9, 77_000, "", "")
	preferJournal := stLine(6, 10, 500_000, "", "")

	invoice := dCand(1, 1_500_000, "INV/2026/10/00031")
	otherSame := jCand(5, 1_500_000) // nominal sama tapi tanpa referensi: kalah dari referensi
	payment := jCand(2, -250_000, "BYR/2026/10/00007")
	fee := jCand(8, -15_000) // selisih 1 hari
	feeFar := jCand(20, -15_000)
	tieA, tieB := jCand(7, 99_000), jCand(9, 99_000)
	docNear := dCand(10, 500_000)
	jNear := jCand(10, 500_000)
	shortRef := dCand(25, 77_000, "AB") // referensi pendek tidak dicari di keterangan

	got := domain.SuggestMatches(
		[]domain.StatementLine{none, tie, byAmount, byRefExact, byRef, preferJournal},
		[]domain.MatchCandidate{otherSame, invoice, payment, feeFar, fee, tieA, tieB, docNear, jNear, shortRef},
		domain.DefaultMatchToleranceDays,
	)
	byLine := map[uuid.UUID]domain.Suggestion{}
	for _, s := range got {
		byLine[s.LineID] = s
	}
	require.Contains(t, byLine, byRef.ID)
	assert.Equal(t, domain.RuleReference, byLine[byRef.ID].Rule)
	assert.Equal(t, invoice.DocumentID, byLine[byRef.ID].Candidate.DocumentID)

	require.Contains(t, byLine, byRefExact.ID)
	assert.Equal(t, domain.RuleReference, byLine[byRefExact.ID].Rule)
	assert.Equal(t, payment.JournalLineID, byLine[byRefExact.ID].Candidate.JournalLineID)

	require.Contains(t, byLine, byAmount.ID)
	assert.Equal(t, domain.RuleAmountDate, byLine[byAmount.ID].Rule)
	assert.Equal(t, fee.JournalLineID, byLine[byAmount.ID].Candidate.JournalLineID)

	assert.NotContains(t, byLine, tie.ID, "dua kandidat sama kuat → manual")
	assert.NotContains(t, byLine, none.ID)

	require.Contains(t, byLine, preferJournal.ID)
	assert.Equal(t, domain.MatchJournalLine, byLine[preferJournal.ID].Candidate.Kind)
}

func TestSuggestMatchesUsesCandidateOnce(t *testing.T) {
	a, b := stLine(1, 5, 100, "", ""), stLine(2, 5, 100, "", "")
	c := jCand(5, 100)
	got := domain.SuggestMatches([]domain.StatementLine{a, b}, []domain.MatchCandidate{c}, 3)
	require.Len(t, got, 1)
	assert.Equal(t, a.ID, got[0].LineID, "baris lebih awal (nomor baris) didahulukan")
}

func TestStatementLineTransitions(t *testing.T) {
	l := stLine(1, 5, 100, "", "")
	c := jCand(5, 100)
	require.NoError(t, l.Suggest(c, domain.RuleAmountDate))
	assert.Equal(t, domain.LineSuggested, l.Status)
	assert.Equal(t, c.JournalLineID, *l.JournalLineID)

	l.ClearSuggestion()
	assert.Equal(t, domain.LineUnmatched, l.Status)
	assert.Nil(t, l.JournalLineID)
	l.ClearSuggestion() // tidak berubah bila tidak ada usulan

	d := dCand(5, 100)
	require.NoError(t, l.Suggest(d, domain.RuleReference))
	assert.Equal(t, d.Source, l.CandidateSource)
	assert.Equal(t, d.DocumentID, *l.CandidateDocID)

	jl, je, pay := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, l.Confirm(domain.MatchDocument, domain.RuleReference, jl, je, &pay, "INV · BYR", time.Now(), uuid.New()))
	assert.Equal(t, domain.LineMatched, l.Status)
	assert.Equal(t, "INV · BYR", l.CandidateLabel)
	assert.True(t, apperr.HasCode(l.Suggest(c, domain.RuleManual), apperr.CodeInvalidStateTransition))
	assert.Error(t, l.Confirm(domain.MatchJournalLine, domain.RuleManual, jl, je, nil, "", time.Now(), uuid.New()))

	require.NoError(t, l.Unmatch())
	assert.Equal(t, domain.LineUnmatched, l.Status)
	assert.Nil(t, l.PaymentID)
	assert.Error(t, l.Unmatch())

	require.NoError(t, l.Confirm(domain.MatchNewEntry, domain.RuleManual, jl, je, &pay, "", time.Now(), uuid.New()))
	assert.Empty(t, l.CandidateSource)
}
