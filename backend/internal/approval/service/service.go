// Package service berisi use case engine approval: pengajuan, keputusan, inbox, kebijakan, delegasi,
// dan pengingat (BRD 5.17, NFR-11, Technical Guideline 5.5).
package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"balans/internal/approval/domain"
	"balans/internal/approval/postgres"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// Permission modul approval (Desain Role & Approval bagian 3.1).
const (
	PermPolicyRead  = "approval.policy.read"
	PermPolicyWrite = "approval.policy.write"
)

// Directory membaca user dan role dari modul identity.
type Directory interface {
	// User mengembalikan user aktif beserta role-nya; NOT_FOUND jika tidak ada atau nonaktif.
	User(ctx context.Context, id uuid.UUID) (domain.Principal, error)
	// UsersWithRole mengembalikan user aktif dengan role; division membatasi role ber-scope divisi.
	UsersWithRole(ctx context.Context, role string, division *uuid.UUID) ([]domain.Principal, error)
	// RoleName mengembalikan nama role; ok false jika kode tidak dikenal.
	RoleName(ctx context.Context, code string) (string, bool, error)
	// SearchUsers mencari user aktif berdasarkan nama atau email.
	SearchUsers(ctx context.Context, q string, limit int) ([]domain.Principal, error)
}

// Notifier mengirim notifikasi in-app dan email lewat modul notification.
type Notifier interface {
	Notify(ctx context.Context, userIDs []uuid.UUID, kind, title, body, link string) error
	QueueEmail(ctx context.Context, to, subject, text string) (uuid.UUID, error)
}

// Decided adalah keputusan akhir yang diteruskan ke modul pemilik dokumen.
type Decided struct {
	RequestID uuid.UUID
	Subject   domain.Subject
	Decision  domain.Decision
	DecidedBy uuid.UUID
	Note      string
}

// Handler dipanggil di transaksi yang sama dengan keputusan level terakhir (approve) atau penolakan.
type Handler struct {
	OnApproved func(ctx context.Context, d Decided) error
	OnRejected func(ctx context.Context, d Decided) error
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	ListPolicies(ctx context.Context, subjectType string, includeEnded bool, at time.Time) ([]domain.Policy, error)
	ActivePolicies(ctx context.Context, t domain.SubjectType, at time.Time) ([]domain.Policy, error)
	GetPolicyForUpdate(ctx context.Context, id uuid.UUID) (domain.Policy, error)
	InsertPolicy(ctx context.Context, p domain.Policy, actorID uuid.UUID) (uuid.UUID, error)
	EndPolicy(ctx context.Context, p domain.Policy, version int, actorID uuid.UUID) error

	CreateRequest(ctx context.Context, r domain.Request) (uuid.UUID, error)
	GetRequest(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Request, error)
	PendingBySubjectForUpdate(ctx context.Context, t domain.SubjectType, subjectID uuid.UUID) (domain.Request, bool, error)
	ListBySubject(ctx context.Context, t domain.SubjectType, subjectID uuid.UUID) ([]domain.Request, error)
	ListRequests(ctx context.Context, f postgres.RequestFilter, page paging.Page) (paging.Result[domain.Request], error)
	PendingCandidates(ctx context.Context, userIDs []uuid.UUID, roleCodes []string) ([]domain.Request, error)
	DueForReminder(ctx context.Context, before time.Time) ([]domain.Request, error)
	SaveRequest(ctx context.Context, r domain.Request, version int, actorID uuid.UUID) error
	SaveSteps(ctx context.Context, steps []domain.Step) error

	InsertDelegation(ctx context.Context, d domain.Delegation, actorID uuid.UUID) (uuid.UUID, error)
	GetDelegationForUpdate(ctx context.Context, id uuid.UUID) (domain.Delegation, error)
	ListDelegations(ctx context.Context, userID *uuid.UUID, includePast bool, today clock.BusinessDate) ([]domain.Delegation, error)
	ActiveDelegationsTo(ctx context.Context, to uuid.UUID, today clock.BusinessDate) ([]domain.Delegation, error)
	ActiveDelegationsFrom(ctx context.Context, from []uuid.UUID, today clock.BusinessDate) ([]domain.Delegation, error)
	CancelDelegation(ctx context.Context, d domain.Delegation, version int, actorID uuid.UUID) error
}

