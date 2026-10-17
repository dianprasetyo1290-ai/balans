package domain

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// Status adalah status permintaan approval (Desain Status Dokumen bagian 1).
type Status string

const (
	StatusPendingApproval Status = "pending_approval"
	StatusApproved        Status = "approved"
	StatusRejected        Status = "rejected"
	StatusCancelled       Status = "cancelled"
)

var requestTransitions = map[Status][]Status{
	StatusPendingApproval: {StatusApproved, StatusRejected, StatusCancelled},
}

// Decision adalah keputusan approver pada satu level.
type Decision string

const (
	DecisionApproved Decision = "approved"
	DecisionRejected Decision = "rejected"
)

// MaxNoteLength adalah panjang maksimum catatan keputusan.
const MaxNoteLength = 1000

// RoleGrant adalah role yang dipegang user. Division adalah scope divisi role (null = semua divisi).
type RoleGrant struct {
	Code     string
	Division *uuid.UUID
}

// Principal adalah user beserta role-nya, dipakai untuk menilai siapa yang boleh memutuskan sebuah level.
type Principal struct {
	UserID uuid.UUID
	Name   string
	Email  string
	Roles  []RoleGrant
}

// Step adalah snapshot satu level approval saat dokumen diajukan.
type Step struct {
	ID           uuid.UUID
	Level        int
	PolicyID     uuid.UUID
	ApproverRole string
	ApproverUser *uuid.UUID
	Substitute   *uuid.UUID
	// Division adalah divisi dokumen, membatasi approver ber-scope divisi (Head Divisi).
	Division *uuid.UUID
	// RequesterIsApprover: pembuat dokumen memegang jabatan approver level ini, sehingga level naik ke pengganti.
	RequesterIsApprover bool
	Decision            *Decision
	DecidedBy           *uuid.UUID
	DecidedByName       string
	DelegatedFrom       *uuid.UUID
	Note                string
	ActivatedAt         *time.Time
	DecidedAt           *time.Time
	LastRemindedAt      *time.Time
}

// HoldsPosition memeriksa apakah user memegang jabatan approver level ini.
func (s Step) HoldsPosition(p Principal) bool {
	if s.ApproverUser != nil {
		return p.UserID == *s.ApproverUser
	}
	return slices.ContainsFunc(p.Roles, func(g RoleGrant) bool {
		return g.Code == s.ApproverRole && (g.Division == nil || s.Division == nil || *g.Division == *s.Division)
	})
}

// canDecide menilai user secara langsung (tanpa delegasi). Pembuat dokumen tidak pernah boleh memutuskan;
// jika pembuat memegang jabatan approver, pengganti level ini ikut berhak.
func (s Step) canDecide(p Principal, requester uuid.UUID) bool {
	if p.UserID == requester {
		return false
	}
	if s.HoldsPosition(p) {
		return true
	}
	return s.RequesterIsApprover && s.Substitute != nil && *s.Substitute == p.UserID
}

// Request adalah permintaan approval satu dokumen beserta langkah-langkahnya.
type Request struct {
	ID              uuid.UUID
	Subject         Subject
	RequestedBy     uuid.UUID
	RequestedByName string
	Status          Status
	CurrentLevel    int
	Steps           []Step
	DecidedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int
}

