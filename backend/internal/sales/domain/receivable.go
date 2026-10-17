package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// PaymentMethod adalah cara terima pembayaran pelanggan.
type PaymentMethod string

const (
	MethodBankTransfer PaymentMethod = "bank_transfer"
	MethodCash         PaymentMethod = "cash"
)

// Valid memeriksa cara bayar.
func (m PaymentMethod) Valid() bool { return m == MethodBankTransfer || m == MethodCash }

// PaymentAllocation adalah bagian pembayaran untuk satu invoice.
type PaymentAllocation struct {
	InvoiceID     uuid.UUID
	InvoiceNumber string
	Amount        money.Amount
}

// CustomerPayment adalah pembayaran pelanggan/mitra yang dialokasikan ke satu atau banyak invoice (ACC-15) dan
// dijurnal A-PB (Dr Kas/Bank, Cr Piutang Usaha).
type CustomerPayment struct {
	ID             uuid.UUID
	Number         string
	PartyType      string
	PartyID        uuid.UUID
	PartyName      string
	PaymentDate    clock.BusinessDate
	Amount         money.Amount
	Method         PaymentMethod
	AccountCode    string
	Reference      string
	ProofFileID    *uuid.UUID
	Note           string
	Status         Status
	CancelReason   string
	JournalEntryID *uuid.UUID
	Allocations    []PaymentAllocation
	CreatedBy      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
}

// PaymentInput adalah isian pembayaran pelanggan.
type PaymentInput struct {
	PartyType   string
	PartyID     uuid.UUID
	PaymentDate clock.BusinessDate
	Amount      money.Amount
	Method      PaymentMethod
	AccountCode string
	Reference   string
	ProofFileID *uuid.UUID
	Note        string
	Allocations []PaymentAllocation
}

// NewPayment memvalidasi pembayaran terhadap invoice yang dialokasikan (sudah dikunci service): pihak sama,
// invoice terbuka, alokasi ≤ sisa piutang, dan jumlah alokasi = nominal pembayaran (tanpa sisa tak teralokasi).
func NewPayment(in PaymentInput, invoices map[uuid.UUID]Invoice, cashAccounts []string) (CustomerPayment, error) {
	in.Reference, in.Note = strings.TrimSpace(in.Reference), strings.TrimSpace(in.Note)
	f := fieldErrors{}
	if in.PartyType != PartyCustomer && in.PartyType != PartyPartner {
		f.add("partyType", "Jenis pihak tidak dikenal")
	}
	if in.PartyID == uuid.Nil {
		f.add("partyId", "Pilih pelanggan atau mitra")
	}
	if in.PaymentDate.IsZero() {
		f.add("paymentDate", "Tanggal bayar wajib diisi")
	}
	if msg := validAmount(in.Amount, true); msg != "" {
		f.add("amount", msg)
	}
	if !in.Method.Valid() {
		f.add("method", "Cara bayar tidak dikenal")
	}
	found := false
	for _, c := range cashAccounts {
		found = found || c == in.AccountCode
	}
	if !found {
		f.add("accountCode", "Pilih akun Kas & Bank")
	}
	if tooLong(in.Reference, MaxTextLength) {
		f.add("reference", "Referensi maksimal 200 karakter")
	}
	if tooLong(in.Note, MaxNoteLength) {
		f.add("note", "Catatan maksimal 1000 karakter")
	}
	if len(in.Allocations) == 0 {
		f.add("allocations", "Alokasikan pembayaran ke minimal satu invoice")
	}
	sum, seen := money.Zero, map[uuid.UUID]bool{}
	for i, a := range in.Allocations {
		key := fmt.Sprintf("allocations.%d.", i)
		inv, ok := invoices[a.InvoiceID]
		switch {
		case !ok:
			f.add(key+"invoiceId", "Invoice tidak ditemukan")
			continue
		case seen[a.InvoiceID]:
			f.add(key+"invoiceId", "Invoice "+inv.Number+" sudah dialokasikan di baris lain")
		case inv.PartyType != in.PartyType || inv.PartyID != in.PartyID:
			f.add(key+"invoiceId", "Invoice "+inv.Number+" milik pihak lain")
		case !inv.IsOpen():
			f.add(key+"invoiceId", "Invoice "+inv.Number+" tidak punya sisa piutang")
		}
		seen[a.InvoiceID] = true
		if msg := validAmount(a.Amount, true); msg != "" {
			f.add(key+"amount", msg)
		} else if a.Amount.Cmp(inv.Outstanding()) > 0 {
			f.add(key+"amount", "Melebihi sisa piutang "+inv.Outstanding().Rupiah())
		}
		in.Allocations[i].InvoiceNumber = inv.Number
		sum = sum.Add(a.Amount)
	}
	if len(f) == 0 && !sum.Equal(in.Amount) {
		f.add("amount", fmt.Sprintf("Jumlah alokasi %s harus sama dengan nominal pembayaran", sum.Rupiah()))
	}
	if err := f.err(); err != nil {
		return CustomerPayment{}, err
	}
	return CustomerPayment{
		PartyType: in.PartyType, PartyID: in.PartyID, PaymentDate: in.PaymentDate, Amount: in.Amount,
		Method: in.Method, AccountCode: in.AccountCode, Reference: in.Reference, ProofFileID: in.ProofFileID,
		Note: in.Note, Status: StatusPosted, Allocations: in.Allocations,
	}, nil
}

