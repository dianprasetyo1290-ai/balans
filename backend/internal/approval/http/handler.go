package http

import (
	"context"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"balans/internal/approval/domain"
	"balans/internal/approval/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

//go:generate go tool oapi-codegen -config ../../../api/codegen/approval.yaml ../../../api/openapi.bundle.yaml

// ApprovalHandler mengimplementasikan StrictServerInterface hasil generate.
type ApprovalHandler struct {
	svc   *service.Service
	clock clock.Clock
}

var _ StrictServerInterface = (*ApprovalHandler)(nil)

// NewHandler membuat handler.
func NewHandler(svc *service.Service, c clock.Clock) *ApprovalHandler {
	return &ApprovalHandler{svc: svc, clock: c}
}

// Register mendaftarkan route modul ke router.
func Register(r chi.Router, h *ApprovalHandler, rs *httpx.Responder) {
	strict := NewStrictHandlerWithOptions(h, nil, StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  rs.RequestError,
		ResponseErrorHandlerFunc: rs.Error,
	})
	HandlerWithOptions(strict, ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: rs.RequestError})
}

// --- Pemetaan response ------------------------------------------------------------------------

// names mengumpulkan nama user dan jabatan yang dirujuk response agar diambil sekali per request.
type names struct {
	users map[uuid.UUID]domain.Principal
	roles map[string]string
}

func (h *ApprovalHandler) resolveNames(ctx context.Context, userIDs []uuid.UUID, roles []string) (names, error) {
	users, err := h.svc.UsersByID(ctx, userIDs)
	if err != nil {
		return names{}, err
	}
	n := names{users: users, roles: map[string]string{}}
	for _, r := range roles {
		if _, ok := n.roles[r]; !ok {
			n.roles[r] = h.svc.RoleName(ctx, r)
		}
	}
	return n, nil
}

func (n names) user(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	if p, ok := n.users[*id]; ok {
		return &p.Name
	}
	unknown := "Pengguna tidak aktif"
	return &unknown
}

func requestRefs(list []domain.Request) ([]uuid.UUID, []string) {
	var ids []uuid.UUID
	var roles []string
	for _, r := range list {
		for _, st := range r.Steps {
			for _, id := range []*uuid.UUID{st.ApproverUser, st.Substitute, st.DelegatedFrom} {
				if id != nil {
					ids = append(ids, *id)
				}
			}
			roles = append(roles, st.ApproverRole)
		}
	}
	return ids, roles
}

func stepStatus(r domain.Request, st domain.Step) ApprovalStepStatus {
	switch {
	case st.Decision != nil && *st.Decision == domain.DecisionApproved:
		return ApprovalStepStatusApproved
	case st.Decision != nil:
		return ApprovalStepStatusRejected
	case r.IsPending() && st.Level == r.CurrentLevel:
		return ApprovalStepStatusPending
	case r.IsPending():
		return ApprovalStepStatusWaiting
	}
	return ApprovalStepStatusSkipped
}

func toRequest(r domain.Request, n names, actions []string) ApprovalRequest {
	steps := make([]ApprovalStep, len(r.Steps))
	for i, st := range r.Steps {
		steps[i] = ApprovalStep{
			Level: st.Level, ApproverRole: st.ApproverRole, ApproverRoleName: n.roles[st.ApproverRole],
			ApproverUserId: st.ApproverUser, ApproverUserName: n.user(st.ApproverUser),
			SubstituteUserId: st.Substitute, SubstituteUserName: n.user(st.Substitute),
			Status: stepStatus(r, st), DecidedById: st.DecidedBy, DecidedByName: st.DecidedByName,
			DelegatedFromName: n.user(st.DelegatedFrom), Note: st.Note, ActivatedAt: st.ActivatedAt, DecidedAt: st.DecidedAt,
		}
	}
	allowed := make([]ApprovalRequestAllowedActions, len(actions))
	for i, a := range actions {
		allowed[i] = ApprovalRequestAllowedActions(a)
	}
	return ApprovalRequest{
		Id: r.ID, SubjectType: string(r.Subject.Type), SubjectTypeLabel: r.Subject.Type.Label(), SubjectId: r.Subject.ID,
		SubjectLabel: r.Subject.Label, SubjectLink: r.Subject.Link, Summary: r.Subject.Summary,
		Amount: r.Subject.Amount.String(), DivisionId: r.Subject.Division, RequestedById: r.RequestedBy,
		RequestedByName: r.RequestedByName, Status: ApprovalStatus(r.Status), CurrentLevel: r.CurrentLevel,
		TotalLevels: r.TotalLevels(), Steps: steps, CreatedAt: r.CreatedAt, DecidedAt: r.DecidedAt, Version: r.Version,
		AllowedActions: allowed,
	}
}

