package domain

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// --- Rekonsiliasi bank (S-402, Technical Guideline 5.8.5) -------------------------------------

// LineStatus adalah status baris mutasi bank.
type LineStatus string

const (
	LineUnmatched LineStatus = "unmatched"
	LineSuggested LineStatus = "suggested"
	LineMatched   LineStatus = "matched"
)

// MatchKind adalah jenis pasangan baris mutasi.
type MatchKind string

const (
	// MatchJournalLine: transaksi yang sudah dijurnal di akun rekening.
	MatchJournalLine MatchKind = "journal_line"
	// MatchDocument: dokumen terbuka dari penyedia kandidat (piutang, utang, pengajuan biaya) yang dibayar saat
	// usulan dikonfirmasi.
	MatchDocument MatchKind = "document"
	// MatchNewEntry: jurnal baru dibuat dari baris yang tidak cocok (biaya bank, bunga, dll.).
	MatchNewEntry MatchKind = "new_entry"
)

// MatchRule adalah aturan pencocokan yang menghasilkan pasangan.
type MatchRule string

const (
	RuleReference  MatchRule = "reference"
	RuleAmountDate MatchRule = "amount_date"
	RuleManual     MatchRule = "manual"
)

// DefaultMatchToleranceDays adalah selisih tanggal maksimal aturan nominal + tanggal.
// TODO(setting): pindahkan ke settings `bank_reconciliation.tolerance_days`.
const DefaultMatchToleranceDays = 3

// MaxStatementLines membatasi jumlah baris satu file mutasi.
const MaxStatementLines = 5000

// StatementRow adalah satu baris hasil baca file mutasi.
type StatementRow struct {
	// Row adalah nomor baris di file (1 = baris pertama) untuk pesan error.
	Row         int
	Date        clock.BusinessDate
	Description string
	Reference   string
	// Amount bertanda: positif = dana masuk (kredit rekening koran), negatif = keluar.
	Amount  money.Amount
	Balance *money.Amount
}

// ParsedStatement adalah hasil ParseStatementRows.
type ParsedStatement struct {
	Rows []StatementRow
	// Skipped adalah jumlah baris non-transaksi yang dilewati (judul, saldo awal, total).
	Skipped int
}

type columnRole int

const (
	colNone columnRole = iota
	colDate
	colDescription
	colReference
	colDebit
	colCredit
	colAmount
	colDC
	colBalance
)

var headerAliases = map[columnRole][]string{
	colDate: {"tanggal", "tgl", "date", "tanggal transaksi", "tgl transaksi", "transaction date", "posting date",
		"tanggal mutasi", "tgl mutasi", "trx date", "value date", "tanggal valuta"},
	colDescription: {"keterangan", "deskripsi", "description", "uraian", "remark", "remarks", "uraian transaksi",
		"keterangan transaksi", "transaction description", "berita"},
	colReference: {"referensi", "reference", "no referensi", "no ref", "ref", "nomor referensi", "reference no",
		"ref no", "kode referensi", "no transaksi", "nomor transaksi", "transaction id", "cheque no"},
	colDebit:   {"debit", "debet", "mutasi debit", "mutasi debet", "keluar", "withdrawal", "withdrawals", "db", "dana keluar"},
	colCredit:  {"kredit", "credit", "mutasi kredit", "masuk", "deposit", "deposits", "cr", "dana masuk"},
	colAmount:  {"nominal", "jumlah", "amount", "mutasi", "nilai"},
	colDC:      {"db cr", "d k", "dk", "dc", "d c", "cr db", "jenis", "jenis mutasi", "type"},
	colBalance: {"saldo", "balance", "saldo akhir", "running balance"},
}

