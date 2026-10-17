package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// Batas panjang field.
const (
	MaxReasonLength      = 500
	MaxNoteLength        = 500
	MaxSourceLabelLength = 120
	MaxActionLength      = 60
	MaxBatchNumberLength = 40
	// MaxLines membatasi baris per posting agar transaksi tetap singkat (TG 7.2).
	MaxLines = 500
)

// ActionPost adalah aksi default posting dokumen sumber.
const ActionPost = "post"

var sourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Source adalah dokumen sumber mutasi. (Type, ID, Action) unik: aksi yang sama tidak bisa memutasi stok dua kali.
type Source struct {
	// Type adalah jenis dokumen snake_case, mis. "goods_receipt", "stock_transfer".
	Type string
	ID   uuid.UUID
	// Action membedakan beberapa mutasi dari satu dokumen, mis. "post", "qc", "reserve", "release". Kosong = "post".
	Action string
	// Label adalah nomor dokumen untuk kartu stok, mis. "GR/2026/10/0001".
	Label        string
	BusinessDate clock.BusinessDate
}

// Normalize merapikan dan memvalidasi sumber; field error memakai prefix "source.".
func (s Source) Normalize(fields map[string]string) Source {
	s.Type = strings.TrimSpace(s.Type)
	s.Action = strings.TrimSpace(s.Action)
	s.Label = strings.TrimSpace(s.Label)
	if s.Action == "" {
		s.Action = ActionPost
	}
	if !sourceTypePattern.MatchString(s.Type) || len(s.Type) > 60 {
		fields["source.type"] = "Jenis dokumen sumber tidak valid"
	}
	if s.ID == uuid.Nil {
		fields["source.id"] = "Dokumen sumber wajib diisi"
	}
	if len(s.Action) > MaxActionLength {
		fields["source.action"] = "Aksi terlalu panjang"
	}
	if len([]rune(s.Label)) > MaxSourceLabelLength {
		fields["source.label"] = "Label dokumen maksimal 120 karakter"
	}
	if s.BusinessDate.IsZero() {
		fields["source.businessDate"] = "Tanggal bisnis wajib diisi"
	}
	return s
}

// LineKind adalah jenis baris posting.
type LineKind string

const (
	// KindReceive menambah stok di To dan membuat cost layer (penerimaan, retur masuk, selisih lebih).
	KindReceive LineKind = "receive"
	// KindIssue mengurangi stok dari From dan mengembalikan HPP FIFO (penjualan, pemakaian, pemusnahan).
	KindIssue LineKind = "issue"
	// KindMove memindah stok From → To dalam batch yang sama: pindah lokasi, ganti status (quarantine →
	// available, available → reserved, → in_transit). Tidak mengonsumsi cost layer.
	KindMove LineKind = "move"
)

// Place adalah lokasi + status stok.
type Place struct {
	LocationID uuid.UUID
	Status     StockStatus
}

// ReservationRef menunjuk reservasi dokumen yang dipakai baris yang mengambil stok berstatus reserved.
type ReservationRef struct {
	SourceType string
	SourceID   uuid.UUID
	// SourceLineID opsional: hanya reservasi baris itu.
	SourceLineID *uuid.UUID
	// ReservationID opsional: hanya satu reservasi tertentu.
	ReservationID *uuid.UUID
}

// NewBatch adalah batch yang dicari/dibuat saat penerimaan (unik per produk + nomor batch).
type NewBatch struct {
	Number         string
	ProductionDate *clock.BusinessDate
	ExpiryDate     *clock.BusinessDate
}

// Line adalah satu baris posting.
type Line struct {
	Kind         LineKind
	SourceLineID *uuid.UUID
	ProductID    uuid.UUID
	Qty          decimal.Decimal
	// BatchID: batch yang diambil/diterima. Untuk issue/move produk berbatch, nil = dialokasikan otomatis FEFO.
	BatchID *uuid.UUID
	// Batch: khusus receive, batch dicari/dibuat dari nomornya (alternatif BatchID).
	Batch *NewBatch
	From  *Place
	To    *Place
	// UnitCost: khusus receive, HPP per unit (4 desimal). Nil = biaya cost layer terakhir batch tersebut.
	UnitCost *decimal.Decimal
	// BatchReason wajib bila batch dipilih manual dari stok available dan bukan saran FEFO (INV-09).
	BatchReason string
	// Reservation wajib bila From.Status = reserved.
	Reservation *ReservationRef
	Note        string
}

// Posting adalah satu aksi dokumen sumber terhadap stok; seluruh baris berhasil atau tidak sama sekali.
type Posting struct {
	Source Source
	Lines  []Line
	// ReservationExpiresAt: batas waktu reservasi yang dibuat baris move → reserved (TG 5.7.5); nil = tanpa batas.
	ReservationExpiresAt *time.Time
}

func lineField(i int, f string) string { return fmt.Sprintf("lines[%d].%s", i, f) }

func validPlace(p *Place, fields map[string]string, i int, name string) {
	switch {
	case p == nil:
		fields[lineField(i, name)] = "Lokasi wajib diisi"
	case p.LocationID == uuid.Nil:
		fields[lineField(i, name+".locationId")] = "Lokasi wajib diisi"
	case !p.Status.Valid():
		fields[lineField(i, name+".status")] = "Status stok tidak valid"
	}
}