// Deps adalah dependensi Service.
type Deps struct {
	Store     store
	Tx        txRunner
	Directory Directory
	Notifier  Notifier
	Audit     audit.Recorder
	Clock     clock.Clock
	Log       *slog.Logger
	// BaseURL adalah URL ERP untuk tautan di email.
	BaseURL string
}

// Service adalah engine approval.
type Service struct {
	Deps

	mu       sync.RWMutex
	handlers map[domain.SubjectType]Handler
	readPerm map[domain.SubjectType]string
}

// New membuat Service.
func New(d Deps) *Service {
	return &Service{Deps: d, handlers: map[domain.SubjectType]Handler{}, readPerm: map[domain.SubjectType]string{}}
}

// RegisterHandler didaftarkan modul pemilik dokumen saat wiring. readPermission memberi pemegangnya hak
// melihat riwayat persetujuan dokumen tersebut.
func (s *Service) RegisterHandler(t domain.SubjectType, readPermission string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[t] = h
	s.readPerm[t] = readPermission
}

func (s *Service) handler(t domain.SubjectType) (Handler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handlers[t]
	return h, ok
}

func (s *Service) canReadSubject(a actor.Actor, t domain.SubjectType) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.readPerm[t]
	return ok && p != "" && a.Has(p)
}

func loggedIn(ctx context.Context) (actor.Actor, error) {
	a, ok := actor.From(ctx)
	if !ok || a.UserID == uuid.Nil {
		return actor.Actor{}, apperr.New(apperr.CodeAuthSessionExpired)
	}
	return a, nil
}

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

// --- Pengajuan --------------------------------------------------------------------------------

// Request mengajukan dokumen untuk approval. Dipanggil service dokumen di dalam transaksi bisnisnya; level
// pertama langsung aktif dan approver-nya diberi notifikasi in-app + email setelah transaksi commit.
func (s *Service) Request(ctx context.Context, subj domain.Subject) (domain.Request, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	if _, ok := s.handler(subj.Type); !ok {
		return domain.Request{}, fmt.Errorf("approval handler for %q is not registered", subj.Type)
	}
	var req domain.Request
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		requester, err := s.Directory.User(ctx, a.UserID)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		policies, err := s.Store.ActivePolicies(ctx, subj.Type, now)
		if err != nil {
			return err
		}
		req, err = domain.NewRequest(subj, requester, domain.SelectPolicies(policies, subj, now), now)
		if err != nil {
			return err
		}
		// Setiap level wajib punya minimal satu approver aktif selain pembuat, agar dokumen tidak tertahan.
		for _, st := range req.Steps {
			candidates, err := s.candidates(ctx, req, st)
			if err != nil {
				return err
			}
			if len(candidates) == 0 {
				return apperr.WithMessage(apperr.CodeApprovalPolicyMissing, fmt.Sprintf(
					"Belum ada approver aktif untuk level %d (%s) selain pembuat dokumen. Atur pengganti di Pengaturan › Matriks Persetujuan.",
					st.Level, s.roleLabel(ctx, st.ApproverRole)))
			}
		}
		if req.ID, err = s.Store.CreateRequest(ctx, req); err != nil {
			return err
		}
		if req, err = s.Store.GetRequest(ctx, req.ID, false); err != nil {
			return err
		}
		return s.notifyApprovers(ctx, req, false)
	})
	return req, err
}

