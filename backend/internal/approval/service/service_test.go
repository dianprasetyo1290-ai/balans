package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/approval/domain"
	"balans/internal/approval/postgres"
	"balans/internal/approval/service"
	"balans/internal/audit"
	"balans/internal/notification"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/platform/paging"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

// directory adalah pengganti modul identity: user aktif beserta role-nya.
type directory struct {
	mu    sync.Mutex
	users map[uuid.UUID]domain.Principal
}

func (d *directory) add(name string, roles ...domain.RoleGrant) domain.Principal {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := uuid.New()
	p := domain.Principal{UserID: id, Name: name, Email: strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@balans.test", Roles: roles}
	d.users[id] = p
	return p
}

func (d *directory) User(_ context.Context, id uuid.UUID) (domain.Principal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.users[id]
	if !ok {
		return domain.Principal{}, apperr.New(apperr.CodeNotFound)
	}
	return p, nil
}

func (d *directory) UsersWithRole(_ context.Context, role string, division *uuid.UUID) ([]domain.Principal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []domain.Principal
	for _, p := range d.users {
		for _, g := range p.Roles {
			if g.Code == role && (division == nil || g.Division == nil || *g.Division == *division) {
				out = append(out, p)
				break
			}
		}
	}
	return out, nil
}

func (d *directory) RoleName(_ context.Context, code string) (string, bool, error) {
	return "Role " + code, code != "unknown", nil
}

func (d *directory) SearchUsers(_ context.Context, q string, _ int) ([]domain.Principal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []domain.Principal
	for _, p := range d.users {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (d *directory) deactivate(id uuid.UUID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.users, id)
}

type nopSender struct{}

func (nopSender) Send(context.Context, notification.Email) error { return nil }

type calls struct {
	mu       sync.Mutex
	approved []service.Decided
	rejected []service.Decided
	fail     error
}

func (c *calls) handler() service.Handler {
	return service.Handler{
		OnApproved: func(_ context.Context, d service.Decided) error {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.fail != nil {
				return c.fail
			}
			c.approved = append(c.approved, d)
			return nil
		},
		OnRejected: func(_ context.Context, d service.Decided) error {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.rejected = append(c.rejected, d)
			return nil
		},
	}
}

type harness struct {
	t     *testing.T
	pool  *pgxpool.Pool
	svc   *service.Service
	dir   *directory
	clock *clock.Fixed
	calls *calls

	purchasing, hfa, director, presDir, finance, admin domain.Principal
}

// monday09 adalah Senin 19 Okt 2026 09.00 WIB (jam kerja).
var monday09 = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	// Setiap test mulai dari matriks hasil seed migrasi tanpa permintaan sisa test lain.
	for _, q := range []string{
		`DELETE FROM approval_steps`, `DELETE FROM approval_requests`, `DELETE FROM approval_delegations`,
		`DELETE FROM notifications`, `DELETE FROM notification_emails`, `DELETE FROM approval_policies WHERE created_by <> '00000000-0000-0000-0000-000000000000'`,
		`UPDATE approval_policies SET valid_to = NULL, version = 1, substitute_user_id = NULL, approver_user_id = NULL`,
	} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err)
	}
	tx := db.NewTxManager(pool)
	jobs, err := jobqueue.NewInserter(pool)
	require.NoError(t, err)
	fixed := clock.NewFixed(monday09)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifier := notification.New(notification.Deps{Pool: pool, Tx: tx, Jobs: jobs, Clock: fixed, Log: log, Sender: nopSender{}})
	dir := &directory{users: map[uuid.UUID]domain.Principal{}}
	svc := service.New(service.Deps{
		Store: postgres.NewStore(pool), Tx: tx, Directory: dir, Notifier: notifier,
		Audit: audit.New(audit.Deps{Pool: pool}), Clock: fixed, Log: log, BaseURL: "https://erp.balans.test",
	})
	c := &calls{}
	for _, info := range domain.SubjectTypes() {
		svc.RegisterHandler(info.Type, "test."+string(info.Type)+".read", c.handler())
	}
	h := &harness{t: t, pool: pool, svc: svc, dir: dir, clock: fixed, calls: c}
	h.purchasing = dir.add("Rina Purchasing", domain.RoleGrant{Code: "purchasing"})
	h.hfa = dir.add("Budi Head FA", domain.RoleGrant{Code: "head_finance"})
	h.director = dir.add("Dewi Direksi", domain.RoleGrant{Code: "director"})
	h.presDir = dir.add("Eko Dirut", domain.RoleGrant{Code: "president_director"})
	h.finance = dir.add("Fajar Finance", domain.RoleGrant{Code: "finance"})
	h.admin = dir.add("Ani Admin", domain.RoleGrant{Code: "system_admin"})
	return h
}

