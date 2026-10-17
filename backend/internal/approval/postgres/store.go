// Package postgres adalah repository modul approval.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"balans/internal/approval/domain"
	"balans/internal/approval/postgres/sqlcgen"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

// Store menyimpan kebijakan, permintaan, langkah, dan delegasi approval.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore membuat Store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) q(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.Conn(ctx, s.pool))
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func uuidPtr(id uuid.NullUUID) *uuid.UUID {
	if !id.Valid {
		return nil
	}
	v := id.UUID
	return &v
}

func nullAmount(a *money.Amount) decimal.NullDecimal {
	if a == nil {
		return decimal.NullDecimal{}
	}
	return decimal.NullDecimal{Decimal: a.Decimal(), Valid: true}
}

func amountPtr(d decimal.NullDecimal) *money.Amount {
	if !d.Valid {
		return nil
	}
	a := money.FromDecimal(d.Decimal)
	return &a
}

func notFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(apperr.CodeNotFound, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}

// --- Kebijakan --------------------------------------------------------------------------------

func toPolicy(r sqlcgen.ApprovalPolicy) domain.Policy {
	return domain.Policy{
		ID: r.ID, SubjectType: domain.SubjectType(r.SubjectType), Division: uuidPtr(r.DivisionID), Level: int(r.Level),
		MinAmount: amountPtr(r.MinAmount), MaxAmount: amountPtr(r.MaxAmount), Condition: r.Condition,
		ApproverRole: r.ApproverRoleCode, ApproverUser: uuidPtr(r.ApproverUserID), Substitute: uuidPtr(r.SubstituteUserID),
		ValidFrom: r.ValidFrom, ValidTo: r.ValidTo, UpdatedAt: r.UpdatedAt, Version: int(r.Version),
	}
}

func toPolicies(rows []sqlcgen.ApprovalPolicy) []domain.Policy {
	out := make([]domain.Policy, len(rows))
	for i, r := range rows {
		out[i] = toPolicy(r)
	}
	return out
}

// ListPolicies mengembalikan kebijakan; includeEnded memuat versi yang sudah ditutup.
func (s *Store) ListPolicies(ctx context.Context, subjectType string, includeEnded bool, at time.Time) ([]domain.Policy, error) {
	rows, err := s.q(ctx).ListPolicies(ctx, sqlcgen.ListPoliciesParams{
		SubjectType: pgtype.Text{String: subjectType, Valid: subjectType != ""}, IncludeEnded: includeEnded, At: at,
	})
	if err != nil {
		return nil, fmt.Errorf("list approval policies: %w", err)
	}
	return toPolicies(rows), nil
}

// ActivePolicies mengembalikan kebijakan jenis dokumen yang berlaku pada waktu at.
func (s *Store) ActivePolicies(ctx context.Context, t domain.SubjectType, at time.Time) ([]domain.Policy, error) {
	rows, err := s.q(ctx).ListActivePoliciesBySubjectType(ctx, sqlcgen.ListActivePoliciesBySubjectTypeParams{SubjectType: string(t), At: at})
	if err != nil {
		return nil, fmt.Errorf("list active approval policies: %w", err)
	}
	return toPolicies(rows), nil
}

// GetPolicyForUpdate mengunci satu kebijakan.
func (s *Store) GetPolicyForUpdate(ctx context.Context, id uuid.UUID) (domain.Policy, error) {
	r, err := s.q(ctx).GetPolicyForUpdate(ctx, id)
	if err != nil {
		return domain.Policy{}, notFound(err, "get approval policy")
	}
	return toPolicy(r), nil
}

// isConstraintViolation mendeteksi pelanggaran constraint tertentu (23505 unik, 23P01 EXCLUDE tumpang tindih).
func isConstraintViolation(err error, code, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code && pgErr.ConstraintName == constraint
}

