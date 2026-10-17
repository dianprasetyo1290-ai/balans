package service

import (
	"context"

	"balans/internal/approval"
	"balans/internal/inventory/domain"
)

// ApprovalRegistration adalah handler keputusan approval satu jenis dokumen beserta permission bacanya.
type ApprovalRegistration struct {
	Type           approval.SubjectType
	ReadPermission string
	Handler        approval.Handler
}

// ApprovalRegistrations mengembalikan handler `internal_usage`, `disposal`, dan `return_disposition` yang dipanggil
// engine approval di transaksi keputusan (menggantikan handler pengembangan).
func (s *Service) ApprovalRegistrations() []ApprovalRegistration {
	handler := func(decide func(ctx context.Context, d approval.Decided, a domain.DocAction) error) approval.Handler {
		return approval.Handler{
			OnApproved: func(ctx context.Context, d approval.Decided) error { return decide(ctx, d, domain.ActionApprove) },
			OnRejected: func(ctx context.Context, d approval.Decided) error { return decide(ctx, d, domain.ActionReject) },
		}
	}
	return []ApprovalRegistration{
		{approval.SubjectInternalUsage, domain.PermUsageRead, handler(s.decideUsage)},
		{approval.SubjectDisposal, domain.PermDisposalRead, handler(s.decideDisposal)},
		{approval.SubjectReturnDisposition, domain.PermDispositionRead, handler(s.decideDisposition)},
	}
}