func (h *ApprovalHandler) requests(ctx context.Context, list []domain.Request) ([]ApprovalRequest, error) {
	ids, roles := requestRefs(list)
	n, err := h.resolveNames(ctx, ids, roles)
	if err != nil {
		return nil, err
	}
	out := make([]ApprovalRequest, len(list))
	for i, r := range list {
		out[i] = toRequest(r, n, nil)
	}
	return out, nil
}

// detail memetakan satu permintaan beserta aksi yang boleh dilakukan user.
func (h *ApprovalHandler) detail(ctx context.Context, r domain.Request) (ApprovalRequest, error) {
	ids, roles := requestRefs([]domain.Request{r})
	n, err := h.resolveNames(ctx, ids, roles)
	if err != nil {
		return ApprovalRequest{}, err
	}
	var actions []string
	if r.IsPending() {
		tasks, err := h.svc.Inbox(ctx, r.Subject.Type)
		if err != nil {
			return ApprovalRequest{}, err
		}
		if slices.ContainsFunc(tasks, func(t service.Task) bool { return t.Request.ID == r.ID }) {
			actions = []string{"approve", "reject"}
		}
	}
	return toRequest(r, n, actions), nil
}

// --- Inbox & keputusan ------------------------------------------------------------------------

// ListApprovalInbox mengembalikan Persetujuan Saya.
func (h *ApprovalHandler) ListApprovalInbox(ctx context.Context, req ListApprovalInboxRequestObject) (ListApprovalInboxResponseObject, error) {
	var t domain.SubjectType
	if req.Params.SubjectType != nil {
		t = domain.SubjectType(*req.Params.SubjectType)
	}
	tasks, err := h.svc.Inbox(ctx, t)
	if err != nil {
		return nil, err
	}
	list := make([]domain.Request, len(tasks))
	for i, task := range tasks {
		list[i] = task.Request
	}
	ids, roles := requestRefs(list)
	n, err := h.resolveNames(ctx, ids, roles)
	if err != nil {
		return nil, err
	}
	items := make([]ApprovalTask, len(tasks))
	for i, task := range tasks {
		items[i] = ApprovalTask{Request: toRequest(task.Request, n, []string{"approve", "reject"})}
		if task.DelegatedFrom != nil {
			items[i].DelegatedFromUserId = &task.DelegatedFrom.UserID
			items[i].DelegatedFromName = &task.DelegatedFrom.Name
		}
	}
	return ListApprovalInbox200JSONResponse{Items: items, TotalItems: len(items)}, nil
}

// GetApprovalInboxCount mengembalikan jumlah untuk badge menu.
func (h *ApprovalHandler) GetApprovalInboxCount(ctx context.Context, _ GetApprovalInboxCountRequestObject) (GetApprovalInboxCountResponseObject, error) {
	n, err := h.svc.InboxCount(ctx)
	if err != nil {
		return nil, err
	}
	return GetApprovalInboxCount200JSONResponse{Pending: n}, nil
}

// ListApprovalRequests mengembalikan riwayat permintaan.
func (h *ApprovalHandler) ListApprovalRequests(ctx context.Context, req ListApprovalRequestsRequestObject) (ListApprovalRequestsResponseObject, error) {
	p := req.Params
	page := paging.New(p.Page, p.PageSize)
	if p.SubjectId != nil {
		if p.SubjectType == nil {
			return nil, apperr.Validation(map[string]string{"subjectType": "Jenis dokumen wajib diisi bersama subjectId"})
		}
		list, err := h.svc.ListBySubject(ctx, domain.SubjectType(*p.SubjectType), *p.SubjectId)
		if err != nil {
			return nil, err
		}
		items, err := h.requests(ctx, list)
		if err != nil {
			return nil, err
		}
		total := int64(len(items))
		return ListApprovalRequests200JSONResponse{
			Items: items, Page: 1, PageSize: max(len(items), 1), TotalItems: total, TotalPages: 1,
		}, nil
	}
	scope := service.ScopeRequested
	if p.Scope != nil {
		scope = service.ListScope(*p.Scope)
	}
	var status domain.Status
	if p.Status != nil {
		status = domain.Status(*p.Status)
	}
	var t domain.SubjectType
	if p.SubjectType != nil {
		t = domain.SubjectType(*p.SubjectType)
	}
	res, err := h.svc.ListRequests(ctx, scope, status, t, page)
	if err != nil {
		return nil, err
	}
	items, err := h.requests(ctx, res.Items)
	if err != nil {
		return nil, err
	}
	return ListApprovalRequests200JSONResponse{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, TotalItems: res.Total,
		TotalPages: res.Page.TotalPages(res.Total),
	}, nil
}

