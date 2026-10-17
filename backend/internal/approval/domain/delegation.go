package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

// MaxDelegationDays membatasi satu delegasi agar tidak berlaku tanpa batas waktu.
const MaxDelegationDays = 90

// Delegation adalah pengalihan hak approval user yang cuti ke user lain selama rentang tanggal WIB (inklusif).
type Delegation struct {
	ID          uuid.UUID
	From        uuid.UUID
	To          uuid.UUID
	ValidFrom   clock.BusinessDate
	ValidTo     clock.BusinessDate
	Reason      string
	CancelledAt *time.Time
	CreatedBy   uuid.UUID
	CreatedAt   time.Time
	Version     int
}

// DelegationInput adalah isian delegasi.
type DelegationInput struct {
	From      uuid.UUID
	To        uuid.UUID
	ValidFrom clock.BusinessDate
	ValidTo   clock.BusinessDate
	Reason    string
}

// NewDelegation memvalidasi delegasi baru. today adalah tanggal bisnis saat ini.
func NewDelegation(in DelegationInput, today clock.BusinessDate) (Delegation, error) {
	fields := map[string]string{}
	reason := strings.TrimSpace(in.Reason)
	switch {
	case in.To == uuid.Nil:
		fields["toUserId"] = "Penerima delegasi wajib diisi"
	case in.From == in.To:
		fields["toUserId"] = "Penerima delegasi harus orang lain"
	}
	switch {
	case in.ValidFrom.IsZero():
		fields["validFrom"] = "Tanggal mulai wajib diisi"
	case in.ValidFrom.Before(today):
		fields["validFrom"] = "Tanggal mulai tidak boleh sebelum hari ini"
	}
	switch {
	case in.ValidTo.IsZero():
		fields["validTo"] = "Tanggal selesai wajib diisi"
	case !in.ValidFrom.IsZero() && in.ValidTo.Before(in.ValidFrom):
		fields["validTo"] = "Tanggal selesai tidak boleh sebelum tanggal mulai"
	case !in.ValidFrom.IsZero() && in.ValidFrom.AddDays(MaxDelegationDays-1).Before(in.ValidTo):
		fields["validTo"] = "Delegasi maksimal 90 hari"
	}
	if utf8.RuneCountInString(reason) > 500 {
		fields["reason"] = "Alasan maksimal 500 karakter"
	}
	if len(fields) > 0 {
		return Delegation{}, apperr.Validation(fields)
	}
	return Delegation{From: in.From, To: in.To, ValidFrom: in.ValidFrom, ValidTo: in.ValidTo, Reason: reason}, nil
}

// ActiveOn memeriksa apakah delegasi berlaku pada tanggal tertentu.
func (d Delegation) ActiveOn(date clock.BusinessDate) bool {
	return d.CancelledAt == nil && !date.Before(d.ValidFrom) && !date.After(d.ValidTo)
}

// Cancel menghentikan delegasi yang belum berakhir, mis. approver kembali lebih cepat dari cuti.
func (d *Delegation) Cancel(now time.Time, today clock.BusinessDate) error {
	if d.CancelledAt != nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Delegasi sudah dibatalkan.")
	}
	if d.ValidTo.Before(today) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Delegasi sudah berakhir.")
	}
	d.CancelledAt = &now
	return nil
}