func (h *harness) as(p domain.Principal, perms ...string) context.Context {
	set := map[string]struct{}{}
	for _, perm := range perms {
		set[perm] = struct{}{}
	}
	roles := make([]string, len(p.Roles))
	for i, g := range p.Roles {
		roles[i] = g.Code
	}
	return actor.With(context.Background(), actor.Actor{UserID: p.UserID, FullName: p.Name, Roles: roles, Permissions: set})
}

func (h *harness) submit(p domain.Principal, t domain.SubjectType, amount string, mutate ...func(*domain.Subject)) domain.Request {
	h.t.Helper()
	s := domain.Subject{Type: t, ID: uuid.New(), Label: "DOC/2026/10/" + uuid.NewString()[:5], Amount: money.MustParse(amount)}
	for _, m := range mutate {
		m(&s)
	}
	r, err := h.svc.Request(h.as(p), s)
	require.NoError(h.t, err)
	return r
}

func (h *harness) notifications(p domain.Principal) []string {
	h.t.Helper()
	rows, err := h.pool.Query(context.Background(), `SELECT kind || '|' || title FROM notifications WHERE user_id = $1 ORDER BY created_at, id`, p.UserID)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(h.t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(h.t, rows.Err())
	return out
}

func (h *harness) emailsTo(p domain.Principal) int {
	h.t.Helper()
	var n int
	require.NoError(h.t, h.pool.QueryRow(context.Background(), `SELECT count(*) FROM notification_emails WHERE recipient = $1`, p.Email).Scan(&n))
	return n
}

func (h *harness) inbox(p domain.Principal) []service.Task {
	h.t.Helper()
	tasks, err := h.svc.Inbox(h.as(p), "")
	require.NoError(h.t, err)
	return tasks
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func TestPurchaseOrderApprovedLevelByLevel(t *testing.T) {
	h := setup(t)
	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "60000000", func(s *domain.Subject) { s.Summary = "Tube 30 ml" })
	require.Equal(t, 3, req.TotalLevels())
	assert.Equal(t, "Rina Purchasing", req.RequestedByName)

	// Level 1 aktif: hanya Head F&A yang diberi tahu dan melihatnya di inbox.
	assert.Equal(t, []string{"approval.pending|Menunggu persetujuan Anda: " + req.Subject.Label}, h.notifications(h.hfa))
	assert.Equal(t, 1, h.emailsTo(h.hfa))
	assert.Empty(t, h.notifications(h.director))
	assert.Len(t, h.inbox(h.hfa), 1)
	assert.Empty(t, h.inbox(h.director))
	assert.Empty(t, h.inbox(h.purchasing), "pembuat tidak melihat dokumennya sendiri di inbox")

	_, err := h.svc.Decide(h.as(h.director), req.ID, req.Version, domain.DecisionApproved, "")
	assert.Equal(t, apperr.CodeForbidden, code(err), "Direksi belum boleh memutuskan level 1")
	_, err = h.svc.Decide(h.as(h.purchasing), req.ID, req.Version, domain.DecisionApproved, "")
	assert.Equal(t, apperr.CodeForbidden, code(err), "pembuat tidak boleh menyetujui dokumennya sendiri")

	req, err = h.svc.Decide(h.as(h.hfa), req.ID, req.Version, domain.DecisionApproved, "Harga wajar")
	require.NoError(t, err)
	assert.Equal(t, 2, req.CurrentLevel)
	assert.Len(t, h.notifications(h.director), 1)
	assert.Empty(t, h.calls.approved, "handler baru dipanggil setelah level terakhir")

	_, err = h.svc.Decide(h.as(h.director), req.ID, req.Version-1, domain.DecisionApproved, "")
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	req, err = h.svc.Decide(h.as(h.director), req.ID, req.Version, domain.DecisionApproved, "")
	require.NoError(t, err)
	req, err = h.svc.Decide(h.as(h.presDir), req.ID, req.Version, domain.DecisionApproved, "")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusApproved, req.Status)

	require.Len(t, h.calls.approved, 1)
	assert.Equal(t, req.Subject.ID, h.calls.approved[0].Subject.ID)
	assert.Equal(t, h.presDir.UserID, h.calls.approved[0].DecidedBy)
	assert.Contains(t, h.notifications(h.purchasing), "approval.decided|"+req.Subject.Label+" disetujui")

	// Keputusan tercatat di tab Riwayat dokumen.
	var approvals int
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE entity_type = 'purchase_order' AND entity_id = $1 AND action = 'approve'`,
		req.Subject.ID).Scan(&approvals))
	assert.Equal(t, 3, approvals)

	stored, err := h.svc.Get(h.as(h.purchasing), req.ID)
	require.NoError(t, err)
	assert.Equal(t, "Harga wajar", stored.Steps[0].Note)
	assert.Equal(t, h.hfa.UserID, *stored.Steps[0].DecidedBy)
}

func TestRejectReturnsToRequesterAndAllowsResubmit(t *testing.T) {
	h := setup(t)
	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "5000000")

	_, err := h.svc.Request(h.as(h.purchasing), req.Subject)
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err), "dokumen yang masih menunggu tidak bisa diajukan lagi")

	_, err = h.svc.Decide(h.as(h.hfa), req.ID, req.Version, domain.DecisionRejected, " ")
	assert.Equal(t, apperr.CodeValidationFailed, code(err))

	req, err = h.svc.Decide(h.as(h.hfa), req.ID, req.Version, domain.DecisionRejected, "Pemasok belum terdaftar")
	require.NoError(t, err)
	assert.Equal(t, domain.StatusRejected, req.Status)
	require.Len(t, h.calls.rejected, 1)
	assert.Equal(t, "Pemasok belum terdaftar", h.calls.rejected[0].Note)
	assert.Contains(t, h.notifications(h.purchasing), "approval.decided|"+req.Subject.Label+" ditolak")

	again, err := h.svc.Request(h.as(h.purchasing), req.Subject)
	require.NoError(t, err, "setelah direvisi dokumen bisa diajukan lagi")
	list, err := h.svc.ListBySubject(h.as(h.purchasing), req.Subject.Type, req.Subject.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, again.ID, list[0].ID, "terbaru lebih dulu")
}

func TestHandlerErrorRollsBackDecision(t *testing.T) {
	h := setup(t)
	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "100")
	h.calls.fail = errors.New("PO sudah dibatalkan")

	_, err := h.svc.Decide(h.as(h.hfa), req.ID, req.Version, domain.DecisionApproved, "")
	require.Error(t, err)

	stored, err := h.svc.Get(h.as(h.purchasing), req.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusPendingApproval, stored.Status, "keputusan dan handler satu transaksi")
	assert.Nil(t, stored.Steps[0].Decision)
	assert.NotContains(t, h.notifications(h.purchasing), "approval.decided|"+req.Subject.Label+" disetujui")
}

func TestCancelPendingRequest(t *testing.T) {
	h := setup(t)
	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "100")
	require.NoError(t, h.svc.Cancel(h.as(h.purchasing), req.Subject.Type, req.Subject.ID))
	require.NoError(t, h.svc.Cancel(h.as(h.purchasing), req.Subject.Type, req.Subject.ID), "tidak ada yang berjalan = no-op")

	stored, err := h.svc.Get(h.as(h.purchasing), req.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusCancelled, stored.Status)
	assert.Empty(t, h.inbox(h.hfa))
	_, err = h.svc.Decide(h.as(h.hfa), req.ID, stored.Version, domain.DecisionApproved, "")
	assert.Equal(t, apperr.CodeInvalidStateTransition, code(err))
}

func TestDelegationWhileOnLeave(t *testing.T) {
	h := setup(t)
	today := clock.Today(h.clock)
	_, err := h.svc.CreateDelegation(h.as(h.purchasing), domain.DelegationInput{From: h.hfa.UserID, To: h.finance.UserID, ValidFrom: today, ValidTo: today})
	assert.Equal(t, apperr.CodeForbidden, code(err), "hanya pemilik atau pengelola kebijakan")

	d, err := h.svc.CreateDelegation(h.as(h.hfa), domain.DelegationInput{To: h.finance.UserID, ValidFrom: today, ValidTo: today.AddDays(3), Reason: "Cuti"})
	require.NoError(t, err)
	_, err = h.svc.CreateDelegation(h.as(h.hfa), domain.DelegationInput{To: h.director.UserID, ValidFrom: today.AddDays(2), ValidTo: today.AddDays(5)})
	assert.Equal(t, apperr.CodeValidationFailed, code(err), "delegasi tumpang tindih ditolak")

	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "100")
	assert.Len(t, h.notifications(h.finance), 1, "penerima delegasi ikut diberi tahu")

	tasks := h.inbox(h.finance)
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].DelegatedFrom)
	assert.Equal(t, h.hfa.UserID, tasks[0].DelegatedFrom.UserID)

	req, err = h.svc.Decide(h.as(h.finance), req.ID, req.Version, domain.DecisionApproved, "")
	require.NoError(t, err)
	assert.Equal(t, h.hfa.UserID, *req.Steps[0].DelegatedFrom)

	mine, err := h.svc.ListDelegations(h.as(h.finance), false, false)
	require.NoError(t, err)
	assert.Len(t, mine, 1)
	_, err = h.svc.ListDelegations(h.as(h.finance), true, false)
	assert.Equal(t, apperr.CodeForbidden, code(err))

	_, err = h.svc.CancelDelegation(h.as(h.finance), d.ID, d.Version)
	assert.Equal(t, apperr.CodeForbidden, code(err), "penerima tidak bisa menghentikan delegasi")
	d, err = h.svc.CancelDelegation(h.as(h.hfa), d.ID, d.Version)
	require.NoError(t, err)
	assert.NotNil(t, d.CancelledAt)

	h.submit(h.purchasing, domain.SubjectPurchaseOrder, "200")
	assert.Empty(t, h.inbox(h.finance), "delegasi yang dihentikan tidak memberi hak lagi")
}

func TestActingApproverGoesToSubstitute(t *testing.T) {
	h := setup(t)
	acting := h.dir.add("Gita Acting", domain.RoleGrant{Code: "head_finance"}, domain.RoleGrant{Code: "purchasing"})
	h.dir.deactivate(h.hfa.UserID)

	_, err := h.svc.Request(h.as(acting), domain.Subject{Type: domain.SubjectPurchaseOrder, ID: uuid.New(), Label: "PO/1", Amount: money.MustParse("100")})
	assert.Equal(t, apperr.CodeApprovalPolicyMissing, code(err), "tanpa approver lain dokumen tidak bisa diajukan")

	policies, err := h.svc.ListPolicies(h.as(h.admin, service.PermPolicyRead), domain.SubjectPurchaseOrder, false)
	require.NoError(t, err)
	level1 := policies[0]
	require.Equal(t, 1, level1.Level)
	_, err = h.svc.UpdatePolicy(h.as(h.admin, service.PermPolicyWrite), level1.ID, level1.Version, domain.PolicyInput{
		ApproverRole: "head_finance", Substitute: &h.finance.UserID,
	})
	require.NoError(t, err)

	req := h.submit(acting, domain.SubjectPurchaseOrder, "100")
	assert.True(t, req.Steps[0].RequesterIsApprover)
	assert.Len(t, h.inbox(h.finance), 1, "level naik ke pengganti")
	_, err = h.svc.Decide(h.as(acting), req.ID, req.Version, domain.DecisionApproved, "")
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.Decide(h.as(h.finance), req.ID, req.Version, domain.DecisionApproved, "")
	require.NoError(t, err)
}

func TestPolicyChangeDoesNotAffectRunningRequests(t *testing.T) {
	h := setup(t)
	running := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "20000000")
	require.Equal(t, "director", running.Steps[1].ApproverRole)

	read := h.as(h.admin, service.PermPolicyRead)
	write := h.as(h.admin, service.PermPolicyWrite)
	policies, err := h.svc.ListPolicies(read, domain.SubjectPurchaseOrder, false)
	require.NoError(t, err)
	level2 := policies[1]
	require.Equal(t, 2, level2.Level)

	_, err = h.svc.ListPolicies(h.as(h.purchasing), "", false)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.UpdatePolicy(read, level2.ID, level2.Version, domain.PolicyInput{ApproverRole: "president_director"})
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.UpdatePolicy(write, level2.ID, level2.Version, domain.PolicyInput{ApproverRole: "unknown"})
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
	_, err = h.svc.UpdatePolicy(write, level2.ID, level2.Version, domain.PolicyInput{ApproverRole: "head_finance", ApproverUser: &h.director.UserID})
	assert.Contains(t, apperr.As(err).Fields, "approverUserId", "user harus memegang jabatan")

	h.clock.Advance(time.Minute)
	updated, err := h.svc.UpdatePolicy(write, level2.ID, level2.Version, domain.PolicyInput{
		ApproverRole: "president_director", MinAmount: new(money.MustParse("15000000")),
	})
	require.NoError(t, err)
	assert.NotEqual(t, level2.ID, updated.ID, "versi baru")
	_, err = h.svc.UpdatePolicy(write, level2.ID, level2.Version, domain.PolicyInput{ApproverRole: "director"})
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	all, err := h.svc.ListPolicies(read, domain.SubjectPurchaseOrder, true)
	require.NoError(t, err)
	assert.Len(t, all, 4, "versi lama tetap tersimpan")

	stored, err := h.svc.Get(h.as(h.purchasing), running.ID)
	require.NoError(t, err)
	assert.Equal(t, "director", stored.Steps[1].ApproverRole, "dokumen berjalan memakai snapshot")

	fresh := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "20000000")
	assert.Equal(t, "president_director", fresh.Steps[1].ApproverRole)
	small := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "12000000")
	assert.Equal(t, 1, small.TotalLevels(), "batas baru Rp15 jt")

	h.clock.Advance(time.Minute)
	ended, err := h.svc.EndPolicy(write, updated.ID, updated.Version)
	require.NoError(t, err)
	assert.NotNil(t, ended.ValidTo)
	_, err = h.svc.EndPolicy(write, updated.ID, ended.Version)
	assert.Error(t, err)

	created, err := h.svc.CreatePolicy(write, domain.PolicyInput{SubjectType: domain.SubjectPurchaseOrder, Level: 4, ApproverRole: "director", MinAmount: new(money.MustParse("100000000"))})
	require.NoError(t, err)
	assert.Equal(t, 4, created.Level)
	_, err = h.svc.CreatePolicy(write, domain.PolicyInput{SubjectType: domain.SubjectPurchaseOrder, Level: 4, ApproverRole: "director"})
	assert.Contains(t, apperr.As(err).Fields, "level", "level yang sama di periode yang sama ditolak")
}

func TestDivisionHeadOnlyForOwnDivision(t *testing.T) {
	h := setup(t)
	logistics, sales := uuid.New(), uuid.New()
	headLog := h.dir.add("Hadi Head Logistik", domain.RoleGrant{Code: "division_head", Division: &logistics})
	headSales := h.dir.add("Indah Head Sales", domain.RoleGrant{Code: "division_head", Division: &sales})

	req := h.submit(h.purchasing, domain.SubjectPurchaseRequest, "1000000", func(s *domain.Subject) { s.Division = &logistics })
	assert.Len(t, h.inbox(headLog), 1)
	assert.Empty(t, h.inbox(headSales))
	assert.Empty(t, h.notifications(headSales))
	_, err := h.svc.Decide(h.as(headSales), req.ID, req.Version, domain.DecisionApproved, "")
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.Decide(h.as(headLog), req.ID, req.Version, domain.DecisionApproved, "")
	require.NoError(t, err)
}

func TestVisibilityAndLists(t *testing.T) {
	h := setup(t)
	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "100")

	_, err := h.svc.Get(h.as(h.finance), req.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.Get(h.as(h.finance, "test.purchase_order.read"), req.ID)
	assert.NoError(t, err, "pemegang hak baca dokumen boleh melihat")
	_, err = h.svc.Get(h.as(h.hfa), req.ID)
	assert.NoError(t, err, "approver yang berhak boleh melihat")
	_, err = h.svc.ListBySubject(h.as(h.finance), req.Subject.Type, req.Subject.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))

	_, err = h.svc.Decide(h.as(h.hfa), req.ID, req.Version, domain.DecisionApproved, "")
	require.NoError(t, err)

	page := paging.New(nil, nil)
	mine, err := h.svc.ListRequests(h.as(h.purchasing), service.ScopeRequested, "", "", page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, mine.Total)
	decided, err := h.svc.ListRequests(h.as(h.hfa), service.ScopeDecided, domain.StatusApproved, domain.SubjectPurchaseOrder, page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, decided.Total)
	_, err = h.svc.ListRequests(h.as(h.hfa), service.ScopeAll, "", "", page)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	all, err := h.svc.ListRequests(h.as(h.admin, service.PermPolicyRead), service.ScopeAll, "", "", page)
	require.NoError(t, err)
	assert.EqualValues(t, 1, all.Total)

	users, err := h.svc.SearchUsers(h.as(h.purchasing), "dewi")
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, h.director.UserID, users[0].UserID)
}

func TestRemindersDuringWorkingHoursOnly(t *testing.T) {
	h := setup(t)
	req := h.submit(h.purchasing, domain.SubjectPurchaseOrder, "100")
	ctx := context.Background()

	n, err := h.svc.Remind(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "belum 1 jam")

	h.clock.Advance(time.Hour)
	n, err = h.svc.Remind(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Contains(t, h.notifications(h.hfa), "approval.reminder|Pengingat persetujuan: "+req.Subject.Label)
	assert.Equal(t, 2, h.emailsTo(h.hfa))

	h.clock.Advance(30 * time.Minute)
	n, err = h.svc.Remind(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "pengingat berikutnya 1 jam setelah pengingat terakhir")

	h.clock.Advance(8 * time.Hour) // 18.30 WIB
	n, err = h.svc.Remind(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "di luar jam kerja tidak ada pengingat")
	assert.Len(t, slices.DeleteFunc(h.notifications(h.hfa), func(s string) bool { return !strings.HasPrefix(s, "approval.reminder") }), 1)
}

func TestRequestValidation(t *testing.T) {
	h := setup(t)
	_, err := h.svc.Request(context.Background(), domain.Subject{Type: domain.SubjectPurchaseOrder})
	assert.Equal(t, apperr.CodeAuthSessionExpired, code(err))
	_, err = h.svc.Request(h.as(h.purchasing), domain.Subject{Type: "tanpa_handler", ID: uuid.New(), Label: "X"})
	assert.Error(t, err)
	_, err = h.svc.Request(h.as(h.purchasing), domain.Subject{Type: domain.SubjectPurchaseOrder, ID: uuid.New()})
	assert.Equal(t, apperr.CodeValidationFailed, code(err))
}
