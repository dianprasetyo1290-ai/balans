package domain

import (
	"cmp"
	"slices"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
)

// MaxLevel adalah jumlah level maksimum per jenis dokumen.
const MaxLevel = 9

// Policy adalah satu level kebijakan approval yang berlaku pada rentang waktu tertentu (effective-dated).
type Policy struct {
	ID          uuid.UUID
	SubjectType SubjectType
	// Division null = berlaku untuk semua divisi. Kebijakan per divisi menggantikan level yang sama.
	Division *uuid.UUID
	Level    int
	// MinAmount: level berlaku jika nominal > MinAmount (null = semua nominal).
	MinAmount *money.Amount
	// MaxAmount: level berlaku jika nominal <= MaxAmount (null = tanpa batas atas).
	MaxAmount *money.Amount
	Condition string
	// ApproverRole adalah jabatan approver; ApproverUser mempersempit ke satu orang.
	ApproverRole string
	ApproverUser *uuid.UUID
	// Substitute adalah pengganti saat pembuat dokumen sendiri adalah approver level ini.
	Substitute *uuid.UUID
	ValidFrom  time.Time
	ValidTo    *time.Time
	UpdatedAt  time.Time
	Version    int
}

// PolicyInput adalah isian kebijakan dari Pengaturan › Matriks Persetujuan.
type PolicyInput struct {
	SubjectType  SubjectType
	Division     *uuid.UUID
	Level        int
	MinAmount    *money.Amount
	MaxAmount    *money.Amount
	Condition    string
	ApproverRole string
	ApproverUser *uuid.UUID
	Substitute   *uuid.UUID
}

// Validate memeriksa isian kebijakan. Keberadaan role dan user diperiksa service.
func (in PolicyInput) Validate() error {
	fields := map[string]string{}
	info, ok := LookupSubjectType(in.SubjectType)
	if !ok {
		fields["subjectType"] = "Jenis dokumen tidak dikenal"
	}
	if in.Level < 1 || in.Level > MaxLevel {
		fields["level"] = "Level harus 1 sampai 9"
	}
	if in.MinAmount != nil && in.MinAmount.IsNegative() {
		fields["minAmount"] = "Batas bawah tidak boleh negatif"
	}
	if in.MaxAmount != nil && !in.MaxAmount.IsPositive() {
		fields["maxAmount"] = "Batas atas harus lebih dari nol"
	}
	if in.MinAmount != nil && in.MaxAmount != nil && in.MaxAmount.Cmp(*in.MinAmount) <= 0 {
		fields["maxAmount"] = "Batas atas harus lebih besar dari batas bawah"
	}
	if in.Condition != "" && ok && !info.HasCondition(in.Condition) {
		fields["condition"] = "Syarat tidak berlaku untuk jenis dokumen ini"
	}
	if in.ApproverRole == "" {
		fields["approverRole"] = "Jabatan approver wajib diisi"
	}
	if in.ApproverUser != nil && in.Substitute != nil && *in.ApproverUser == *in.Substitute {
		fields["substituteUserId"] = "Pengganti harus berbeda dari approver"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// NewPolicy membuat kebijakan yang berlaku mulai validFrom.
func NewPolicy(in PolicyInput, validFrom time.Time) (Policy, error) {
	if err := in.Validate(); err != nil {
		return Policy{}, err
	}
	return Policy{
		SubjectType: in.SubjectType, Division: in.Division, Level: in.Level, MinAmount: in.MinAmount,
		MaxAmount: in.MaxAmount, Condition: in.Condition, ApproverRole: in.ApproverRole,
		ApproverUser: in.ApproverUser, Substitute: in.Substitute, ValidFrom: validFrom,
	}, nil
}

// ActiveAt memeriksa apakah kebijakan berlaku pada waktu t.
func (p Policy) ActiveAt(t time.Time) bool {
	return !t.Before(p.ValidFrom) && (p.ValidTo == nil || t.Before(*p.ValidTo))
}

// End menutup kebijakan pada waktu at. Dokumen yang sudah diajukan tetap memakai langkah snapshot-nya.
func (p *Policy) End(at time.Time) error {
	if p.ValidTo != nil && !at.Before(*p.ValidTo) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Kebijakan ini sudah tidak berlaku.")
	}
	if !at.After(p.ValidFrom) {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Kebijakan belum mulai berlaku.")
	}
	p.ValidTo = &at
	return nil
}

// Applies memeriksa nominal dan syarat dokumen terhadap kebijakan.
func (p Policy) Applies(s Subject) bool {
	if p.MinAmount != nil && s.Amount.Cmp(*p.MinAmount) <= 0 {
		return false
	}
	if p.MaxAmount != nil && s.Amount.Cmp(*p.MaxAmount) > 0 {
		return false
	}
	if p.Condition != "" && !slices.Contains(s.Flags, p.Condition) {
		return false
	}
	return true
}

// SelectPolicies memilih level yang berlaku untuk dokumen pada waktu submit, berurutan menurut level.
// Kebijakan per divisi menggantikan kebijakan umum di level yang sama.
func SelectPolicies(policies []Policy, s Subject, at time.Time) []Policy {
	byLevel := map[int]Policy{}
	for _, p := range policies {
		if p.SubjectType != s.Type || !p.ActiveAt(at) {
			continue
		}
		switch {
		case p.Division == nil:
			if cur, ok := byLevel[p.Level]; ok && cur.Division != nil {
				continue
			}
		case s.Division == nil || *p.Division != *s.Division:
			continue
		}
		byLevel[p.Level] = p
	}
	selected := make([]Policy, 0, len(byLevel))
	for _, p := range byLevel {
		if p.Applies(s) {
			selected = append(selected, p)
		}
	}
	slices.SortFunc(selected, func(a, b Policy) int { return cmp.Compare(a.Level, b.Level) })
	return selected
}
