package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// EntryType adalah asal jurnal.
type EntryType string

const (
	EntryManual    EntryType = "manual"
	EntryAutomatic EntryType = "automatic"
	EntryReversal  EntryType = "reversal"
)

// EntryStatus adalah status jurnal. Jurnal otomatis dan Balik Jurnal langsung diposting.
type EntryStatus string

const (
	EntryDraft  EntryStatus = "draft"
	EntryPosted EntryStatus = "posted"
)

// Source type bawaan modul accounting.
const (
	SourceManualJournal   = "manual_journal"
	SourceJournalReversal = "journal_reversal"
)

// MaxLines adalah batas baris satu jurnal agar form dan posting tetap ringan.
const MaxLines = 200

// Party adalah pihak sub-ledger pada baris jurnal (pelanggan, pemasok, karyawan).
type Party struct {
	Type string
	ID   uuid.UUID
}

// Line adalah satu baris jurnal. Satu baris hanya debit atau kredit (ck_journal_lines_debit_xor_credit).
type Line struct {
	ID          uuid.UUID
	LineNo      int
	AccountID   uuid.UUID
	AccountCode string
	AccountName string
	Debit       money.Amount
	Credit      money.Amount
	Dimensions  Dimensions
	Party       *Party
	AmountKey   string
	Memo        string
}

// Entry adalah satu jurnal.
type Entry struct {
	ID              uuid.UUID
	Number          string
	BusinessDate    clock.BusinessDate
	FiscalPeriodID  *uuid.UUID
	EntryType       EntryType
	SourceType      string
	SourceID        uuid.UUID
	SourceLabel     string
	PostingRuleCode string
	Description     string
	ReversalOfID    *uuid.UUID
	// ReversalOfNumber dan ReversedBy* diisi read model (jurnal asal / jurnal pembalik).
	ReversalOfNumber string
	ReversedByID     *uuid.UUID
	ReversedByNumber string
	Status           EntryStatus
	TotalAmount      money.Amount
	PostedAt         *time.Time
	PostedBy         *uuid.UUID
	CreatedAt        time.Time
	CreatedBy        uuid.UUID
	UpdatedAt        time.Time
	Version          int
	Lines            []Line
}

// Totals menjumlahkan debit dan kredit.
func (e Entry) Totals() (debit, credit money.Amount) {
	for _, l := range e.Lines {
		debit, credit = debit.Add(l.Debit), credit.Add(l.Credit)
	}
	return debit, credit
}

// IsPosted: jurnal sudah diposting dan tidak bisa diubah (ACC-13).
func (e Entry) IsPosted() bool { return e.Status == EntryPosted }

// isWholeCents memastikan nilai tersimpan tanpa pembulatan diam-diam (numeric(18,2)).
func isWholeCents(a money.Amount) bool { return a.Round().Equal(a) }

// ValidateLines memeriksa setiap baris: akun diisi, debit XOR kredit, positif, 2 desimal.
func ValidateLines(lines []Line) map[string]string {
	fields := map[string]string{}
	if len(lines) < 2 {
		fields["lines"] = "Jurnal minimal dua baris"
	}
	if len(lines) > MaxLines {
		fields["lines"] = fmt.Sprintf("Jurnal paling banyak %d baris", MaxLines)
	}
	for i, l := range lines {
		p := fmt.Sprintf("lines.%d.", i)
		if l.AccountID == uuid.Nil {
			fields[p+"accountId"] = "Akun wajib dipilih"
		}
		switch {
		case l.Debit.IsNegative() || l.Credit.IsNegative():
			fields[p+"debit"] = "Nominal tidak boleh negatif"
		case l.Debit.IsPositive() && l.Credit.IsPositive():
			fields[p+"debit"] = "Isi debit atau kredit, tidak keduanya"
		case l.Debit.IsZero() && l.Credit.IsZero():
			fields[p+"debit"] = "Isi nominal debit atau kredit"
		case !isWholeCents(l.Debit) || !isWholeCents(l.Credit):
			fields[p+"debit"] = "Nominal paling banyak 2 angka di belakang koma"
		}
		if len(l.Memo) > 500 {
			fields[p+"memo"] = "Keterangan baris paling panjang 500 karakter"
		}
	}
	return fields
}

// CheckBalanced menolak jurnal yang total debitnya tidak sama dengan total kredit (Technical Guideline 5.8.1).
func (e Entry) CheckBalanced() error {
	debit, credit := e.Totals()
	if !debit.Equal(credit) {
		return apperr.Validation(map[string]string{
			"lines": fmt.Sprintf("Jurnal tidak balance: total debit %s, total kredit %s, selisih %s",
				debit.String(), credit.String(), debit.Sub(credit).String()),
		})
	}
	if !debit.IsPositive() {
		return apperr.Validation(map[string]string{"lines": "Total jurnal harus lebih dari nol"})
	}
	return nil
}

// ManualInput adalah isian Jurnal Umum (S-127).
type ManualInput struct {
	BusinessDate clock.BusinessDate
	Description  string
	Lines        []Line
}

// NewManual menyusun draft Jurnal Umum. Balance diperiksa saat posting, agar draft bisa disimpan bertahap.
func NewManual(in ManualInput) (Entry, error) {
	e := Entry{EntryType: EntryManual, SourceType: SourceManualJournal, Status: EntryDraft}
	if err := e.applyManual(in); err != nil {
		return Entry{}, err
	}
	return e, nil
}

