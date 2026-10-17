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
	"balans/internal/platform/money"
)

func jline(debit, credit string) domain.Line {
	return domain.Line{AccountID: uuid.New(), Debit: money.MustParse(debit), Credit: money.MustParse(credit)}
}

func TestValidateLines(t *testing.T) {
	tests := []struct {
		name  string
		lines []domain.Line
		want  []string
	}{
		{"dua baris valid", []domain.Line{jline("10", "0"), jline("0", "10")}, nil},
		{"satu baris", []domain.Line{jline("10", "0")}, []string{"lines"}},
		{"debit dan kredit sekaligus", []domain.Line{jline("10", "10"), jline("0", "10")}, []string{"lines.0.debit"}},
		{"keduanya nol", []domain.Line{jline("0", "0"), jline("0", "10")}, []string{"lines.0.debit"}},
		{"negatif", []domain.Line{jline("-10", "0"), jline("0", "10")}, []string{"lines.0.debit"}},
		{"tiga desimal", []domain.Line{jline("10", "0"), jline("0", "10.001")}, []string{"lines.1.debit"}},
		{"akun kosong", []domain.Line{{Debit: money.FromInt(1)}, jline("0", "1")}, []string{"lines.0.accountId"}},
		{"memo terlalu panjang", []domain.Line{{AccountID: uuid.New(), Debit: money.FromInt(1), Memo: string(make([]byte, 501))}, jline("0", "1")}, []string{"lines.0.memo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := domain.ValidateLines(tt.lines)
			keys := make([]string, 0, len(fields))
			for k := range fields {
				keys = append(keys, k)
			}
			assert.ElementsMatch(t, tt.want, keys)
		})
	}
	many := make([]domain.Line, domain.MaxLines+1)
	for i := range many {
		many[i] = jline("1", "0")
	}
	assert.Contains(t, domain.ValidateLines(many), "lines")
}

func manual(t *testing.T, lines ...domain.Line) domain.Entry {
	t.Helper()
	e, err := domain.NewManual(domain.ManualInput{BusinessDate: date, Description: "  Koreksi biaya  ", Lines: lines})
	require.NoError(t, err)
	return e
}

