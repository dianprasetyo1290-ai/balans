package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
)

// Status adalah status dokumen gudang (Desain Status Dokumen bagian 1 dan 3).
type Status string

const (
	StatusDraft             Status = "draft"
	StatusSubmitted         Status = "submitted"
	StatusPendingApproval   Status = "pending_approval"
	StatusRejected          Status = "rejected"
	StatusInProgress        Status = "in_progress"
	StatusInTransit         Status = "in_transit"
	StatusPartiallyReceived Status = "partially_received"
	StatusReceived          Status = "received"
	StatusPosted            Status = "posted"
	StatusCompleted         Status = "completed"
	StatusCancelled         Status = "cancelled"
)

// Aksi dokumen gudang (allowedActions dan nama transisi).
const (
	ActionEdit     = "edit"
	ActionSubmit   = "submit"
	ActionRelease  = "release"
	ActionPrepare  = "prepare"
	ActionPost     = "post"
	ActionShip     = "ship"
	ActionReceive  = "receive"
	ActionComplete = "complete"
	ActionCancel   = "cancel"
	ActionStart    = "start"
	ActionCount    = "count"
	ActionApprove  = "approve"
	ActionReject   = "reject"
	ActionRecount  = "recount"
	ActionQC       = "qc"
)

// Batas dokumen.
const (
	MaxDocumentLines = 200
	MaxNoteLength    = 500
	// QtyScale adalah jumlah desimal qty (numeric(18,3)).
	QtyScale = 3
)

// transition adalah satu baris tabel transisi status.
type transition struct {
	from   Status
	action string
	to     Status
}

// transitionTable mencari status tujuan aksi; aksi yang tidak terdaftar ditolak INVALID_STATE_TRANSITION.
type transitionTable []transition

func (t transitionTable) next(entity string, from Status, action string) (Status, error) {
	for _, tr := range t {
		if tr.from == from && tr.action == action {
			return tr.to, nil
		}
	}
	return "", apperr.InvalidStateTransition(entity, string(from), action)
}

func (t transitionTable) allows(from Status, action string) bool {
	_, err := t.next("", from, action)
	return err == nil
}

var sourceTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidSourceType memeriksa jenis dokumen sumber snake_case (mis. goods_receipt).
func ValidSourceType(s string) bool { return sourceTypePattern.MatchString(s) && len(s) <= 60 }

func lineField(i int, f string) string { return fmt.Sprintf("lines[%d].%s", i, f) }

// validQty memeriksa qty positif dengan maksimal 3 desimal.
func validQty(q decimal.Decimal) bool {
	return q.IsPositive() && q.Equal(q.Round(QtyScale))
}

// validQtyOrZero memeriksa qty ≥ 0 dengan maksimal 3 desimal.
func validQtyOrZero(q decimal.Decimal) bool {
	return !q.IsNegative() && q.Equal(q.Round(QtyScale))
}

func checkNote(note string, field string, fields map[string]string) string {
	note = strings.TrimSpace(note)
	if len([]rune(note)) > MaxNoteLength {
		fields[field] = fmt.Sprintf("Maksimal %d karakter", MaxNoteLength)
	}
	return note
}