// GetApprovalRequest mengembalikan detail permintaan.
func (h *ApprovalHandler) GetApprovalRequest(ctx context.Context, req GetApprovalRequestRequestObject) (GetApprovalRequestResponseObject, error) {
	r, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, r)
	if err != nil {
		return nil, err
	}
	return GetApprovalRequest200JSONResponse(out), nil
}

// ApproveApprovalRequest menyetujui level aktif.
func (h *ApprovalHandler) ApproveApprovalRequest(ctx context.Context, req ApproveApprovalRequestRequestObject) (ApproveApprovalRequestResponseObject, error) {
	note := ""
	if req.Body.Note != nil {
		note = *req.Body.Note
	}
	r, err := h.svc.Decide(ctx, req.Id, req.Body.Version, domain.DecisionApproved, note)
	if err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, r)
	if err != nil {
		return nil, err
	}
	return ApproveApprovalRequest200JSONResponse(out), nil
}

// RejectApprovalRequest menolak dokumen.
func (h *ApprovalHandler) RejectApprovalRequest(ctx context.Context, req RejectApprovalRequestRequestObject) (RejectApprovalRequestResponseObject, error) {
	r, err := h.svc.Decide(ctx, req.Id, req.Body.Version, domain.DecisionRejected, req.Body.Note)
	if err != nil {
		return nil, err
	}
	out, err := h.detail(ctx, r)
	if err != nil {
		return nil, err
	}
	return RejectApprovalRequest200JSONResponse(out), nil
}

// --- Jenis dokumen & kebijakan ----------------------------------------------------------------

// ListApprovalSubjectTypes mengembalikan jenis dokumen.
func (h *ApprovalHandler) ListApprovalSubjectTypes(ctx context.Context, _ ListApprovalSubjectTypesRequestObject) (ListApprovalSubjectTypesResponseObject, error) {
	if _, ok := actor.From(ctx); !ok {
		return nil, apperr.New(apperr.CodeAuthSessionExpired)
	}
	types := domain.SubjectTypes()
	out := make([]ApprovalSubjectType, len(types))
	for i, t := range types {
		conds := make([]ApprovalCondition, len(t.Conditions))
		for j, c := range t.Conditions {
			conds[j] = ApprovalCondition{Code: c.Code, Label: c.Label}
		}
		out[i] = ApprovalSubjectType{Type: string(t.Type), Label: t.Label, AmountLabel: t.AmountLabel, Conditions: conds}
	}
	return ListApprovalSubjectTypes200JSONResponse(out), nil
}

func (h *ApprovalHandler) policies(ctx context.Context, list []domain.Policy) ([]ApprovalPolicy, error) {
	var ids []uuid.UUID
	var roles []string
	for _, p := range list {
		for _, id := range []*uuid.UUID{p.ApproverUser, p.Substitute} {
			if id != nil {
				ids = append(ids, *id)
			}
		}
		roles = append(roles, p.ApproverRole)
	}
	n, err := h.resolveNames(ctx, ids, roles)
	if err != nil {
		return nil, err
	}
	now := h.clock.Now()
	out := make([]ApprovalPolicy, len(list))
	for i, p := range list {
		out[i] = ApprovalPolicy{
			Id: p.ID, SubjectType: string(p.SubjectType), SubjectTypeLabel: p.SubjectType.Label(), DivisionId: p.Division,
			Level: p.Level, MinAmount: moneyString(p.MinAmount), MaxAmount: moneyString(p.MaxAmount),
			Condition: p.Condition, ApproverRole: p.ApproverRole, ApproverRoleName: n.roles[p.ApproverRole],
			ApproverUserId: p.ApproverUser, ApproverUserName: n.user(p.ApproverUser), SubstituteUserId: p.Substitute,
			SubstituteUserName: n.user(p.Substitute), ValidFrom: p.ValidFrom, ValidTo: p.ValidTo,
			IsActive: p.ActiveAt(now), UpdatedAt: p.UpdatedAt, Version: p.Version,
		}
	}
	return out, nil
}