func (e *Entry) applyManual(in ManualInput) error {
	in.Description = strings.TrimSpace(in.Description)
	fields := ValidateLines(in.Lines)
	if in.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal jurnal wajib diisi"
	}
	if in.Description == "" {
		fields["description"] = "Keterangan jurnal wajib diisi"
	} else if len(in.Description) > 500 {
		fields["description"] = "Keterangan paling panjang 500 karakter"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	e.BusinessDate, e.Description = in.BusinessDate, in.Description
	e.Lines = make([]Line, len(in.Lines))
	for i, l := range in.Lines {
		l.LineNo = i + 1
		l.Memo = strings.TrimSpace(l.Memo)
		e.Lines[i] = l
	}
	debit, _ := e.Totals()
	e.TotalAmount = debit
	return nil
}

// CheckEditable menolak perubahan jurnal yang sudah diposting atau bukan Jurnal Umum manual (ACC-13).
func (e Entry) CheckEditable() error {
	if e.Status != EntryDraft {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Jurnal yang sudah diposting tidak bisa diubah atau dihapus. Koreksi dengan Balik Jurnal.")
	}
	if e.EntryType != EntryManual {
		return apperr.InvalidStateTransition("journal_entry", string(e.EntryType), "edit")
	}
	return nil
}

// UpdateManual mengganti isi draft Jurnal Umum.
func (e *Entry) UpdateManual(in ManualInput) error {
	if err := e.CheckEditable(); err != nil {
		return err
	}
	return e.applyManual(in)
}

// Post memeriksa draft siap diposting. Nomor, periode, dan waktu diisi service di transaksi posting.
func (e *Entry) Post(number string, periodID uuid.UUID, at time.Time, by uuid.UUID) error {
	if e.Status != EntryDraft {
		return apperr.InvalidStateTransition("journal_entry", string(e.Status), string(EntryPosted))
	}
	if err := e.CheckBalanced(); err != nil {
		return err
	}
	debit, _ := e.Totals()
	e.Number, e.FiscalPeriodID, e.Status, e.TotalAmount = number, &periodID, EntryPosted, debit
	e.PostedAt, e.PostedBy = &at, &by
	return nil
}

// CheckReversible memeriksa syarat Balik Jurnal: sudah diposting, belum dibalik, dan bukan jurnal pembalik.
func (e Entry) CheckReversible() error {
	switch {
	case e.Status != EntryPosted:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya jurnal yang sudah diposting yang bisa dibalik.")
	case e.ReversedByID != nil:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Jurnal ini sudah dibalik oleh "+e.ReversedByNumber+".")
	case e.EntryType == EntryReversal:
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Jurnal pembalik tidak bisa dibalik lagi. Buat Jurnal Umum baru bila perlu koreksi.")
	}
	return nil
}

// NewReversal membuat jurnal pembalik: debit dan kredit ditukar, dimensi dan pihak tetap (S-128).
func (e Entry) NewReversal(date clock.BusinessDate, reason string) (Entry, error) {
	if err := e.CheckReversible(); err != nil {
		return Entry{}, err
	}
	reason = strings.TrimSpace(reason)
	fields := map[string]string{}
	if reason == "" {
		fields["reason"] = "Alasan Balik Jurnal wajib diisi"
	} else if len(reason) > 400 {
		fields["reason"] = "Alasan paling panjang 400 karakter"
	}
	if date.IsZero() {
		fields["businessDate"] = "Tanggal Balik Jurnal wajib diisi"
	} else if date.Before(e.BusinessDate) {
		fields["businessDate"] = "Tanggal Balik Jurnal tidak boleh sebelum tanggal jurnal asal"
	}
	if len(fields) > 0 {
		return Entry{}, apperr.Validation(fields)
	}
	id := e.ID
	r := Entry{
		BusinessDate: date, EntryType: EntryReversal, SourceType: SourceJournalReversal, SourceID: e.ID,
		SourceLabel: e.Number, Description: fmt.Sprintf("Balik Jurnal %s: %s", e.Number, reason),
		ReversalOfID: &id, ReversalOfNumber: e.Number, Status: EntryDraft, TotalAmount: e.TotalAmount,
	}
	r.Lines = make([]Line, len(e.Lines))
	for i, l := range e.Lines {
		l.ID = uuid.Nil
		l.LineNo = i + 1
		l.Debit, l.Credit = l.Credit, l.Debit
		r.Lines[i] = l
	}
	return r, nil
}

// Action adalah aksi yang ditampilkan di detail jurnal (allowedActions).
type Action string

const (
	ActionEdit    Action = "edit"
	ActionDelete  Action = "delete"
	ActionPost    Action = "post"
	ActionReverse Action = "reverse"
)

// Permission modul accounting untuk jurnal (Desain Role & Approval 3.4).
const (
	PermJournalRead    = "accounting.journal.read"
	PermJournalWrite   = "accounting.journal.write"
	PermJournalPost    = "accounting.journal.post"
	PermJournalReverse = "accounting.journal.cancel"
)

// AllowedActions mengembalikan aksi yang boleh dilakukan pemegang permission.
// Jurnal otomatis dibalik lewat pembatalan dokumen sumbernya, bukan dari halaman jurnal.
func (e Entry) AllowedActions(has func(string) bool) []Action {
	var out []Action
	switch {
	case e.Status == EntryDraft && e.EntryType == EntryManual:
		if has(PermJournalWrite) {
			out = append(out, ActionEdit, ActionDelete)
		}
		if has(PermJournalPost) {
			out = append(out, ActionPost)
		}
	case e.EntryType == EntryManual && e.CheckReversible() == nil && has(PermJournalReverse):
		out = append(out, ActionReverse)
	}
	return out
}