// InsertPolicy menyimpan kebijakan baru. Level yang sudah berlaku di periode yang sama ditolak.
func (s *Store) InsertPolicy(ctx context.Context, p domain.Policy, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertPolicy(ctx, sqlcgen.InsertPolicyParams{
		SubjectType: string(p.SubjectType), DivisionID: nullUUID(p.Division), Level: int32(p.Level), //nolint:gosec // level 1–9
		MinAmount: nullAmount(p.MinAmount), MaxAmount: nullAmount(p.MaxAmount), Condition: p.Condition,
		ApproverRoleCode: p.ApproverRole, ApproverUserID: nullUUID(p.ApproverUser), SubstituteUserID: nullUUID(p.Substitute),
		ValidFrom: p.ValidFrom, ActorID: actorID,
	})
	if isConstraintViolation(err, "23P01", "ex_approval_policies_period") {
		return uuid.Nil, apperr.Validation(map[string]string{"level": "Level ini sudah diatur untuk jenis dokumen dan divisi yang sama"})
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert approval policy: %w", err)
	}
	return id, nil
}

// EndPolicy menutup kebijakan dengan optimistic locking.
func (s *Store) EndPolicy(ctx context.Context, p domain.Policy, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).EndPolicy(ctx, sqlcgen.EndPolicyParams{
		ID: p.ID, ValidTo: p.ValidTo, Version: int32(version), ActorID: actorID, //nolint:gosec // versi kecil
	})
	if err != nil {
		return fmt.Errorf("end approval policy: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}

// --- Permintaan & langkah ---------------------------------------------------------------------

func toRequest(r sqlcgen.ApprovalRequest) domain.Request {
	return domain.Request{
		ID: r.ID,
		Subject: domain.Subject{
			Type: domain.SubjectType(r.SubjectType), ID: r.SubjectID, Label: r.SubjectLabel, Link: r.SubjectLink,
			Summary: r.Summary, Amount: money.FromDecimal(r.Amount), Division: uuidPtr(r.DivisionID), Flags: r.Flags,
		},
		RequestedBy: r.RequestedBy, RequestedByName: r.RequestedByName, Status: domain.Status(r.Status),
		CurrentLevel: int(r.CurrentLevel), DecidedAt: r.DecidedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		Version: int(r.Version),
	}
}

func toStep(r sqlcgen.ApprovalStep) domain.Step {
	st := domain.Step{
		ID: r.ID, Level: int(r.Level), PolicyID: r.PolicyID, ApproverRole: r.ApproverRoleCode,
		ApproverUser: uuidPtr(r.ApproverUserID), Substitute: uuidPtr(r.SubstituteUserID), Division: uuidPtr(r.DivisionID),
		RequesterIsApprover: r.RequesterIsApprover, DecidedBy: uuidPtr(r.DecidedBy), DecidedByName: r.DecidedByName,
		DelegatedFrom: uuidPtr(r.DelegatedFromUserID), Note: r.Note, ActivatedAt: r.ActivatedAt, DecidedAt: r.DecidedAt,
		LastRemindedAt: r.LastRemindedAt,
	}
	if r.Decision.Valid {
		d := domain.Decision(r.Decision.String)
		st.Decision = &d
	}
	return st
}

// withSteps memuat langkah untuk sekumpulan permintaan dalam satu query.
func (s *Store) withSteps(ctx context.Context, rows []sqlcgen.ApprovalRequest) ([]domain.Request, error) {
	if len(rows) == 0 {
		return []domain.Request{}, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	steps, err := s.q(ctx).ListStepsByRequests(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list approval steps: %w", err)
	}
	byRequest := map[uuid.UUID][]domain.Step{}
	for _, st := range steps {
		byRequest[st.ApprovalRequestID] = append(byRequest[st.ApprovalRequestID], toStep(st))
	}
	out := make([]domain.Request, len(rows))
	for i, r := range rows {
		out[i] = toRequest(r)
		out[i].Steps = byRequest[r.ID]
	}
	return out, nil
}

func (s *Store) one(ctx context.Context, row sqlcgen.ApprovalRequest) (domain.Request, error) {
	list, err := s.withSteps(ctx, []sqlcgen.ApprovalRequest{row})
	if err != nil {
		return domain.Request{}, err
	}
	return list[0], nil
}

// CreateRequest menyimpan permintaan beserta langkahnya. Dokumen yang masih menunggu persetujuan ditolak.
func (s *Store) CreateRequest(ctx context.Context, r domain.Request) (uuid.UUID, error) {
	flags := r.Subject.Flags
	if flags == nil {
		flags = []string{}
	}
	id, err := s.q(ctx).InsertRequest(ctx, sqlcgen.InsertRequestParams{
		SubjectType: string(r.Subject.Type), SubjectID: r.Subject.ID, SubjectLabel: r.Subject.Label,
		SubjectLink: r.Subject.Link, Summary: r.Subject.Summary, Amount: r.Subject.Amount.Decimal(),
		DivisionID: nullUUID(r.Subject.Division), Flags: flags, RequestedBy: r.RequestedBy,
		RequestedByName: r.RequestedByName, Status: string(r.Status), CurrentLevel: int32(r.CurrentLevel), //nolint:gosec // level 1–9
		TotalLevels: int32(r.TotalLevels()), Now: r.CreatedAt, //nolint:gosec // level 1–9
	})
	if isConstraintViolation(err, "23505", "uq_approval_requests_active_subject") {
		return uuid.Nil, apperr.WithMessage(apperr.CodeInvalidStateTransition, "Dokumen ini masih menunggu persetujuan.")
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert approval request: %w", err)
	}
	for _, st := range r.Steps {
		if err := s.q(ctx).InsertStep(ctx, sqlcgen.InsertStepParams{
			ApprovalRequestID: id, Level: int32(st.Level), PolicyID: st.PolicyID, ApproverRoleCode: st.ApproverRole, //nolint:gosec // level 1–9
			ApproverUserID: nullUUID(st.ApproverUser), SubstituteUserID: nullUUID(st.Substitute),
			DivisionID: nullUUID(st.Division), RequesterIsApprover: st.RequesterIsApprover, ActivatedAt: st.ActivatedAt,
		}); err != nil {
			return uuid.Nil, fmt.Errorf("insert approval step: %w", err)
		}
	}
	return id, nil
}

// GetRequest mengembalikan permintaan beserta langkahnya; forUpdate mengunci baris permintaan.
func (s *Store) GetRequest(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Request, error) {
	var (
		row sqlcgen.ApprovalRequest
		err error
	)
	if forUpdate {
		row, err = s.q(ctx).GetRequestForUpdate(ctx, id)
	} else {
		row, err = s.q(ctx).GetRequest(ctx, id)
	}
	if err != nil {
		return domain.Request{}, notFound(err, "get approval request")
	}
	return s.one(ctx, row)
}

// PendingBySubjectForUpdate mengunci permintaan yang masih berjalan untuk dokumen; ok false jika tidak ada.
func (s *Store) PendingBySubjectForUpdate(ctx context.Context, t domain.SubjectType, subjectID uuid.UUID) (domain.Request, bool, error) {
	row, err := s.q(ctx).GetPendingRequestBySubjectForUpdate(ctx, sqlcgen.GetPendingRequestBySubjectForUpdateParams{
		SubjectType: string(t), SubjectID: subjectID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Request{}, false, nil
	}
	if err != nil {
		return domain.Request{}, false, fmt.Errorf("get pending approval request: %w", err)
	}
	r, err := s.one(ctx, row)
	return r, err == nil, err
}

// ListBySubject mengembalikan riwayat permintaan satu dokumen, terbaru lebih dulu.
func (s *Store) ListBySubject(ctx context.Context, t domain.SubjectType, subjectID uuid.UUID) ([]domain.Request, error) {
	rows, err := s.q(ctx).ListRequestsBySubject(ctx, sqlcgen.ListRequestsBySubjectParams{SubjectType: string(t), SubjectID: subjectID})
	if err != nil {
		return nil, fmt.Errorf("list approval requests by subject: %w", err)
	}
	return s.withSteps(ctx, rows)
}

// RequestFilter adalah kriteria daftar permintaan.
type RequestFilter struct {
	RequestedBy *uuid.UUID
	DecidedBy   *uuid.UUID
	Status      domain.Status
	SubjectType domain.SubjectType
}

// ListRequests mengembalikan permintaan sesuai filter, terbaru lebih dulu.
func (s *Store) ListRequests(ctx context.Context, f RequestFilter, page paging.Page) (paging.Result[domain.Request], error) {
	status := pgtype.Text{String: string(f.Status), Valid: f.Status != ""}
	subject := pgtype.Text{String: string(f.SubjectType), Valid: f.SubjectType != ""}
	rows, err := s.q(ctx).ListRequests(ctx, sqlcgen.ListRequestsParams{
		RequestedBy: nullUUID(f.RequestedBy), DecidedBy: nullUUID(f.DecidedBy), Status: status, SubjectType: subject,
		RowLimit: page.Limit(), RowOffset: page.Offset(),
	})
	if err != nil {
		return paging.Result[domain.Request]{}, fmt.Errorf("list approval requests: %w", err)
	}
	total, err := s.q(ctx).CountRequests(ctx, sqlcgen.CountRequestsParams{
		RequestedBy: nullUUID(f.RequestedBy), DecidedBy: nullUUID(f.DecidedBy), Status: status, SubjectType: subject,
	})
	if err != nil {
		return paging.Result[domain.Request]{}, fmt.Errorf("count approval requests: %w", err)
	}
	items, err := s.withSteps(ctx, rows)
	if err != nil {
		return paging.Result[domain.Request]{}, err
	}
	return paging.Result[domain.Request]{Items: items, Page: page, Total: total}, nil
}

// PendingCandidates menyaring kasar permintaan yang mungkin bisa diputuskan user atau role tertentu.
func (s *Store) PendingCandidates(ctx context.Context, userIDs []uuid.UUID, roleCodes []string) ([]domain.Request, error) {
	if roleCodes == nil {
		roleCodes = []string{}
	}
	rows, err := s.q(ctx).ListPendingCandidates(ctx, sqlcgen.ListPendingCandidatesParams{UserIds: userIDs, RoleCodes: roleCodes})
	if err != nil {
		return nil, fmt.Errorf("list pending approval candidates: %w", err)
	}
	return s.withSteps(ctx, rows)
}

// DueForReminder mengembalikan permintaan yang level aktifnya belum diputuskan sejak sebelum waktu before.
func (s *Store) DueForReminder(ctx context.Context, before time.Time) ([]domain.Request, error) {
	rows, err := s.q(ctx).ListPendingDueForReminder(ctx, before)
	if err != nil {
		return nil, fmt.Errorf("list approval requests due for reminder: %w", err)
	}
	return s.withSteps(ctx, rows)
}

// SaveRequest menyimpan status permintaan (optimistic locking terhadap version) dan semua langkahnya.
func (s *Store) SaveRequest(ctx context.Context, r domain.Request, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).UpdateRequest(ctx, sqlcgen.UpdateRequestParams{
		ID: r.ID, Status: string(r.Status), CurrentLevel: int32(r.CurrentLevel), DecidedAt: r.DecidedAt, //nolint:gosec // level 1–9
		Now: r.UpdatedAt, ActorID: actorID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return fmt.Errorf("update approval request: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return s.SaveSteps(ctx, r.Steps)
}

// SaveSteps menyimpan keputusan, aktivasi, dan waktu pengingat langkah.
func (s *Store) SaveSteps(ctx context.Context, steps []domain.Step) error {
	for _, st := range steps {
		decision := pgtype.Text{}
		if st.Decision != nil {
			decision = pgtype.Text{String: string(*st.Decision), Valid: true}
		}
		if err := s.q(ctx).UpdateStep(ctx, sqlcgen.UpdateStepParams{
			ID: st.ID, Decision: decision, DecidedBy: nullUUID(st.DecidedBy), DecidedByName: st.DecidedByName,
			DelegatedFromUserID: nullUUID(st.DelegatedFrom), Note: st.Note, ActivatedAt: st.ActivatedAt,
			DecidedAt: st.DecidedAt, LastRemindedAt: st.LastRemindedAt,
		}); err != nil {
			return fmt.Errorf("update approval step: %w", err)
		}
	}
	return nil
}

// --- Delegasi ---------------------------------------------------------------------------------

func toDelegation(r sqlcgen.ApprovalDelegation) domain.Delegation {
	return domain.Delegation{
		ID: r.ID, From: r.FromUserID, To: r.ToUserID, ValidFrom: r.ValidFrom, ValidTo: r.ValidTo, Reason: r.Reason,
		CancelledAt: r.CancelledAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, Version: int(r.Version),
	}
}

func toDelegations(rows []sqlcgen.ApprovalDelegation) []domain.Delegation {
	out := make([]domain.Delegation, len(rows))
	for i, r := range rows {
		out[i] = toDelegation(r)
	}
	return out
}

// InsertDelegation menyimpan delegasi. Delegasi yang tumpang tindih untuk user yang sama ditolak.
func (s *Store) InsertDelegation(ctx context.Context, d domain.Delegation, actorID uuid.UUID) (uuid.UUID, error) {
	id, err := s.q(ctx).InsertDelegation(ctx, sqlcgen.InsertDelegationParams{
		FromUserID: d.From, ToUserID: d.To, ValidFrom: d.ValidFrom, ValidTo: d.ValidTo, Reason: d.Reason, ActorID: actorID,
	})
	if isConstraintViolation(err, "23P01", "ex_approval_delegations_period") {
		return uuid.Nil, apperr.Validation(map[string]string{"validFrom": "Sudah ada delegasi lain pada rentang tanggal ini"})
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert approval delegation: %w", err)
	}
	return id, nil
}

// GetDelegationForUpdate mengunci satu delegasi.
func (s *Store) GetDelegationForUpdate(ctx context.Context, id uuid.UUID) (domain.Delegation, error) {
	r, err := s.q(ctx).GetDelegationForUpdate(ctx, id)
	if err != nil {
		return domain.Delegation{}, notFound(err, "get approval delegation")
	}
	return toDelegation(r), nil
}

// ListDelegations mengembalikan delegasi yang melibatkan user (nil = semua user).
func (s *Store) ListDelegations(ctx context.Context, userID *uuid.UUID, includePast bool, today clock.BusinessDate) ([]domain.Delegation, error) {
	rows, err := s.q(ctx).ListDelegations(ctx, sqlcgen.ListDelegationsParams{UserID: nullUUID(userID), IncludePast: includePast, Today: today})
	if err != nil {
		return nil, fmt.Errorf("list approval delegations: %w", err)
	}
	return toDelegations(rows), nil
}

// ActiveDelegationsTo mengembalikan delegasi yang sedang berlaku kepada user.
func (s *Store) ActiveDelegationsTo(ctx context.Context, to uuid.UUID, today clock.BusinessDate) ([]domain.Delegation, error) {
	rows, err := s.q(ctx).ListActiveDelegationsTo(ctx, sqlcgen.ListActiveDelegationsToParams{ToUserID: to, Today: today})
	if err != nil {
		return nil, fmt.Errorf("list delegations to user: %w", err)
	}
	return toDelegations(rows), nil
}

// ActiveDelegationsFrom mengembalikan delegasi yang sedang berlaku dari sekumpulan user.
func (s *Store) ActiveDelegationsFrom(ctx context.Context, from []uuid.UUID, today clock.BusinessDate) ([]domain.Delegation, error) {
	if len(from) == 0 {
		return nil, nil
	}
	rows, err := s.q(ctx).ListActiveDelegationsFrom(ctx, sqlcgen.ListActiveDelegationsFromParams{FromUserIds: from, Today: today})
	if err != nil {
		return nil, fmt.Errorf("list delegations from users: %w", err)
	}
	return toDelegations(rows), nil
}

// CancelDelegation menyimpan pembatalan delegasi dengan optimistic locking.
func (s *Store) CancelDelegation(ctx context.Context, d domain.Delegation, version int, actorID uuid.UUID) error {
	n, err := s.q(ctx).CancelDelegation(ctx, sqlcgen.CancelDelegationParams{
		ID: d.ID, Now: *d.CancelledAt, ActorID: actorID, Version: int32(version), //nolint:gosec // versi kecil
	})
	if err != nil {
		return fmt.Errorf("cancel approval delegation: %w", err)
	}
	if n == 0 {
		return apperr.New(apperr.CodeVersionConflict)
	}
	return nil
}
