package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/money"
)

// registerDevApprovalHandlers dipakai hanya di APP_ENV=local selama modul dokumen (PO, PR, dll.) belum
// dibangun, agar inbox persetujuan bisa didemokan. Modul dokumen yang mendaftarkan handler-nya sendiri
// menggantikan handler ini.
func registerDevApprovalHandlers(m *approval.Module, log *slog.Logger) {
	logDecision := func(ctx context.Context, d approval.Decided) error {
		log.InfoContext(ctx, "dev approval decided", "subject_type", d.Subject.Type, "subject", d.Subject.Label, "decision", d.Decision)
		return nil
	}
	for _, t := range approval.SubjectTypes() {
		m.RegisterHandler(t, "", approval.Handler{OnApproved: logDecision, OnRejected: logDecision})
	}
}

type devApprovalSample struct {
	requesterRole string
	subject       approval.Subject
}

// devApprovalSamples adalah dokumen contoh yang menunggu persetujuan untuk demo lokal.
func devApprovalSamples() []devApprovalSample {
	sample := func(t approval.SubjectType, label, summary string, amount int64, flags ...string) approval.Subject {
		return approval.Subject{
			Type: t, ID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("balans-dev/"+label)), Label: label,
			Summary: summary, Amount: money.FromInt(amount), Flags: flags,
		}
	}
	// PR/PO dan pengajuan biaya tidak lagi disemai: modul procurement dan accounting mendaftarkan handler-nya
	// sendiri dan menolak dokumen yang tidak ada di tabelnya.
	return []devApprovalSample{
		{"warehouse_staff", sample(approval.SubjectReturnDisposition, "RJ/2026/10/90001", "Retur MT · 3 baris pemusnahan", 1_750_000, approval.ConditionHasDisposal)},
	}
}

// seedDevApprovals mengajukan dokumen contoh. Dokumen yang masih menunggu dilewati, sehingga aman diulang.
func (a *App) seedDevApprovals(ctx context.Context) (int, error) {
	if a.approval == nil {
		return 0, nil
	}
	created := 0
	for _, s := range devApprovalSamples() {
		users, err := a.identity.UsersWithRole(ctx, s.requesterRole, nil)
		if err != nil {
			return created, err
		}
		if len(users) == 0 {
			continue
		}
		u := users[0]
		userCtx := actor.With(ctx, actor.Actor{UserID: u.ID, FullName: u.FullName, Email: u.Email})
		_, err = a.approval.Request(userCtx, s.subject)
		switch {
		case apperr.HasCode(err, apperr.CodeInvalidStateTransition):
			continue
		case err != nil:
			return created, fmt.Errorf("seed approval %s: %w", s.subject.Label, err)
		}
		created++
	}
	return created, nil
}