// Cancel membatalkan permintaan yang masih berjalan untuk dokumen (dokumen dibatalkan atau ditarik).
// Tidak melakukan apa pun jika dokumen tidak sedang menunggu persetujuan.
func (s *Service) Cancel(ctx context.Context, t domain.SubjectType, subjectID uuid.UUID) error {
	return s.Tx.WithTx(ctx, func(ctx context.Context) error {
		req, ok, err := s.Store.PendingBySubjectForUpdate(ctx, t, subjectID)
		if err != nil || !ok {
			return err
		}
		version := req.Version
		if err := req.Cancel(s.Clock.Now()); err != nil {
			return err
		}
		return s.Store.SaveRequest(ctx, req, version, actor.UserID(ctx))
	})
}

// --- Keputusan --------------------------------------------------------------------------------

// Decide menyetujui atau menolak level aktif. Saat level terakhir disetujui atau dokumen ditolak, handler
// modul pemilik dokumen dipanggil di transaksi yang sama.
func (s *Service) Decide(ctx context.Context, id uuid.UUID, version int, decision domain.Decision, note string) (domain.Request, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	var req domain.Request
	err = s.Tx.WithTx(ctx, func(ctx context.Context) error {
		if req, err = s.Store.GetRequest(ctx, id, true); err != nil {
			return err
		}
		if req.Version != version {
			return apperr.New(apperr.CodeVersionConflict)
		}
		decider, err := s.Directory.User(ctx, a.UserID)
		if err != nil {
			return err
		}
		delegators, err := s.delegatorsOf(ctx, a.UserID)
		if err != nil {
			return err
		}
		delegatedFrom, err := req.Authorize(decider, delegators)
		if err != nil {
			return err
		}
		level := req.CurrentLevel
		now := s.Clock.Now()
		final := true
		switch decision {
		case domain.DecisionApproved:
			final, err = req.Approve(decider, delegatedFrom, note, now)
		case domain.DecisionRejected:
			err = req.Reject(decider, delegatedFrom, note, now)
		default:
			err = apperr.Validation(map[string]string{"decision": "Keputusan tidak dikenal"})
		}
		if err != nil {
			return err
		}
		if err := s.Store.SaveRequest(ctx, req, version, a.UserID); err != nil {
			return err
		}
		req.Version = version + 1
		if err := s.recordDecision(ctx, req, level, decision, delegatedFrom); err != nil {
			return err
		}
		if !final {
			return s.notifyApprovers(ctx, req, false)
		}
		if err := s.runHandler(ctx, req, decision, a.UserID); err != nil {
			return err
		}
		return s.notifyRequester(ctx, req, decision)
	})
	if err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (s *Service) runHandler(ctx context.Context, req domain.Request, decision domain.Decision, by uuid.UUID) error {
	h, ok := s.handler(req.Subject.Type)
	if !ok {
		return fmt.Errorf("approval handler for %q is not registered", req.Subject.Type)
	}
	step := req.CurrentStep()
	d := Decided{RequestID: req.ID, Subject: req.Subject, Decision: decision, DecidedBy: by, Note: step.Note}
	fn := h.OnApproved
	if decision == domain.DecisionRejected {
		fn = h.OnRejected
	}
	if fn == nil {
		return nil
	}
	return fn(ctx, d)
}

// recordDecision mencatat keputusan di audit trail dokumen (tab Riwayat dokumen).
func (s *Service) recordDecision(ctx context.Context, req domain.Request, level int, decision domain.Decision, delegatedFrom *uuid.UUID) error {
	step := req.Steps[slices.IndexFunc(req.Steps, func(st domain.Step) bool { return st.Level == level })]
	action := audit.ActionApprove
	if decision == domain.DecisionRejected {
		action = audit.ActionReject
	}
	note := fmt.Sprintf("Level %d dari %d", level, req.TotalLevels())
	if delegatedFrom != nil {
		if from, err := s.Directory.User(ctx, *delegatedFrom); err == nil {
			note += " (delegasi dari " + from.Name + ")"
		}
	}
	if step.Note != "" {
		note += ": " + step.Note
	}
	return s.Audit.Record(ctx, audit.Entry{
		Action: action, EntityType: string(req.Subject.Type), EntityID: req.Subject.ID, EntityLabel: req.Subject.Label,
		Before: map[string]any{"approvalStatus": string(domain.StatusPendingApproval), "approvalLevel": level},
		After:  map[string]any{"approvalStatus": string(req.Status), "approvalLevel": req.CurrentLevel},
		Note:   note,
	})
}

// --- Siapa yang berhak --------------------------------------------------------------------------

// candidates mengembalikan user yang berhak memutuskan sebuah langkah, termasuk pengganti dan penerima delegasi.
func (s *Service) candidates(ctx context.Context, req domain.Request, st domain.Step) ([]domain.Principal, error) {
	var holders []domain.Principal
	if st.ApproverUser != nil {
		p, err := s.Directory.User(ctx, *st.ApproverUser)
		if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
			return nil, err
		}
		if err == nil {
			holders = append(holders, p)
		}
	} else {
		var err error
		if holders, err = s.Directory.UsersWithRole(ctx, st.ApproverRole, st.Division); err != nil {
			return nil, err
		}
	}
	if st.RequesterIsApprover && st.Substitute != nil {
		p, err := s.Directory.User(ctx, *st.Substitute)
		if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
			return nil, err
		}
		if err == nil {
			holders = append(holders, p)
		}
	}
	ids := make([]uuid.UUID, 0, len(holders))
	for _, h := range holders {
		ids = append(ids, h.UserID)
	}
	delegations, err := s.Store.ActiveDelegationsFrom(ctx, ids, s.today())
	if err != nil {
		return nil, err
	}
	for _, d := range delegations {
		if d.From == req.RequestedBy {
			continue
		}
		p, err := s.Directory.User(ctx, d.To)
		if err != nil && !apperr.HasCode(err, apperr.CodeNotFound) {
			return nil, err
		}
		if err == nil {
			holders = append(holders, p)
		}
	}
	out := make([]domain.Principal, 0, len(holders))
	seen := map[uuid.UUID]bool{req.RequestedBy: true}
	for _, h := range holders {
		if !seen[h.UserID] {
			seen[h.UserID] = true
			out = append(out, h)
		}
	}
	return out, nil
}