func (h *ApprovalHandler) policy(ctx context.Context, p domain.Policy) (ApprovalPolicy, error) {
	list, err := h.policies(ctx, []domain.Policy{p})
	if err != nil {
		return ApprovalPolicy{}, err
	}
	return list[0], nil
}

func moneyString(a *money.Amount) *Money {
	if a == nil {
		return nil
	}
	s := a.String()
	return &s
}

func parseMoney(s *Money, field string, fields map[string]string) *money.Amount {
	if s == nil || *s == "" {
		return nil
	}
	a, err := money.Parse(*s)
	if err != nil {
		fields[field] = "Nominal tidak valid"
		return nil
	}
	return &a
}

func policyFields(minAmount, maxAmount *Money, condition *string, role string, user, substitute *uuid.UUID) (domain.PolicyInput, error) {
	fields := map[string]string{}
	in := domain.PolicyInput{
		MinAmount: parseMoney(minAmount, "minAmount", fields), MaxAmount: parseMoney(maxAmount, "maxAmount", fields),
		ApproverRole: role, ApproverUser: user, Substitute: substitute,
	}
	if condition != nil {
		in.Condition = *condition
	}
	if len(fields) > 0 {
		return in, apperr.Validation(fields)
	}
	return in, nil
}

// ListApprovalPolicies mengembalikan matriks persetujuan.
func (h *ApprovalHandler) ListApprovalPolicies(ctx context.Context, req ListApprovalPoliciesRequestObject) (ListApprovalPoliciesResponseObject, error) {
	var t domain.SubjectType
	if req.Params.SubjectType != nil {
		t = domain.SubjectType(*req.Params.SubjectType)
	}
	list, err := h.svc.ListPolicies(ctx, t, req.Params.IncludeEnded != nil && *req.Params.IncludeEnded)
	if err != nil {
		return nil, err
	}
	out, err := h.policies(ctx, list)
	if err != nil {
		return nil, err
	}
	return ListApprovalPolicies200JSONResponse(out), nil
}

