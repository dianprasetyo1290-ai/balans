package domain

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
)

// DocStatus adalah status dokumen persediaan (Desain Status Dokumen bagian 1).
type DocStatus string

const (
	DocDraft              DocStatus = "draft"
	DocPendingApproval    DocStatus = "pending_approval"
	DocApproved           DocStatus = "approved"
	DocRejected           DocStatus = "rejected"
	DocInProgress         DocStatus = "in_progress"
	DocAwaitingSettlement DocStatus = "awaiting_settlement"
	DocPosted             DocStatus = "posted"
	DocCancelled          DocStatus = "cancelled"
)

var docStatusLabels = map[DocStatus]string{
	DocDraft: "Draft", DocPendingApproval: "Menunggu Persetujuan", DocApproved: "Disetujui", DocRejected: "Ditolak",
	DocInProgress: "Diproses", DocAwaitingSettlement: "Menunggu Penyelesaian", DocPosted: "Diposting",
	DocCancelled: "Dibatalkan",
}

// Label adalah label status untuk tampilan.
func (s DocStatus) Label() string { return docStatusLabels[s] }

// Valid memeriksa status.
func (s DocStatus) Valid() bool {
	_, ok := docStatusLabels[s]
	return ok
}

// DocAction adalah aksi bernama dokumen (allowedActions, Technical Guideline 5.4).
type DocAction string

const (
	ActionEdit     DocAction = "edit"
	ActionSubmit   DocAction = "submit"
	ActionApprove  DocAction = "approve"
	ActionReject   DocAction = "reject"
	ActionRevise   DocAction = "revise"
	ActionRelease  DocAction = "release"
	ActionHandover DocAction = "handover"
	ActionSettle   DocAction = "settle"
	ActionStart    DocAction = "start"
	ActionReport   DocAction = "report"
	ActionExecute  DocAction = "post"
	ActionCancel   DocAction = "cancel"
)

// transition adalah satu baris tabel transisi: aksi dari status asal ke status tujuan.
type transition struct {
	action DocAction
	from   []DocStatus
	to     DocStatus
}

func findTransition(table []transition, a DocAction, from DocStatus) (DocStatus, bool) {
	for _, t := range table {
		if t.action == a && slices.Contains(t.from, from) {
			return t.to, true
		}
	}
	return "", false
}

// errTransition menolak aksi yang tidak berlaku untuk status dokumen.
func errTransition(a DocAction, s DocStatus) error {
	return apperr.WithMessage(apperr.CodeInvalidStateTransition,
		fmt.Sprintf("Aksi %s tidak bisa dilakukan pada dokumen berstatus %s", a, s.Label()))
}

// CheckVersion menolak perubahan dengan versi lama (optimistic locking, TG 7.2).
func CheckVersion(current, given int) error {
	if current != given {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// Actor adalah pelaku sebuah langkah dokumen (ID + nama snapshot).
type Actor struct {
	ID   uuid.UUID
	Name string
}

// Step adalah pelaku dan waktu satu langkah dokumen; nil = belum terjadi.
type Step struct {
	By   uuid.UUID
	Name string
	At   time.Time
}

// NewStep membuat langkah dari pelaku.
func NewStep(a Actor, at time.Time) *Step { return &Step{By: a.ID, Name: a.Name, At: at} }

// Batas dokumen.
const (
	MaxDocLines      = 100
	MaxDocNoteLength = 500
)

func checkNote(fields map[string]string, key, note string) string {
	note = strings.TrimSpace(note)
	if len([]rune(note)) > MaxDocNoteLength {
		fields[key] = "Maksimal 500 karakter"
	}
	return note
}

func checkLineCount(fields map[string]string, n int) {
	switch {
	case n == 0:
		fields["lines"] = "Minimal satu baris"
	case n > MaxDocLines:
		fields["lines"] = fmt.Sprintf("Maksimal %d baris", MaxDocLines)
	}
}

func checkQty(fields map[string]string, key string, q decimal.Decimal) {
	if msg := ValidateQty(q); msg != "" {
		fields[key] = msg
	}
}

func docLineField(i int, f string) string { return fmt.Sprintf("lines[%d].%s", i, f) }

func validationOrNil(fields map[string]string) error {
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// SumDecimal menjumlahkan nilai.
func SumDecimal(values ...decimal.Decimal) decimal.Decimal {
	total := decimal.Zero
	for _, v := range values {
		total = total.Add(v)
	}
	return total
}