func normalizeHeader(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func roleOf(header string) columnRole {
	h := normalizeHeader(header)
	if h == "" {
		return colNone
	}
	for role := colDate; role <= colBalance; role++ {
		if slices.Contains(headerAliases[role], h) {
			return role
		}
	}
	return colNone
}

// detectHeader mencari baris judul kolom pada 20 baris pertama: wajib ada tanggal dan nominal (debit/kredit
// atau nominal).
func detectHeader(rows [][]string) (int, map[columnRole]int, bool) {
	for i := 0; i < len(rows) && i < 20; i++ {
		cols := map[columnRole]int{}
		for j, cell := range rows[i] {
			if r := roleOf(cell); r != colNone {
				if _, dup := cols[r]; !dup {
					cols[r] = j
				}
			}
		}
		_, hasDate := cols[colDate]
		_, hasDebit := cols[colDebit]
		_, hasCredit := cols[colCredit]
		_, hasAmount := cols[colAmount]
		if hasDate && ((hasDebit && hasCredit) || hasAmount) {
			return i, cols, true
		}
	}
	return 0, nil, false
}

func cell(row []string, cols map[columnRole]int, role columnRole) string {
	j, ok := cols[role]
	if !ok || j >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[j])
}

// ParseStatementRows membaca baris file mutasi (CSV atau sel Excel mentah). Baris judul kolom dikenali dari
// nama kolom umum bank Indonesia (Tanggal, Keterangan, Referensi, Debit/Mutasi Debit, Kredit/Mutasi Kredit,
// atau Nominal + D/K, Saldo). Baris tanpa tanggal valid atau bernominal nol dilewati (judul, saldo awal, total);
// baris bertanggal dengan nominal tidak valid ditolak dengan nomor barisnya.
func ParseStatementRows(rows [][]string) (ParsedStatement, error) {
	hdr, cols, ok := detectHeader(rows)
	if !ok {
		return ParsedStatement{}, apperr.Validation(map[string]string{
			"fileId": "Kolom Tanggal dan Debit/Kredit (atau Nominal) tidak ditemukan di file mutasi",
		})
	}
	var out ParsedStatement
	for i := hdr + 1; i < len(rows); i++ {
		row := rows[i]
		if isBlank(row) {
			continue
		}
		date, ok := parseStatementDate(cell(row, cols, colDate))
		if !ok {
			out.Skipped++
			continue
		}
		amount, err := rowAmount(row, cols)
		if err != nil {
			return ParsedStatement{}, apperr.Validation(map[string]string{
				"fileId": fmt.Sprintf("Baris %d: %s", i+1, err.Error()),
			})
		}
		if amount.IsZero() {
			out.Skipped++
			continue
		}
		r := StatementRow{
			Row: i + 1, Date: date, Amount: amount,
			Description: truncate(cell(row, cols, colDescription), 500), Reference: truncate(cell(row, cols, colReference), 100),
		}
		if s := cell(row, cols, colBalance); s != "" {
			if b, _, err := parseStatementAmount(s); err == nil {
				r.Balance = &b
			}
		}
		out.Rows = append(out.Rows, r)
		if len(out.Rows) > MaxStatementLines {
			return ParsedStatement{}, apperr.Validation(map[string]string{
				"fileId": fmt.Sprintf("File mutasi maksimal %d baris transaksi", MaxStatementLines),
			})
		}
	}
	if len(out.Rows) == 0 {
		return ParsedStatement{}, apperr.Validation(map[string]string{"fileId": "File mutasi tidak berisi transaksi"})
	}
	return out, nil
}

