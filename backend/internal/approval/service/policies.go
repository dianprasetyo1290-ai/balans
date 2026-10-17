package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"balans/internal/approval/domain"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
)

// policySnapshot adalah bentuk kebijakan untuk audit trail.
func policySnapshot(p domain.Policy) map[string]any {
	m := map[string]any{
		"subjectType": p.SubjectType, "divisionId": p.Division, "level": p.Level, "condition": p.Condition,
		"approverRole": p.ApproverRole, "approverUserId": p.ApproverUser, "substituteUserId": p.Substitute,
		"validFrom": p.ValidFrom, "validTo": p.ValidTo,
	}
	if p.MinAmount != nil {
		m["minAmount"] = p.MinAmount.String()
	}
	if p.MaxAmount != nil {
		m["maxAmount"] = p.MaxAmount.String()
	}
	return m
}

func policyLabel(p domain.Policy) string {
	return fmt.Sprintf("%s level %d", p.SubjectType.Label(), p.Level)
}

// ListPolicies mengembalikan matriks persetujuan.
func (s *Service) ListPolicies(ctx context.Context, subjectType domain.SubjectType, includeEnded bool) ([]domain.Policy, error) {
	if _, err := actor.Require(ctx, PermPolicyRead); err != nil {
		return nil, err
	}
	return s.Store.ListPolicies(ctx, string(subjectType), includeEnded, s.Clock.Now())
}

// validateApprovers memastikan role ada dan user approver/pengganti aktif.
func (s *Service) validateApprovers(ctx context.Context, in domain.PolicyInput) error {
	fields := map[string]string{}
	if in.ApproverRole != "" {
		if _, ok, err := s.Directory.RoleName(ctx, in.ApproverRole); err != nil {
			return err
		} else if !ok {
			fields["approverRole"] = "Jabatan tidak dikenal"
		}
	}
	check := func(id *uuid.UUID, field string) error {
		if id == nil {
			return nil
		}
		p, err := s.Directory.User(ctx, *id)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			fields[field] = "Pengguna tidak ditemukan atau nonaktif"
			return nil
		}
		if err != nil {
			return err
		}
		if field == "approverUserId" && in.ApproverRole != "" && !(domain.Step{ApproverRole: in.ApproverRole}).HoldsPosition(p) {
			fields[field] = "Pengguna tidak memegang jabatan ini"
		}
		return nil
	}
	if err := check(in.ApproverUser, "approverUserId"); err != nil {
		return err
	}
	if err := check(in.Substitute, "substituteUserId"); err != nil {
		return err
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// CreatePolicy menambah level kebijakan yang langsung berlaku.
func (s *Service) CreatePolicy(ctx context.Context, in domain.PolicyInput) (domain.Policy, error) {
	a, err := actor.Require(ctx, PermPolicyWrite)
	if err != nil {
		return domain.Policy{}, err
	}
	p, err := domain.NewPolicy(in, s.Clock.Now())
	if err != nil {
		return domain.Policy{}, err
	}
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.validateApprovers(ctx, in); err != nil {
			return err
		}
		if p.ID, err = s.Store.InsertPolicy(ctx, p, a.UserID); err != nil {
			return err
		}
		p.Version, p.UpdatedAt = 1, p.ValidFrom
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionCreate, EntityType: "approval_policy", EntityID: p.ID, EntityLabel: policyLabel(p),
			After: policySnapshot(p),
		})
	})
	return p, err
}

// UpdatePolicy menutup versi kebijakan yang berlaku dan membuat versi baru mulai sekarang (effective-dated).
// Jenis dokumen, divisi, dan level tidak berubah; dokumen yang sudah diajukan tetap memakai versi lama.
func (s *Service) UpdatePolicy(ctx context.Context, id uuid.UUID, version int, in domain.PolicyInput) (domain.Policy, error) {
	a, err := actor.Require(ctx, PermPolicyWrite)
	if err != nil {
		return domain.Policy{}, err
	}
	var next domain.Policy
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.GetPolicyForUpdate(ctx, id)
		if err != nil {
			return err
		}
		if cur.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		in.SubjectType, in.Division, in.Level = cur.SubjectType, cur.Division, cur.Level
		now := s.Clock.Now()
		if next, err = domain.NewPolicy(in, now); err != nil {
			return err
		}
		if err := s.validateApprovers(ctx, in); err != nil {
			return err
		}
		before := cur
		if err := cur.End(now); err != nil {
			return err
		}
		if err := s.Store.EndPolicy(ctx, cur, version, a.UserID); err != nil {
			return err
		}
		if next.ID, err = s.Store.InsertPolicy(ctx, next, a.UserID); err != nil {
			return err
		}
		next.Version, next.UpdatedAt = 1, now
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionUpdate, EntityType: "approval_policy", EntityID: next.ID, EntityLabel: policyLabel(next),
			Before: policySnapshot(before), After: policySnapshot(next), Note: "Versi baru menggantikan kebijakan " + before.ID.String(),
		})
	})
	return next, err
}

// EndPolicy menghentikan level kebijakan mulai sekarang.
func (s *Service) EndPolicy(ctx context.Context, id uuid.UUID, version int) (domain.Policy, error) {
	a, err := actor.Require(ctx, PermPolicyWrite)
	if err != nil {
		return domain.Policy{}, err
	}
	var p domain.Policy
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if p, err = s.Store.GetPolicyForUpdate(ctx, id); err != nil {
			return err
		}
		if p.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		before := p
		if err := p.End(s.Clock.Now()); err != nil {
			return err
		}
		if err := s.Store.EndPolicy(ctx, p, version, a.UserID); err != nil {
			return err
		}
		p.Version, p.UpdatedAt = p.Version+1, *p.ValidTo
		return s.Audit.Record(ctx, audit.Entry{
			Action: audit.ActionDelete, EntityType: "approval_policy", EntityID: p.ID, EntityLabel: policyLabel(p),
			Before: policySnapshot(before), After: policySnapshot(p), Note: "Kebijakan dihentikan",
		})
	})
	return p, err
}