// Cancel membatalkan pembayaran Diposting (jurnal A-PB dibalik dan piutang invoice dikembalikan oleh service).
func (p *CustomerPayment) Cancel(reason string) error {
	r, err := requireReason(reason, "pembatalan")
	if err != nil {
		return err
	}
	if p.Status != StatusPosted {
		return invalidTransition("Pembayaran "+p.Number, p.Status, StatusCancelled)
	}
	p.Status, p.CancelReason = StatusCancelled, r
	return nil
}

// AgingBucket adalah kelompok umur piutang (RPT-11).
type AgingBucket string

const (
	BucketCurrent AgingBucket = "current"
	Bucket1To30   AgingBucket = "1_30"
	Bucket31To60  AgingBucket = "31_60"
	Bucket61To90  AgingBucket = "61_90"
	BucketOver90  AgingBucket = "over_90"
)

// AgingBuckets adalah urutan kolom laporan umur piutang.
var AgingBuckets = []AgingBucket{BucketCurrent, Bucket1To30, Bucket31To60, Bucket61To90, BucketOver90}

// BucketOf menentukan kelompok umur dari jumlah hari lewat jatuh tempo.
func BucketOf(daysOverdue int) AgingBucket {
	switch {
	case daysOverdue <= 0:
		return BucketCurrent
	case daysOverdue <= 30:
		return Bucket1To30
	case daysOverdue <= 60:
		return Bucket31To60
	case daysOverdue <= 90:
		return Bucket61To90
	}
	return BucketOver90
}

// AgingRow adalah sisa piutang satu pihak per kelompok umur.
type AgingRow struct {
	PartyType string
	PartyID   uuid.UUID
	PartyName string
	Buckets   map[AgingBucket]money.Amount
	Total     money.Amount
	Invoices  int
}

// AgeReceivables mengelompokkan sisa piutang invoice terbuka per pihak pada tanggal asOf, urut nama pihak.
func AgeReceivables(invoices []Invoice, asOf clock.BusinessDate) []AgingRow {
	byParty := map[string]*AgingRow{}
	var order []string
	for _, inv := range invoices {
		out := inv.Outstanding()
		if !inv.IsOpen() || !out.IsPositive() || inv.InvoiceDate.After(asOf) {
			continue
		}
		key := inv.PartyType + ":" + inv.PartyID.String()
		row, ok := byParty[key]
		if !ok {
			row = &AgingRow{PartyType: inv.PartyType, PartyID: inv.PartyID, PartyName: inv.PartyName,
				Buckets: map[AgingBucket]money.Amount{}, Total: money.Zero}
			for _, b := range AgingBuckets {
				row.Buckets[b] = money.Zero
			}
			byParty[key] = row
			order = append(order, key)
		}
		b := BucketOf(inv.DaysOverdue(asOf))
		row.Buckets[b] = row.Buckets[b].Add(out)
		row.Total = row.Total.Add(out)
		row.Invoices++
	}
	out := make([]AgingRow, 0, len(order))
	for _, k := range order {
		out = append(out, *byParty[k])
	}
	sortRows(out)
	return out
}

func sortRows(rows []AgingRow) {
	slices.SortStableFunc(rows, func(a, b AgingRow) int {
		return strings.Compare(strings.ToLower(a.PartyName), strings.ToLower(b.PartyName))
	})
}
