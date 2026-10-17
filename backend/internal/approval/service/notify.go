package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"balans/internal/approval/domain"
)

// Jenis notifikasi in-app (sama dengan konstanta modul notification).
const (
	kindPending  = "approval.pending"
	kindReminder = "approval.reminder"
	kindDecided  = "approval.decided"
)

// inboxPath adalah halaman Persetujuan Saya.
const inboxPath = "/approvals"

func (s *Service) link(req domain.Request) string {
	if req.Subject.Link != "" {
		return req.Subject.Link
	}
	return inboxPath
}

// describe menyusun ringkasan dokumen untuk notifikasi dan email.
func describe(req domain.Request) string {
	parts := []string{req.Subject.Type.Label() + " dari " + req.RequestedByName}
	if req.Subject.Amount.IsPositive() {
		parts = append(parts, req.Subject.Amount.Rupiah())
	}
	parts = append(parts, fmt.Sprintf("Level %d dari %d", req.CurrentLevel, req.TotalLevels()))
	if req.Subject.Summary != "" {
		parts = append(parts, req.Subject.Summary)
	}
	return strings.Join(parts, " · ")
}

// notifyApprovers memberi tahu approver level aktif lewat notifikasi in-app dan email (NFR-11).
func (s *Service) notifyApprovers(ctx context.Context, req domain.Request, reminder bool) error {
	step := req.CurrentStep()
	if step == nil {
		return nil
	}
	people, err := s.candidates(ctx, req, *step)
	if err != nil {
		return err
	}
	if len(people) == 0 {
		s.Log.WarnContext(ctx, "approval step has no active approver", "approval_request_id", req.ID, "level", step.Level)
		return nil
	}
	kind, title := kindPending, "Menunggu persetujuan Anda: "+req.Subject.Label
	if reminder {
		kind, title = kindReminder, "Pengingat persetujuan: "+req.Subject.Label
	}
	body := describe(req)
	ids := make([]uuid.UUID, 0, len(people))
	for _, p := range people {
		ids = append(ids, p.UserID)
	}
	if err := s.Notifier.Notify(ctx, ids, kind, title, body, inboxPath); err != nil {
		return err
	}
	text := fmt.Sprintf("%s\n\n%s\n\nBuka Persetujuan Saya untuk menyetujui atau menolak:\n%s%s\n",
		title, body, s.BaseURL, inboxPath)
	for _, p := range people {
		if p.Email == "" {
			continue
		}
		if _, err := s.Notifier.QueueEmail(ctx, p.Email, title, text); err != nil {
			return err
		}
	}
	return nil
}

// notifyRequester memberi tahu pembuat dokumen hasil keputusan akhir.
func (s *Service) notifyRequester(ctx context.Context, req domain.Request, decision domain.Decision) error {
	step := req.CurrentStep()
	title := req.Subject.Label + " disetujui"
	body := req.Subject.Type.Label() + " disetujui " + step.DecidedByName
	if decision == domain.DecisionRejected {
		title = req.Subject.Label + " ditolak"
		body = fmt.Sprintf("%s ditolak %s di level %d: %s", req.Subject.Type.Label(), step.DecidedByName, step.Level, step.Note)
	}
	if err := s.Notifier.Notify(ctx, []uuid.UUID{req.RequestedBy}, kindDecided, title, body, s.link(req)); err != nil {
		return err
	}
	requester, err := s.Directory.User(ctx, req.RequestedBy)
	if err != nil || requester.Email == "" {
		return nil //nolint:nilerr // pembuat yang sudah nonaktif cukup mendapat notifikasi in-app
	}
	_, err = s.Notifier.QueueEmail(ctx, requester.Email, title, fmt.Sprintf("%s\n\n%s\n\n%s%s\n", title, body, s.BaseURL, s.link(req)))
	return err
}