// Validate merapikan dan memvalidasi posting. Aturan produk/lokasi/batch diperiksa service.
func (p Posting) Validate() (Posting, error) {
	fields := map[string]string{}
	p.Source = p.Source.Normalize(fields)
	switch {
	case len(p.Lines) == 0:
		fields["lines"] = "Minimal satu baris"
	case len(p.Lines) > MaxLines:
		fields["lines"] = fmt.Sprintf("Maksimal %d baris per posting", MaxLines)
	}
	lines := make([]Line, len(p.Lines))
	for i, l := range p.Lines {
		l.BatchReason = strings.TrimSpace(l.BatchReason)
		l.Note = strings.TrimSpace(l.Note)
		if l.ProductID == uuid.Nil {
			fields[lineField(i, "productId")] = "Produk wajib diisi"
		}
		if msg := ValidateQty(l.Qty); msg != "" {
			fields[lineField(i, "qty")] = msg
		}
		if len([]rune(l.BatchReason)) > MaxReasonLength {
			fields[lineField(i, "batchReason")] = "Alasan maksimal 500 karakter"
		}
		if len([]rune(l.Note)) > MaxNoteLength {
			fields[lineField(i, "note")] = "Catatan maksimal 500 karakter"
		}
		switch l.Kind {
		case KindReceive:
			validPlace(l.To, fields, i, "to")
			if l.From != nil || l.Reservation != nil {
				fields[lineField(i, "from")] = "Penerimaan tidak memiliki lokasi asal"
			}
			if l.To != nil && l.To.Status == StatusReserved {
				fields[lineField(i, "to.status")] = "Stok diterima tidak bisa langsung berstatus Dialokasikan"
			}
			if l.UnitCost != nil && (l.UnitCost.IsNegative() || l.UnitCost.Exponent() < -4 && !l.UnitCost.Equal(l.UnitCost.Round(4))) {
				fields[lineField(i, "unitCost")] = "HPP per unit tidak boleh negatif dan maksimal 4 angka di belakang koma"
			}
			if l.Batch != nil {
				if l.BatchID != nil {
					fields[lineField(i, "batch")] = "Isi batch atau nomor batch baru, bukan keduanya"
				}
				nb, msg := normalizeNewBatch(*l.Batch)
				if msg != "" {
					fields[lineField(i, "batch.number")] = msg
				}
				l.Batch = &nb
			}
		case KindIssue, KindMove:
			validPlace(l.From, fields, i, "from")
			if l.Kind == KindMove {
				validPlace(l.To, fields, i, "to")
				if l.From != nil && l.To != nil && *l.From == *l.To {
					fields[lineField(i, "to")] = "Lokasi dan status tujuan sama dengan asal"
				}
			} else if l.To != nil {
				fields[lineField(i, "to")] = "Barang keluar tidak memiliki lokasi tujuan"
			}
			if l.Batch != nil || l.UnitCost != nil {
				fields[lineField(i, "batch")] = "Nomor batch baru dan HPP hanya untuk penerimaan"
			}
			fromReserved := l.From != nil && l.From.Status == StatusReserved
			switch {
			case fromReserved && (l.Reservation == nil || l.Reservation.SourceID == uuid.Nil || !sourceTypePattern.MatchString(l.Reservation.SourceType)):
				fields[lineField(i, "reservation")] = "Stok Dialokasikan hanya bisa diambil dengan referensi reservasi"
			case !fromReserved && l.Reservation != nil:
				fields[lineField(i, "reservation")] = "Referensi reservasi hanya untuk stok Dialokasikan"
			}
		default:
			fields[lineField(i, "kind")] = "Jenis baris tidak valid"
		}
		lines[i] = l
	}
	p.Lines = lines
	if len(fields) > 0 {
		return Posting{}, apperr.Validation(fields)
	}
	return p, nil
}

// IntoReserved memeriksa apakah baris membuat reservasi.
func (l Line) IntoReserved() bool {
	return l.Kind == KindMove && l.To != nil && l.To.Status == StatusReserved
}

func normalizeNewBatch(b NewBatch) (NewBatch, string) {
	b.Number = strings.ToUpper(strings.TrimSpace(b.Number))
	switch {
	case b.Number == "":
		return b, "Nomor batch wajib diisi"
	case len([]rune(b.Number)) > MaxBatchNumberLength:
		return b, "Nomor batch maksimal 40 karakter"
	case b.ProductionDate != nil && b.ExpiryDate != nil && b.ExpiryDate.Before(*b.ProductionDate):
		return b, "Tanggal kedaluwarsa tidak boleh sebelum tanggal produksi"
	}
	return b, ""
}

// Movement adalah satu baris ledger yang akan/sudah ditulis. Qty dan CostAmount bertanda (+ masuk, − keluar).
type Movement struct {
	ID            uuid.UUID
	LineIndex     int
	Type          MovementType
	ProductID     uuid.UUID
	LocationID    uuid.UUID
	BatchID       *uuid.UUID
	Status        StockStatus
	Qty           decimal.Decimal
	UnitCost      decimal.Decimal
	CostAmount    decimal.Decimal
	ReservationID *uuid.UUID
	BatchReason   string
	Note          string
	Consumptions  []Consumption
}

// Key mengembalikan kunci saldo movement.
func (m Movement) Key() BalanceKey {
	return BalanceKey{ProductID: m.ProductID, LocationID: m.LocationID, BatchID: m.BatchID, Status: m.Status}
}