// CreateApprovalPolicy menambah level kebijakan.
func (h *ApprovalHandler) CreateApprovalPolicy(ctx context.Context, req CreateApprovalPolicyRequestObject) (CreateApprovalPolicyResponseObject, error) {
	b := req.Body
	in, err := policyFields(b.MinAmount, b.MaxAmount, b.Condition, b.ApproverRole, b.ApproverUserId, b.SubstituteUserId)
	if err != nil {
		return nil, err
	}
	in.SubjectType, in.Division, in.Level = domain.SubjectType(b.SubjectType), b.DivisionId, b.Level
	p, err := h.svc.CreatePolicy(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.policy(ctx, p)
	if err != nil {
		return nil, err
	}
	return CreateApprovalPolicy201JSONResponse(out), nil
}

// UpdateApprovalPolicy membuat versi baru kebijakan.
func (h *ApprovalHandler) UpdateApprovalPolicy(ctx context.Context, req UpdateApprovalPolicyRequestObject) (UpdateApprovalPolicyResponseObject, error) {
	b := req.Body
	in, err := policyFields(b.MinAmount, b.MaxAmount, b.Condition, b.ApproverRole, b.ApproverUserId, b.SubstituteUserId)
	if err != nil {
		return nil, err
	}
	p, err := h.svc.UpdatePolicy(ctx, req.Id, b.Version, in)
	if err != nil {
		return nil, err
	}
	out, err := h.policy(ctx, p)
	if err != nil {
		return nil, err
	}
	return UpdateApprovalPolicy200JSONResponse(out), nil
}

// EndApprovalPolicy menghentikan kebijakan.
func (h *ApprovalHandler) EndApprovalPolicy(ctx context.Context, req EndApprovalPolicyRequestObject) (EndApprovalPolicyResponseObject, error) {
	p, err := h.svc.EndPolicy(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	out, err := h.policy(ctx, p)
	if err != nil {
		return nil, err
	}
	return EndApprovalPolicy200JSONResponse(out), nil
}

// --- Delegasi ---------------------------------------------------------------------------------

func (h *ApprovalHandler) delegations(ctx context.Context, list []domain.Delegation) ([]ApprovalDelegation, error) {
	ids := make([]uuid.UUID, 0, len(list)*2)
	for _, d := range list {
		ids = append(ids, d.From, d.To)
	}
	n, err := h.resolveNames(ctx, ids, nil)
	if err != nil {
		return nil, err
	}
	today := clock.Today(h.clock)
	out := make([]ApprovalDelegation, len(list))
	for i, d := range list {
		status := ApprovalDelegationStatusActive
		switch {
		case d.CancelledAt != nil:
			status = ApprovalDelegationStatusCancelled
		case d.ValidTo.Before(today):
			status = ApprovalDelegationStatusEnded
		case d.ValidFrom.After(today):
			status = ApprovalDelegationStatusScheduled
		}
		out[i] = ApprovalDelegation{
			Id: d.ID, FromUserId: d.From, FromUserName: *n.user(&d.From), ToUserId: d.To, ToUserName: *n.user(&d.To),
			ValidFrom: toDate(d.ValidFrom), ValidTo: toDate(d.ValidTo), Reason: d.Reason, Status: status,
			CancelledAt: d.CancelledAt, Version: d.Version,
		}
	}
	return out, nil
}

func toDate(d clock.BusinessDate) BusinessDate { return BusinessDate{Time: d.Time()} }

func fromDate(d BusinessDate) clock.BusinessDate {
	if d.IsZero() {
		return clock.BusinessDate{}
	}
	return clock.NewBusinessDate(d.Year(), d.Month(), d.Day())
}

// ListApprovalDelegations mengembalikan delegasi.
func (h *ApprovalHandler) ListApprovalDelegations(ctx context.Context, req ListApprovalDelegationsRequestObject) (ListApprovalDelegationsResponseObject, error) {
	all := req.Params.All != nil && *req.Params.All
	past := req.Params.IncludePast != nil && *req.Params.IncludePast
	list, err := h.svc.ListDelegations(ctx, all, past)
	if err != nil {
		return nil, err
	}
	out, err := h.delegations(ctx, list)
	if err != nil {
		return nil, err
	}
	return ListApprovalDelegations200JSONResponse(out), nil
}

// CreateApprovalDelegation mencatat delegasi.
func (h *ApprovalHandler) CreateApprovalDelegation(ctx context.Context, req CreateApprovalDelegationRequestObject) (CreateApprovalDelegationResponseObject, error) {
	b := req.Body
	in := domain.DelegationInput{To: b.ToUserId, ValidFrom: fromDate(b.ValidFrom), ValidTo: fromDate(b.ValidTo)}
	if b.FromUserId != nil {
		in.From = *b.FromUserId
	}
	if b.Reason != nil {
		in.Reason = *b.Reason
	}
	d, err := h.svc.CreateDelegation(ctx, in)
	if err != nil {
		return nil, err
	}
	out, err := h.delegations(ctx, []domain.Delegation{d})
	if err != nil {
		return nil, err
	}
	return CreateApprovalDelegation201JSONResponse(out[0]), nil
}

// CancelApprovalDelegation menghentikan delegasi.
func (h *ApprovalHandler) CancelApprovalDelegation(ctx context.Context, req CancelApprovalDelegationRequestObject) (CancelApprovalDelegationResponseObject, error) {
	d, err := h.svc.CancelDelegation(ctx, req.Id, req.Body.Version)
	if err != nil {
		return nil, err
	}
	out, err := h.delegations(ctx, []domain.Delegation{d})
	if err != nil {
		return nil, err
	}
	return CancelApprovalDelegation200JSONResponse(out[0]), nil
}

// ListApprovalUserOptions mencari pengguna aktif.
func (h *ApprovalHandler) ListApprovalUserOptions(ctx context.Context, req ListApprovalUserOptionsRequestObject) (ListApprovalUserOptionsResponseObject, error) {
	q := ""
	if req.Params.Q != nil {
		q = *req.Params.Q
	}
	users, err := h.svc.SearchUsers(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]ApprovalUserOption, len(users))
	for i, u := range users {
		out[i] = ApprovalUserOption{Id: u.UserID, FullName: u.Name, Email: u.Email}
	}
	return ListApprovalUserOptions200JSONResponse(out), nil
}