func absAmount(a money.Amount) money.Amount {
	if a.IsNegative() {
		return a.Neg()
	}
	return a
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func isBlank(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func rowAmount(row []string, cols map[columnRole]int) (money.Amount, error) {
	debitS, creditS := cell(row, cols, colDebit), cell(row, cols, colCredit)
	if _, ok := cols[colDebit]; ok && (debitS != "" || creditS != "") {
		debit, credit := money.Zero, money.Zero
		var err error
		if debitS != "" {
			if debit, _, err = parseStatementAmount(debitS); err != nil {
				return money.Zero, err
			}
		}
		if creditS != "" {
			if credit, _, err = parseStatementAmount(creditS); err != nil {
				return money.Zero, err
			}
		}
		return absAmount(credit).Sub(absAmount(debit)), nil
	}
	s := cell(row, cols, colAmount)
	if s == "" {
		return money.Zero, nil
	}
	a, dc, err := parseStatementAmount(s)
	if err != nil {
		return money.Zero, err
	}
	if ind := strings.ToUpper(normalizeHeader(cell(row, cols, colDC))); ind != "" {
		dc = ind
	}
	switch dc {
	case "D", "DB", "DEBIT", "DEBET", "K-", "KELUAR":
		return absAmount(a).Neg(), nil
	case "K", "CR", "C", "KREDIT", "CREDIT", "MASUK":
		return absAmount(a), nil
	}
	return a, nil
}

// parseStatementAmount membaca nominal dengan format Indonesia (1.234.567,89) atau internasional
// (1,234,567.89), awalan Rp, tanda minus/kurung, dan akhiran CR/DB. Mengembalikan indikator D/K akhiran.
func parseStatementAmount(s string) (money.Amount, string, error) {
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	dc := ""
	if f := strings.Fields(upper); len(f) > 1 && slices.Contains([]string{"CR", "DB", "D", "K", "C"}, f[len(f)-1]) {
		dc, upper = f[len(f)-1], strings.Join(f[:len(f)-1], " ")
	} else if n := len(upper); n > 2 && (strings.HasSuffix(upper, "CR") || strings.HasSuffix(upper, "DB")) &&
		upper[n-3] >= '0' && upper[n-3] <= '9' {
		dc, upper = upper[n-2:], upper[:n-2]
	}
	neg := false
	if strings.HasPrefix(upper, "(") && strings.HasSuffix(upper, ")") {
		neg, upper = true, upper[1:len(upper)-1]
	}
	upper = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(upper), "RP"))
	upper = strings.TrimPrefix(upper, ".")
	upper = strings.ReplaceAll(upper, " ", "")
	if strings.HasPrefix(upper, "-") {
		neg, upper = !neg, upper[1:]
	} else if strings.HasPrefix(upper, "+") {
		upper = upper[1:]
	}
	if upper == "" {
		return money.Zero, dc, fmt.Errorf("nominal %q tidak valid", s)
	}
	normalized := normalizeNumber(upper)
	d, err := decimal.NewFromString(normalized)
	if err != nil {
		return money.Zero, dc, fmt.Errorf("nominal %q tidak valid", s)
	}
	if d.Exponent() < -2 && !d.Equal(d.Round(2)) {
		return money.Zero, dc, fmt.Errorf("nominal %q lebih dari 2 angka di belakang koma", s)
	}
	a := money.FromDecimal(d.Round(2))
	if neg {
		a = a.Neg()
	}
	return a, dc, nil
}

// normalizeNumber mengubah pemisah ribuan/desimal menjadi format titik desimal.
func normalizeNumber(s string) string {
	dot, comma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	switch {
	case dot >= 0 && comma >= 0:
		if comma > dot {
			return strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
		}
		return strings.ReplaceAll(s, ",", "")
	case comma >= 0:
		if strings.Count(s, ",") == 1 && len(s)-comma-1 <= 2 {
			return strings.Replace(s, ",", ".", 1)
		}
		return strings.ReplaceAll(s, ",", "")
	case dot >= 0:
		if strings.Count(s, ".") == 1 && len(s)-dot-1 <= 2 {
			return s
		}
		return strings.ReplaceAll(s, ".", "")
	}
	return s
}

var idMonths = strings.NewReplacer(
	"Januari", "Jan", "Februari", "Feb", "Maret", "Mar", "April", "Apr", "Juni", "Jun", "Juli", "Jul",
	"Agustus", "Aug", "September", "Sep", "Oktober", "Oct", "November", "Nov", "Desember", "Dec",
	"Mei", "May", "Agu", "Aug", "Ags", "Aug", "Okt", "Oct", "Des", "Dec",
)