// validateSubject memeriksa data dokumen yang dikirim modul pemilik.
func validateSubject(s Subject) error {
	fields := map[string]string{}
	if _, ok := LookupSubjectType(s.Type); !ok {
		fields["subjectType"] = "Jenis dokumen tidak dikenal"
	}
	if s.ID == uuid.Nil {
		fields["subjectId"] = "Dokumen wajib diisi"
	}
	if strings.TrimSpace(s.Label) == "" {
		fields["subjectLabel"] = "Nomor dokumen wajib diisi"
	}
	if s.Amount.IsNegative() {
		fields["amount"] = "Nominal tidak boleh negatif"
	}
	if s.Link != "" && !strings.HasPrefix(s.Link, "/") {
		fields["subjectLink"] = "Tautan dokumen harus berupa path ERP"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// NewRequest membuat permintaan dari kebijakan yang sudah dipilih (SelectPolicies). Level pertama langsung aktif.
func NewRequest(s Subject, requester Principal, policies []Policy, now time.Time) (Request, error) {
	if err := validateSubject(s); err != nil {
		return Request{}, err
	}
	if len(policies) == 0 {
		return Request{}, apperr.WithMessage(apperr.CodeApprovalPolicyMissing,
			"Kebijakan persetujuan untuk "+s.Type.Label()+" belum diatur. Hubungi Admin Sistem.")
	}
	steps := make([]Step, len(policies))
	for i, p := range policies {
		steps[i] = Step{
			Level: i + 1, PolicyID: p.ID, ApproverRole: p.ApproverRole, ApproverUser: p.ApproverUser,
			Substitute: p.Substitute, Division: s.Division,
		}
		steps[i].RequesterIsApprover = steps[i].HoldsPosition(requester)
	}
	steps[0].ActivatedAt = &now
	return Request{
		Subject: s, RequestedBy: requester.UserID, RequestedByName: requester.Name,
		Status: StatusPendingApproval, CurrentLevel: 1, Steps: steps, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// TotalLevels mengembalikan jumlah level.
func (r *Request) TotalLevels() int { return len(r.Steps) }

// CurrentStep mengembalikan langkah yang sedang menunggu keputusan.
func (r *Request) CurrentStep() *Step {
	for i := range r.Steps {
		if r.Steps[i].Level == r.CurrentLevel {
			return &r.Steps[i]
		}
	}
	return nil
}

// IsPending bernilai true selama permintaan menunggu keputusan.
func (r *Request) IsPending() bool { return r.Status == StatusPendingApproval }

func (r *Request) transitionTo(next Status) error {
	if !slices.Contains(requestTransitions[r.Status], next) {
		return apperr.InvalidStateTransition("approval_request", string(r.Status), string(next))
	}
	r.Status = next
	return nil
}

// Authorize menilai apakah decider boleh memutuskan level aktif, langsung atau sebagai delegasi salah satu
// delegators (user yang sedang mendelegasikan approval kepada decider). Mengembalikan user asal delegasi.
func (r *Request) Authorize(decider Principal, delegators []Principal) (*uuid.UUID, error) {
	if !r.IsPending() {
		return nil, apperr.WithMessage(apperr.CodeInvalidStateTransition, "Dokumen ini sudah tidak menunggu persetujuan.")
	}
	if decider.UserID == r.RequestedBy {
		return nil, apperr.WithMessage(apperr.CodeForbidden, "Pembuat dokumen tidak boleh menyetujui dokumennya sendiri.")
	}
	step := r.CurrentStep()
	if step == nil {
		return nil, apperr.New(apperr.CodeInternal)
	}
	if step.canDecide(decider, r.RequestedBy) {
		return nil, nil
	}
	for _, d := range delegators {
		if d.UserID != r.RequestedBy && step.canDecide(d, r.RequestedBy) {
			from := d.UserID
			return &from, nil
		}
	}
	return nil, apperr.WithMessage(apperr.CodeForbidden, "Anda bukan approver untuk level persetujuan ini.")
}

func validateNote(note string, required bool) (string, error) {
	note = strings.TrimSpace(note)
	switch {
	case required && note == "":
		return "", apperr.Validation(map[string]string{"note": "Catatan wajib diisi saat menolak"})
	case utf8.RuneCountInString(note) > MaxNoteLength:
		return "", apperr.Validation(map[string]string{"note": "Catatan maksimal 1.000 karakter"})
	}
	return note, nil
}

func (r *Request) decide(d Decision, decider Principal, delegatedFrom *uuid.UUID, note string, now time.Time) error {
	step := r.CurrentStep()
	step.Decision = &d
	step.DecidedBy = &decider.UserID
	step.DecidedByName = decider.Name
	step.DelegatedFrom = delegatedFrom
	step.Note = note
	step.DecidedAt = &now
	r.UpdatedAt = now
	return nil
}

// Approve menyetujui level aktif. Mengembalikan true jika ini level terakhir dan dokumen disetujui.
// Panggil Authorize lebih dulu.
func (r *Request) Approve(decider Principal, delegatedFrom *uuid.UUID, note string, now time.Time) (bool, error) {
	note, err := validateNote(note, false)
	if err != nil {
		return false, err
	}
	if !r.IsPending() {
		return false, apperr.InvalidStateTransition("approval_request", string(r.Status), string(StatusApproved))
	}
	if err := r.decide(DecisionApproved, decider, delegatedFrom, note, now); err != nil {
		return false, err
	}
	if r.CurrentLevel < r.TotalLevels() {
		r.CurrentLevel++
		r.CurrentStep().ActivatedAt = &now
		return false, nil
	}
	if err := r.transitionTo(StatusApproved); err != nil {
		return false, err
	}
	r.DecidedAt = &now
	return true, nil
}

// Reject menolak dokumen pada level aktif. Catatan wajib diisi.
func (r *Request) Reject(decider Principal, delegatedFrom *uuid.UUID, note string, now time.Time) error {
	note, err := validateNote(note, true)
	if err != nil {
		return err
	}
	if err := r.transitionTo(StatusRejected); err != nil {
		return err
	}
	r.DecidedAt = &now
	return r.decide(DecisionRejected, decider, delegatedFrom, note, now)
}

// Cancel membatalkan permintaan yang masih berjalan, mis. dokumen dibatalkan atau ditarik untuk direvisi.
func (r *Request) Cancel(now time.Time) error {
	if err := r.transitionTo(StatusCancelled); err != nil {
		return err
	}
	r.DecidedAt = &now
	r.UpdatedAt = now
	return nil
}

// DueForReminder memeriksa apakah level aktif belum diputuskan lebih dari interval sejak aktif atau sejak
// pengingat terakhir (NFR-11).
func (r *Request) DueForReminder(now time.Time, interval time.Duration) bool {
	step := r.CurrentStep()
	if !r.IsPending() || step == nil || step.ActivatedAt == nil {
		return false
	}
	last := *step.ActivatedAt
	if step.LastRemindedAt != nil && step.LastRemindedAt.After(last) {
		last = *step.LastRemindedAt
	}
	return now.Sub(last) >= interval
}