func TestManualJournalLifecycle(t *testing.T) {
	e := manual(t, jline("100", "0"), jline("0", "60"), jline("0", "40"))
	assert.Equal(t, domain.EntryDraft, e.Status)
	assert.Equal(t, domain.EntryManual, e.EntryType)
	assert.Equal(t, "Koreksi biaya", e.Description)
	assert.Equal(t, []int{1, 2, 3}, []int{e.Lines[0].LineNo, e.Lines[1].LineNo, e.Lines[2].LineNo})
	assert.Equal(t, "100.00", e.TotalAmount.String())

	all := func(string) bool { return true }
	none := func(string) bool { return false }
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionDelete, domain.ActionPost}, e.AllowedActions(all))
	assert.Empty(t, e.AllowedActions(none))

	require.NoError(t, e.UpdateManual(domain.ManualInput{BusinessDate: date, Description: "Ubah", Lines: []domain.Line{jline("5", "0"), jline("0", "5")}}))
	assert.Equal(t, "5.00", e.TotalAmount.String())

	now := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	by, period := uuid.New(), uuid.New()
	require.NoError(t, e.Post("JV/2026/10/00001", period, now, by))
	assert.True(t, e.IsPosted())
	assert.Equal(t, "JV/2026/10/00001", e.Number)
	assert.Equal(t, &period, e.FiscalPeriodID)

	// Jurnal terposting tidak bisa diubah, dihapus, atau diposting ulang (ACC-13).
	assert.True(t, apperr.HasCode(e.CheckEditable(), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(e.UpdateManual(domain.ManualInput{}), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(e.Post("x", period, now, by), apperr.CodeInvalidStateTransition))
	assert.Equal(t, []domain.Action{domain.ActionReverse}, e.AllowedActions(all))
}

func TestManualValidation(t *testing.T) {
	_, err := domain.NewManual(domain.ManualInput{Lines: []domain.Line{jline("1", "0")}})
	ae := apperr.As(err)
	assert.Contains(t, ae.Fields, "businessDate")
	assert.Contains(t, ae.Fields, "description")
	assert.Contains(t, ae.Fields, "lines")

	_, err = domain.NewManual(domain.ManualInput{BusinessDate: date, Description: string(make([]byte, 501)), Lines: []domain.Line{jline("1", "0"), jline("0", "1")}})
	assert.Contains(t, apperr.As(err).Fields, "description")
}

func TestPostRejectsUnbalanced(t *testing.T) {
	tests := []struct {
		name  string
		lines []domain.Line
	}{
		{"debit lebih besar", []domain.Line{jline("100", "0"), jline("0", "99.99")}},
		{"kredit lebih besar", []domain.Line{jline("1", "0"), jline("0", "2")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := manual(t, tt.lines...)
			err := e.Post("JV/1", uuid.New(), time.Now(), uuid.New())
			assert.Contains(t, apperr.As(err).Fields, "lines")
			assert.Equal(t, domain.EntryDraft, e.Status)
		})
	}
	zero := domain.Entry{Status: domain.EntryDraft}
	assert.Contains(t, apperr.As(zero.CheckBalanced()).Fields, "lines")
}

func TestReversal(t *testing.T) {
	e := manual(t, jline("100", "0"), jline("0", "100"))
	e.ID = uuid.New()
	e.Lines[0].Dimensions.DivisionID = ptr(divisionID)

	_, err := e.NewReversal(date, "salah akun")
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "draft tidak bisa dibalik")

	require.NoError(t, e.Post("JV/2026/10/00001", uuid.New(), time.Now(), uuid.New()))
	r, err := e.NewReversal(date.AddDays(3), "  salah akun ")
	require.NoError(t, err)
	assert.Equal(t, domain.EntryReversal, r.EntryType)
	assert.Equal(t, domain.SourceJournalReversal, r.SourceType)
	assert.Equal(t, e.ID, r.SourceID)
	assert.Equal(t, &e.ID, r.ReversalOfID)
	assert.Equal(t, "Balik Jurnal JV/2026/10/00001: salah akun", r.Description)
	assert.Equal(t, "100.00", r.Lines[0].Credit.String())
	assert.True(t, r.Lines[0].Debit.IsZero())
	assert.Equal(t, "100.00", r.Lines[1].Debit.String())
	assert.Equal(t, &divisionID, r.Lines[0].Dimensions.DivisionID, "dimensi ikut dibalik")
	require.NoError(t, r.CheckBalanced())

	_, err = e.NewReversal(date.AddDays(-1), "")
	fields := apperr.As(err).Fields
	assert.Contains(t, fields, "reason")
	assert.Contains(t, fields, "businessDate")
	_, err = e.NewReversal(clock.BusinessDate{}, string(make([]byte, 401)))
	assert.Contains(t, apperr.As(err).Fields, "reason")

	// Sudah dibalik.
	reversedBy := uuid.New()
	e.ReversedByID, e.ReversedByNumber = &reversedBy, "JV/2026/10/00002"
	_, err = e.NewReversal(date, "lagi")
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
	assert.Empty(t, e.AllowedActions(func(string) bool { return true }))

	// Jurnal pembalik tidak bisa dibalik lagi.
	r.Status = domain.EntryPosted
	assert.True(t, apperr.HasCode(r.CheckReversible(), apperr.CodeInvalidStateTransition))

	// Jurnal otomatis tidak bisa diubah dari halaman jurnal.
	auto := domain.Entry{Status: domain.EntryDraft, EntryType: domain.EntryAutomatic}
	assert.True(t, apperr.HasCode(auto.CheckEditable(), apperr.CodeInvalidStateTransition))
}