var statementDateLayouts = []string{
	"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006", "02/01/06", "2/1/06", "02-01-06",
	"2006/01/02", "02 Jan 2006", "2 Jan 2006", "02-Jan-2006", "2-Jan-2006", "02 Jan 06", "02-Jan-06",
	"02.01.2006", "2006-01-02 15:04:05", "02/01/2006 15:04:05", "02/01/2006 15:04", "02/01",
}

// parseStatementDate membaca tanggal mutasi: format umum bank Indonesia (DD/MM/YYYY, nama bulan Indonesia)
// atau nomor seri tanggal Excel.
func parseStatementDate(s string) (clock.BusinessDate, bool) {
	s = strings.TrimSpace(strings.Trim(s, "'"))
	if s == "" {
		return clock.BusinessDate{}, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		// Seri tanggal Excel (1900 date system): 25569 = 1970-01-01. Batasi 2000–2099.
		if f >= 36526 && f < 73051 {
			days := int(math.Floor(f))
			t := time.Date(1899, time.December, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
			return clock.NewBusinessDate(t.Year(), t.Month(), t.Day()), true
		}
		return clock.BusinessDate{}, false
	}
	s = idMonths.Replace(s)
	for _, layout := range statementDateLayouts {
		if layout == "02/01" {
			continue // tanpa tahun tidak didukung
		}
		if t, err := time.Parse(layout, s); err == nil {
			if t.Year() < 2000 || t.Year() > 2099 {
				return clock.BusinessDate{}, false
			}
			return clock.NewBusinessDate(t.Year(), t.Month(), t.Day()), true
		}
	}
	return clock.BusinessDate{}, false
}

// StatementLine adalah baris mutasi bank tersimpan beserta usulan/hasil pencocokan.
type StatementLine struct {
	ID              uuid.UUID
	StatementID     uuid.UUID
	BankAccountID   uuid.UUID
	LineNo          int
	TransactionDate clock.BusinessDate
	Description     string
	Reference       string
	Amount          money.Amount
	Balance         *money.Amount
	Status          LineStatus
	MatchKind       MatchKind
	MatchRule       MatchRule
	JournalLineID   *uuid.UUID
	JournalEntryID  *uuid.UUID
	JournalNumber   string
	CandidateSource string
	CandidateDocID  *uuid.UUID
	CandidateLabel  string
	PaymentID       *uuid.UUID
	MatchedAt       *time.Time
	MatchedBy       *uuid.UUID
	Version         int
}

// Suggest menyimpan usulan pasangan (belum dikonfirmasi).
func (l *StatementLine) Suggest(c MatchCandidate, rule MatchRule) error {
	if l.Status == LineMatched {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Baris mutasi sudah direkonsiliasi.")
	}
	l.Status, l.MatchKind, l.MatchRule = LineSuggested, c.Kind, rule
	l.JournalLineID, l.JournalEntryID, l.JournalNumber = nil, nil, ""
	l.CandidateSource, l.CandidateDocID, l.CandidateLabel = "", nil, c.Label
	if c.Kind == MatchJournalLine {
		l.JournalLineID, l.JournalEntryID = ptrUUID(c.JournalLineID), ptrUUID(c.JournalEntryID)
	} else {
		l.CandidateSource, l.CandidateDocID = c.Source, ptrUUID(c.DocumentID)
	}
	return nil
}

func ptrUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

// ClearSuggestion menghapus usulan baris yang belum dikonfirmasi.
func (l *StatementLine) ClearSuggestion() {
	if l.Status != LineSuggested {
		return
	}
	l.Status, l.MatchKind, l.MatchRule = LineUnmatched, "", ""
	l.JournalLineID, l.JournalEntryID, l.JournalNumber = nil, nil, ""
	l.CandidateSource, l.CandidateDocID, l.CandidateLabel = "", nil, ""
}

// Confirm menandai baris direkonsiliasi dengan baris jurnal akun rekening.
func (l *StatementLine) Confirm(kind MatchKind, rule MatchRule, journalLineID, journalEntryID uuid.UUID, paymentID *uuid.UUID,
	label string, at time.Time, by uuid.UUID,
) error {
	if l.Status == LineMatched {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Baris mutasi sudah direkonsiliasi.")
	}
	if kind != MatchDocument {
		l.CandidateSource, l.CandidateDocID = "", nil
	}
	l.Status, l.MatchKind, l.MatchRule = LineMatched, kind, rule
	l.JournalLineID, l.JournalEntryID, l.PaymentID = &journalLineID, &journalEntryID, paymentID
	if label != "" {
		l.CandidateLabel = label
	}
	l.MatchedAt, l.MatchedBy = &at, &by
	return nil
}

// Unmatch melepas pencocokan baris (jurnal yang sudah dibuat tetap ada).
func (l *StatementLine) Unmatch() error {
	if l.Status != LineMatched {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Baris mutasi belum direkonsiliasi.")
	}
	l.Status, l.MatchKind, l.MatchRule = LineUnmatched, "", ""
	l.JournalLineID, l.JournalEntryID, l.JournalNumber, l.PaymentID = nil, nil, "", nil
	l.CandidateSource, l.CandidateDocID, l.CandidateLabel = "", nil, ""
	l.MatchedAt, l.MatchedBy = nil, nil
	return nil
}

// MatchCandidate adalah calon pasangan baris mutasi: baris jurnal akun rekening yang belum direkonsiliasi atau
// dokumen terbuka dari penyedia kandidat pembayaran.
type MatchCandidate struct {
	Kind           MatchKind
	JournalLineID  uuid.UUID
	JournalEntryID uuid.UUID
	Source         string
	DocumentID     uuid.UUID
	Label          string
	Date           clock.BusinessDate
	// Amount bertanda dari sisi rekening: masuk positif, keluar negatif.
	Amount money.Amount
	// References adalah teks yang dicari di referensi/keterangan mutasi (nomor BYR, nomor invoice, referensi
	// transfer).
	References []string
}

// Suggestion adalah usulan pasangan satu baris mutasi.
type Suggestion struct {
	LineID    uuid.UUID
	Candidate MatchCandidate
	Rule      MatchRule
}

// normalizeRef menyisakan huruf kapital dan angka agar "INV/2027/01/00031" cocok dengan "INV2027010031".
func normalizeRef(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// minRefLength adalah panjang minimal referensi kandidat yang dicari di keterangan mutasi.
const minRefLength = 6

func refMatches(line StatementLine, c MatchCandidate) bool {
	ref, desc := normalizeRef(line.Reference), normalizeRef(line.Description)
	for _, r := range c.References {
		n := normalizeRef(r)
		if n == "" {
			continue
		}
		if ref != "" && ref == n {
			return true
		}
		if len(n) >= minRefLength && (strings.Contains(desc, n) || (len(ref) >= minRefLength && strings.Contains(ref, n))) {
			return true
		}
	}
	return false
}

func dayDiff(a, b clock.BusinessDate) int {
	d := int(math.Round(a.Time().Sub(b.Time()).Hours() / 24))
	if d < 0 {
		return -d
	}
	return d
}

func candidateKey(c MatchCandidate) string {
	if c.Kind == MatchJournalLine {
		return "j:" + c.JournalLineID.String()
	}
	return "d:" + c.Source + ":" + c.DocumentID.String()
}

// SuggestMatches menjalankan aturan pencocokan berurutan (Technical Guideline 5.8.5): (1) referensi sama dan
// nominal sama, (2) nominal sama dengan selisih tanggal ≤ toleranceDays (tanggal terdekat; baris jurnal
// didahulukan dari dokumen). Kandidat dipakai paling banyak sekali. Bila ada beberapa kandidat sama kuat,
// baris dibiarkan untuk pencocokan manual. Fungsi murni dan deterministik.
func SuggestMatches(lines []StatementLine, cands []MatchCandidate, toleranceDays int) []Suggestion {
	ordered := slices.Clone(lines)
	slices.SortStableFunc(ordered, func(a, b StatementLine) int {
		if c := a.TransactionDate.Time().Compare(b.TransactionDate.Time()); c != 0 {
			return c
		}
		return a.LineNo - b.LineNo
	})
	used := map[string]bool{}
	done := map[uuid.UUID]bool{}
	var out []Suggestion

	for _, l := range ordered {
		var hits []MatchCandidate
		for _, c := range cands {
			if !used[candidateKey(c)] && c.Amount.Equal(l.Amount) && refMatches(l, c) {
				hits = append(hits, c)
			}
		}
		if best, ok := pickClosest(l, hits); ok {
			used[candidateKey(best)], done[l.ID] = true, true
			out = append(out, Suggestion{LineID: l.ID, Candidate: best, Rule: RuleReference})
		}
	}
	for _, l := range ordered {
		if done[l.ID] {
			continue
		}
		var hits []MatchCandidate
		for _, c := range cands {
			if !used[candidateKey(c)] && c.Amount.Equal(l.Amount) && dayDiff(l.TransactionDate, c.Date) <= toleranceDays {
				hits = append(hits, c)
			}
		}
		if best, ok := pickClosest(l, hits); ok {
			used[candidateKey(best)] = true
			out = append(out, Suggestion{LineID: l.ID, Candidate: best, Rule: RuleAmountDate})
		}
	}
	return out
}

// pickClosest memilih kandidat dengan tanggal terdekat (baris jurnal didahulukan). Seri → tidak ada usulan.
func pickClosest(l StatementLine, hits []MatchCandidate) (MatchCandidate, bool) {
	if len(hits) == 0 {
		return MatchCandidate{}, false
	}
	rank := func(c MatchCandidate) int {
		r := dayDiff(l.TransactionDate, c.Date) * 2
		if c.Kind != MatchJournalLine {
			r++
		}
		return r
	}
	slices.SortStableFunc(hits, func(a, b MatchCandidate) int { return rank(a) - rank(b) })
	if len(hits) > 1 && rank(hits[0]) == rank(hits[1]) {
		return MatchCandidate{}, false
	}
	return hits[0], true
}

// StatementSummary adalah ringkasan satu file mutasi.
type StatementSummary struct {
	ID              uuid.UUID
	BankAccountID   uuid.UUID
	BankAccountCode string
	BankAccountName string
	FileID          uuid.UUID
	FileName        string
	PeriodFrom      clock.BusinessDate
	PeriodTo        clock.BusinessDate
	LineCount       int
	TotalIn         money.Amount
	TotalOut        money.Amount
	MatchedCount    int
	SuggestedCount  int
	CreatedAt       time.Time
	CreatedBy       uuid.UUID
}

// NewStatementSummary menghitung periode dan total dari baris hasil baca.
func NewStatementSummary(rows []StatementRow) StatementSummary {
	var s StatementSummary
	s.TotalIn, s.TotalOut = money.Zero, money.Zero
	for i, r := range rows {
		if i == 0 || r.Date.Before(s.PeriodFrom) {
			s.PeriodFrom = r.Date
		}
		if i == 0 || r.Date.After(s.PeriodTo) {
			s.PeriodTo = r.Date
		}
		if r.Amount.IsPositive() {
			s.TotalIn = s.TotalIn.Add(r.Amount)
		} else {
			s.TotalOut = s.TotalOut.Add(r.Amount.Neg())
		}
	}
	s.LineCount = len(rows)
	return s
}