// delegatorsOf mengembalikan user yang sedang mendelegasikan approval-nya kepada userID.
func (s *Service) delegatorsOf(ctx context.Context, userID uuid.UUID) ([]domain.Principal, error) {
	delegations, err := s.Store.ActiveDelegationsTo(ctx, userID, s.today())
	if err != nil {
		return nil, err
	}
	out := make([]domain.Principal, 0, len(delegations))
	for _, d := range delegations {
		p, err := s.Directory.User(ctx, d.From)
		if apperr.HasCode(err, apperr.CodeNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) roleLabel(ctx context.Context, code string) string {
	if name, ok, err := s.Directory.RoleName(ctx, code); err == nil && ok {
		return name
	}
	return code
}

// --- Inbox ------------------------------------------------------------------------------------

// Task adalah dokumen yang menunggu keputusan user yang login.
type Task struct {
	Request domain.Request
	// DelegatedFrom terisi jika user memutuskan atas nama approver yang sedang cuti.
	DelegatedFrom *domain.Principal
}

// Inbox mengembalikan dokumen yang menunggu keputusan user (Persetujuan Saya), terlama lebih dulu.
func (s *Service) Inbox(ctx context.Context, subjectType domain.SubjectType) ([]Task, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return nil, err
	}
	me, err := s.Directory.User(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	delegators, err := s.delegatorsOf(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	var (
		ids   []uuid.UUID
		roles []string
	)
	for _, p := range append([]domain.Principal{me}, delegators...) {
		ids = append(ids, p.UserID)
		for _, g := range p.Roles {
			if !slices.Contains(roles, g.Code) {
				roles = append(roles, g.Code)
			}
		}
	}
	pending, err := s.Store.PendingCandidates(ctx, ids, roles)
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(pending))
	for _, r := range pending {
		if subjectType != "" && r.Subject.Type != subjectType {
			continue
		}
		from, err := r.Authorize(me, delegators)
		if err != nil {
			continue
		}
		t := Task{Request: r}
		if from != nil {
			i := slices.IndexFunc(delegators, func(p domain.Principal) bool { return p.UserID == *from })
			t.DelegatedFrom = &delegators[i]
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// InboxCount menghitung dokumen yang menunggu keputusan user, untuk badge menu.
func (s *Service) InboxCount(ctx context.Context) (int, error) {
	tasks, err := s.Inbox(ctx, "")
	return len(tasks), err
}

// --- Riwayat ----------------------------------------------------------------------------------

func (s *Service) canView(ctx context.Context, a actor.Actor, r domain.Request) (bool, error) {
	if r.RequestedBy == a.UserID || a.Has(PermPolicyRead) || s.canReadSubject(a, r.Subject.Type) {
		return true, nil
	}
	for _, st := range r.Steps {
		if st.DecidedBy != nil && *st.DecidedBy == a.UserID {
			return true, nil
		}
	}
	if !r.IsPending() {
		return false, nil
	}
	me, err := s.Directory.User(ctx, a.UserID)
	if err != nil {
		return false, err
	}
	delegators, err := s.delegatorsOf(ctx, a.UserID)
	if err != nil {
		return false, err
	}
	_, err = r.Authorize(me, delegators)
	return err == nil, nil
}

// Get mengembalikan satu permintaan. Hanya pembuat, approver, pemegang hak baca dokumen, dan pengelola
// kebijakan yang boleh melihat.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (domain.Request, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	r, err := s.Store.GetRequest(ctx, id, false)
	if err != nil {
		return domain.Request{}, err
	}
	ok, err := s.canView(ctx, a, r)
	if err != nil {
		return domain.Request{}, err
	}
	if !ok {
		return domain.Request{}, apperr.New(apperr.CodeForbidden)
	}
	return r, nil
}

// ListBySubject mengembalikan riwayat persetujuan satu dokumen (panel Persetujuan di detail dokumen).
func (s *Service) ListBySubject(ctx context.Context, t domain.SubjectType, subjectID uuid.UUID) ([]domain.Request, error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.Store.ListBySubject(ctx, t, subjectID)
	if err != nil {
		return nil, err
	}
	for _, r := range list {
		ok, err := s.canView(ctx, a, r)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperr.New(apperr.CodeForbidden)
		}
	}
	return list, nil
}

// ListScope menentukan daftar permintaan yang dilihat.
type ListScope string

const (
	ScopeRequested ListScope = "requested" // diajukan user
	ScopeDecided   ListScope = "decided"   // pernah diputuskan user
	ScopeAll       ListScope = "all"       // semua (pengelola kebijakan)
)

// ListRequests mengembalikan permintaan sesuai scope.
func (s *Service) ListRequests(ctx context.Context, scope ListScope, status domain.Status, t domain.SubjectType, page paging.Page) (paging.Result[domain.Request], error) {
	a, err := loggedIn(ctx)
	if err != nil {
		return paging.Result[domain.Request]{}, err
	}
	f := postgres.RequestFilter{Status: status, SubjectType: t}
	switch scope {
	case ScopeRequested:
		f.RequestedBy = &a.UserID
	case ScopeDecided:
		f.DecidedBy = &a.UserID
	case ScopeAll:
		if !a.Has(PermPolicyRead) {
			return paging.Result[domain.Request]{}, apperr.New(apperr.CodeForbidden)
		}
	default:
		return paging.Result[domain.Request]{}, apperr.Validation(map[string]string{"scope": "Scope tidak dikenal"})
	}
	return s.Store.ListRequests(ctx, f, page)
}
