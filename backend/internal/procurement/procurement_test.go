package procurement_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/document"
	"balans/internal/notification"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/jobqueue"
	"balans/internal/platform/money"
	"balans/internal/procurement"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

// Senin 19 Okt 2026 09.00 WIB.
var monday09 = time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC)

// --- pengganti modul identity, catalog, party, warehouse ---------------------------------------

type directory struct {
	mu    sync.Mutex
	users map[uuid.UUID]approval.Principal
}

func (d *directory) add(name string, roles ...approval.RoleGrant) approval.Principal {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := approval.Principal{UserID: uuid.New(), Name: name, Email: strings.ToLower(strings.ReplaceAll(name, " ", ".")) + "@balans.test", Roles: roles}
	d.users[p.UserID] = p
	return p
}

func (d *directory) User(_ context.Context, id uuid.UUID) (approval.Principal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.users[id]
	if !ok {
		return approval.Principal{}, apperr.New(apperr.CodeNotFound)
	}
	return p, nil
}

func (d *directory) UsersWithRole(_ context.Context, role string, division *uuid.UUID) ([]approval.Principal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []approval.Principal
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
	return "Role " + code, true, nil
}

func (d *directory) SearchUsers(context.Context, string, int) ([]approval.Principal, error) {
	return nil, nil
}

// Users untuk procurement.
func (d *directory) UsersWithRoleRef(ctx context.Context, role string) ([]procurement.UserRef, error) {
	list, err := d.UsersWithRole(ctx, role, nil)
	out := make([]procurement.UserRef, len(list))
	for i, p := range list {
		out[i] = procurement.UserRef{ID: p.UserID, Name: p.Name, Email: p.Email}
	}
	return out, err
}

type users struct{ d *directory }

func (u users) UsersWithRole(ctx context.Context, role string) ([]procurement.UserRef, error) {
	return u.d.UsersWithRoleRef(ctx, role)
}

func (u users) UserNames(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	u.d.mu.Lock()
	defer u.d.mu.Unlock()
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		if p, ok := u.d.users[id]; ok {
			out[id] = p.Name
		}
	}
	return out, nil
}

type masters struct {
	products  map[uuid.UUID]procurement.ProductRef
	suppliers map[uuid.UUID]procurement.SupplierRef
	locations map[uuid.UUID]procurement.LocationRef
	toll      map[uuid.UUID]uuid.UUID
}

func (m *masters) Products(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]procurement.ProductRef, error) {
	out := map[uuid.UUID]procurement.ProductRef{}
	for _, id := range ids {
		if p, ok := m.products[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (m *masters) Supplier(_ context.Context, id uuid.UUID) (procurement.SupplierRef, bool, error) {
	s, ok := m.suppliers[id]
	return s, ok, nil
}

func (m *masters) TollLocation(_ context.Context, supplierID uuid.UUID) (*procurement.LocationRef, error) {
	id, ok := m.toll[supplierID]
	if !ok {
		return nil, nil
	}
	l := m.locations[id]
	return &l, nil
}

func (m *masters) Locations(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]procurement.LocationRef, error) {
	out := map[uuid.UUID]procurement.LocationRef{}
	for _, id := range ids {
		if l, ok := m.locations[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

func (m *masters) product(sku, productType string) uuid.UUID {
	id := uuid.New()
	m.products[id] = procurement.ProductRef{ID: id, SKU: sku, Name: "Produk " + sku, UOM: "pcs", Type: productType, IsActive: true}
	return id
}

func (m *masters) supplier(code string, toll bool) uuid.UUID {
	id := uuid.New()
	m.suppliers[id] = procurement.SupplierRef{ID: id, Code: code, Name: "PT " + code, IsTollManufacturer: toll, PaymentTermDays: 30}
	return id
}

func (m *masters) location(code, locationType string) uuid.UUID {
	id := uuid.New()
	m.locations[id] = procurement.LocationRef{ID: id, Code: code, Name: "Lokasi " + code, Type: locationType, IsActive: true}
	return id
}

// ledger dan numberer menyesuaikan modul accounting/document seperti adapter di internal/app.
type ledger struct{ acc *accounting.Module }

func (l ledger) DivisionOptions(ctx context.Context) ([]procurement.Option, error) {
	list, err := l.acc.DivisionOptions(ctx)
	out := make([]procurement.Option, len(list))
	for i, d := range list {
		out[i] = procurement.Option{ID: d.ID, Code: d.Code, Name: d.Name}
	}
	return out, err
}

func (l ledger) DivisionNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	return l.acc.DivisionNames(ctx, ids)
}

func (l ledger) CheckDivision(ctx context.Context, id uuid.UUID, at clock.BusinessDate, field string) error {
	return l.acc.CheckDivision(ctx, id, at, field)
}

func (l ledger) CashBankAccounts(ctx context.Context) ([]procurement.Option, error) {
	list, err := l.acc.PostableAccounts(ctx, "11")
	out := make([]procurement.Option, len(list))
	for i, a := range list {
		out[i] = procurement.Option{ID: a.ID, Code: a.Code, Name: a.Name}
	}
	return out, err
}

func (l ledger) Post(ctx context.Context, req accounting.PostingRequest) (accounting.JournalEntry, error) {
	return l.acc.Post(ctx, req)
}

func (l ledger) Reverse(ctx context.Context, req accounting.ReversalRequest) (accounting.JournalEntry, error) {
	return l.acc.Reverse(ctx, req)
}

type numberer struct{ docs *document.Module }

func (n numberer) Next(ctx context.Context, t procurement.DocumentType, d clock.BusinessDate) (string, error) {
	types := map[procurement.DocumentType]document.Type{
		procurement.DocPurchaseRequest: document.TypePurchaseRequest, procurement.DocPurchaseOrder: document.TypePurchaseOrder,
		procurement.DocPayment: document.TypePayment,
	}
	return n.docs.Next(ctx, types[t], d)
}

type journalNumberer struct{ docs *document.Module }

func (n journalNumberer) NextJournalNumber(ctx context.Context, d clock.BusinessDate) (string, error) {
	return n.docs.Next(ctx, document.TypeJournal, d)
}

type nopSender struct{}

func (nopSender) Send(context.Context, notification.Email) error { return nil }

// --- harness ---------------------------------------------------------------------------------

type harness struct {
	t        *testing.T
	pool     *pgxpool.Pool
	clock    *clock.Fixed
	dir      *directory
	masters  *masters
	approval *approval.Module
	accounts *accounting.Module
	srv      *httptest.Server
	current  actor.Actor
	mu       sync.Mutex
	division uuid.UUID

	purchasing, divHead, hfa, director, presDir, finance, leader approval.Principal
}

var allPerms = []string{
	"procurement.purchase_request.read", "procurement.purchase_request.write", "procurement.purchase_request.submit",
	"procurement.purchase_order.read", "procurement.purchase_order.write", "procurement.purchase_order.submit",
	"procurement.purchase_order.cancel", "procurement.purchase_advance.read", "procurement.purchase_advance.write",
	"procurement.purchase_advance.post",
}

func setup(t *testing.T) (*harness, *procurement.Module) {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	for _, q := range []string{
		`DELETE FROM approval_steps`, `DELETE FROM approval_requests`, `DELETE FROM notifications`,
		`DELETE FROM notification_emails`,
	} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err)
	}
	tx := db.NewTxManager(pool)
	jobs, err := jobqueue.NewInserter(pool)
	require.NoError(t, err)
	fixed := clock.NewFixed(monday09)
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	rs := httpx.NewResponder(log, nil)
	auditMod := audit.New(audit.Deps{Pool: pool})
	notifier := notification.New(notification.Deps{Pool: pool, Tx: tx, Jobs: jobs, Clock: fixed, Log: log, Sender: nopSender{}})
	docs := document.New(document.Deps{Pool: pool, Clock: fixed, Responder: rs})
	acc := accounting.New(accounting.Deps{
		Pool: pool, Tx: tx, Clock: fixed, Log: log, Responder: rs, Audit: auditMod, Numberer: journalNumberer{docs},
	})
	dir := &directory{users: map[uuid.UUID]approval.Principal{}}
	apr := approval.New(approval.Deps{
		Pool: pool, Tx: tx, Clock: fixed, Log: log, Responder: rs, Audit: auditMod, Directory: dir, Notifier: notifier,
		BaseURL: "https://erp.balans.test",
	})
	ms := &masters{
		products: map[uuid.UUID]procurement.ProductRef{}, suppliers: map[uuid.UUID]procurement.SupplierRef{},
		locations: map[uuid.UUID]procurement.LocationRef{}, toll: map[uuid.UUID]uuid.UUID{},
	}
	m := procurement.New(procurement.Deps{
		Pool: pool, Tx: tx, Clock: fixed, Log: log, Responder: rs, Audit: auditMod, Catalog: ms, Suppliers: ms,
		Locations: ms, Ledger: ledger{acc}, Approvals: apr, Numberer: numberer{docs}, Users: users{dir}, Notifier: notifier,
		BaseURL: "https://erp.balans.test",
	})
	m.RegisterApprovalHandlers(apr)

	h := &harness{t: t, pool: pool, clock: fixed, dir: dir, masters: ms, approval: apr, accounts: acc}
	divisions, err := acc.DivisionOptions(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, divisions)
	h.division = divisions[0].ID
	h.purchasing = dir.add("Rina Purchasing", approval.RoleGrant{Code: "purchasing"})
	h.divHead = dir.add("Hadi Head Divisi", approval.RoleGrant{Code: "division_head", Division: &h.division})
	h.hfa = dir.add("Budi Head FA", approval.RoleGrant{Code: "head_finance"})
	h.director = dir.add("Dewi Direksi", approval.RoleGrant{Code: "director"})
	h.presDir = dir.add("Eko Dirut", approval.RoleGrant{Code: "president_director"})
	h.finance = dir.add("Fajar Finance", approval.RoleGrant{Code: "finance"})
	h.leader = dir.add("Lina Logistik Leader", approval.RoleGrant{Code: "logistics_leader"})

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			h.mu.Lock()
			a := h.current
			h.mu.Unlock()
			next.ServeHTTP(w, req.WithContext(actor.With(req.Context(), a)))
		})
	})
	apr.RegisterRoutes(r)
	m.RegisterRoutes(r)
	h.srv = httptest.NewServer(r)
	t.Cleanup(h.srv.Close)
	return h, m
}

func (h *harness) actorOf(p approval.Principal, perms ...string) actor.Actor {
	set := map[string]struct{}{}
	for _, perm := range perms {
		set[perm] = struct{}{}
	}
	roles := make([]string, len(p.Roles))
	for i, g := range p.Roles {
		roles[i] = g.Code
	}
	return actor.Actor{UserID: p.UserID, FullName: p.Name, Roles: roles, Permissions: set}
}

func (h *harness) as(p approval.Principal, perms ...string) context.Context {
	return actor.With(context.Background(), h.actorOf(p, perms...))
}

// call menjalankan request HTTP sebagai user dan mengurai respons JSON.
func (h *harness) call(p approval.Principal, method, path string, body any, out any) int {
	h.t.Helper()
	h.mu.Lock()
	h.current = h.actorOf(p, allPerms...)
	h.mu.Unlock()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(h.t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, h.srv.URL+path, rd)
	require.NoError(h.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	res, err := h.srv.Client().Do(req)
	require.NoError(h.t, err)
	defer res.Body.Close() //nolint:errcheck // body respons test
	raw, err := io.ReadAll(res.Body)
	require.NoError(h.t, err)
	if out != nil && res.StatusCode < 300 {
		require.NoError(h.t, json.Unmarshal(raw, out), string(raw))
	}
	if res.StatusCode >= 300 {
		h.t.Logf("%s %s → %d %s", method, path, res.StatusCode, raw)
	}
	return res.StatusCode
}

type approvalRequest struct {
	ID           uuid.UUID `json:"id"`
	Version      int       `json:"version"`
	Status       string    `json:"status"`
	CurrentLevel int       `json:"currentLevel"`
	TotalLevels  int       `json:"totalLevels"`
	SubjectLink  string    `json:"subjectLink"`
}

// approve menyetujui level aktif dokumen lewat API approval.
func (h *harness) approve(p approval.Principal, subjectType string, subjectID uuid.UUID) approvalRequest {
	h.t.Helper()
	var list struct {
		Items []approvalRequest `json:"items"`
	}
	require.Equal(h.t, http.StatusOK, h.call(p, http.MethodGet,
		fmt.Sprintf("/api/v1/approval-requests?subjectType=%s&subjectId=%s", subjectType, subjectID), nil, &list))
	require.NotEmpty(h.t, list.Items)
	req := list.Items[0]
	var out approvalRequest
	require.Equal(h.t, http.StatusOK, h.call(p, http.MethodPost, "/api/v1/approval-requests/"+req.ID.String()+"/approve",
		map[string]any{"version": req.Version}, &out))
	return out
}

func (h *harness) notificationCount(p approval.Principal, kind string) int {
	h.t.Helper()
	var n int
	require.NoError(h.t, h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = $2`, p.UserID, kind).Scan(&n))
	return n
}

type purchaseRequest struct {
	ID             uuid.UUID `json:"id"`
	Number         string    `json:"number"`
	Status         string    `json:"status"`
	Version        int       `json:"version"`
	EstimatedTotal string    `json:"estimatedTotal"`
	Lines          []struct {
		ID           uuid.UUID `json:"id"`
		Qty          string    `json:"qty"`
		FulfilledQty string    `json:"fulfilledQty"`
		RemainingQty string    `json:"remainingQty"`
	} `json:"lines"`
	AllowedActions []string `json:"allowedActions"`
}

type purchaseOrder struct {
	ID             uuid.UUID `json:"id"`
	Number         string    `json:"number"`
	Status         string    `json:"status"`
	Version        int       `json:"version"`
	TotalAmount    string    `json:"totalAmount"`
	AllowedActions []string  `json:"allowedActions"`
	Lines          []struct {
		ID                    uuid.UUID `json:"id"`
		ReceivedQty           string    `json:"receivedQty"`
		PurchaseRequestNumber string    `json:"purchaseRequestNumber"`
	} `json:"lines"`
	Materials []struct {
		PlannedQty string `json:"plannedQty"`
	} `json:"materials"`
	TollLocation *struct {
		Code string `json:"code"`
	} `json:"tollLocation"`
	AdvancePaidAmount        string `json:"advancePaidAmount"`
	AdvanceOutstandingAmount string `json:"advanceOutstandingAmount"`
}

type purchaseAdvance struct {
	ID                uuid.UUID  `json:"id"`
	Number            string     `json:"number"`
	Status            string     `json:"status"`
	Version           int        `json:"version"`
	JournalEntryID    *uuid.UUID `json:"journalEntryId"`
	OutstandingAmount string     `json:"outstandingAmount"`
}

func (h *harness) journalLines(id uuid.UUID) map[string][2]string {
	h.t.Helper()
	rows, err := h.pool.Query(context.Background(), `
		SELECT a.code, l.debit_amount::text, l.credit_amount::text, l.division_id, l.party_type
		FROM journal_lines l JOIN accounts a ON a.id = l.account_id WHERE l.journal_entry_id = $1`, id)
	require.NoError(h.t, err)
	defer rows.Close()
	out := map[string][2]string{}
	for rows.Next() {
		var code, debit, credit string
		var division *uuid.UUID
		var partyType *string
		require.NoError(h.t, rows.Scan(&code, &debit, &credit, &division, &partyType))
		require.NotNil(h.t, division, "dimensi divisi wajib A-19")
		assert.Equal(h.t, h.division, *division)
		require.NotNil(h.t, partyType)
		assert.Equal(h.t, "supplier", *partyType)
		out[code] = [2]string{debit, credit}
	}
	require.NoError(h.t, rows.Err())
	return out
}

func today() string { return clock.BusinessDateOf(monday09).String() }

func plusDays(n int) string { return clock.BusinessDateOf(monday09).AddDays(n).String() }

// TestPurchaseFlow: PR → approval Head divisi → PO dari PR → approval 3 level → uang muka A-19 → penerimaan →
// potong uang muka, plus pemenuhan PR dan notifikasi ETA H-1.
func TestPurchaseFlow(t *testing.T) {
	h, m := setup(t)
	tube := h.masters.product("PKG-TUBE30", "packaging")
	pouch := h.masters.product("PKG-POUCH", "packaging")
	supplier := h.masters.supplier("KEMAS", false)

	// 1. Draft PR divisi peminta lalu ajukan.
	var pr purchaseRequest
	require.Equal(t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-requests", map[string]any{
		"divisionId": h.division, "businessDate": today(), "note": "Stok packaging Q4",
		"lines": []map[string]any{
			{"productId": tube, "qty": "1000", "estimatedPrice": "50000"},
			{"productId": pouch, "qty": "200", "estimatedPrice": "30000"},
		},
	}, &pr))
	assert.Equal(t, "draft", pr.Status)
	assert.Empty(t, pr.Number)
	assert.Equal(t, "56000000.00", pr.EstimatedTotal)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-requests/"+pr.ID.String()+"/submit",
		map[string]any{"version": pr.Version}, &pr))
	assert.Equal(t, "pending_approval", pr.Status)
	assert.Equal(t, "PR/2026/10/00001", pr.Number)

	// 2. Head divisi peminta menyetujui (satu level) → PR Disetujui.
	decided := h.approve(h.divHead, "purchase_request", pr.ID)
	assert.Equal(t, "approved", decided.Status)
	assert.Equal(t, "/procurement/purchase-requests/"+pr.ID.String(), decided.SubjectLink)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-requests/"+pr.ID.String(), nil, &pr))
	assert.Equal(t, "approved", pr.Status)
	assert.Contains(t, pr.AllowedActions, "create_order")

	// 3. PO dari PR: tube penuh, pouch sebagian; qty melebihi sisa PR ditolak.
	poBody := func(pouchQty string) map[string]any {
		return map[string]any{
			"orderType": "packaging", "supplierId": supplier, "divisionId": h.division, "businessDate": today(),
			"expectedDate": plusDays(1),
			"lines": []map[string]any{
				{"productId": tube, "qty": "1000", "unitPrice": "50000", "purchaseRequestLineId": pr.Lines[0].ID},
				{"productId": pouch, "qty": pouchQty, "unitPrice": "60000", "purchaseRequestLineId": pr.Lines[1].ID},
			},
		}
	}
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders", poBody("201"), nil))
	var po purchaseOrder
	require.Equal(t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders", poBody("100"), &po))
	assert.Equal(t, "56000000.00", po.TotalAmount, "NON-PKP: tanpa PPN")
	assert.Equal(t, "PR/2026/10/00001", po.Lines[0].PurchaseRequestNumber)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-requests/"+pr.ID.String(), nil, &pr))
	assert.Equal(t, "approved", pr.Status, "PR baru terpenuhi sebagian")
	assert.Equal(t, "100.000", pr.Lines[1].RemainingQty)

	// Uang muka sebelum PO disetujui ditolak.
	assert.Equal(t, http.StatusConflict, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances", map[string]any{
		"purchaseOrderId": po.ID, "amount": "1000", "paymentDate": today(), "bankAccountCode": "1111",
	}, nil))

	// 4. Ajukan PO > Rp50 jt → 3 level: Head F&A, Direksi, Direktur Utama.
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders/"+po.ID.String()+"/submit",
		map[string]any{"version": po.Version}, &po))
	assert.Equal(t, "PO/2026/10/00001", po.Number)
	first := h.approve(h.hfa, "purchase_order", po.ID)
	assert.Equal(t, 3, first.TotalLevels)
	assert.Equal(t, "pending_approval", first.Status)
	h.approve(h.director, "purchase_order", po.ID)
	last := h.approve(h.presDir, "purchase_order", po.ID)
	assert.Equal(t, "approved", last.Status)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	assert.Equal(t, "approved", po.Status)
	assert.Contains(t, po.AllowedActions, "print")
	assert.Contains(t, po.AllowedActions, "create_advance")

	// 5. Uang muka 20 jt → posting A-19 balance (Dr 1401 / Cr 1111) dengan divisi PO dan pihak pemasok.
	var adv purchaseAdvance
	require.Equal(t, http.StatusCreated, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances", map[string]any{
		"purchaseOrderId": po.ID, "amount": "20000000", "paymentDate": today(), "bankAccountCode": "1111", "note": "DP",
	}, &adv))
	assert.Equal(t, "draft", adv.Status)
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances", map[string]any{
		"purchaseOrderId": po.ID, "amount": "36000000.01", "paymentDate": today(), "bankAccountCode": "1111",
	}, nil), "total uang muka melebihi total PO")
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances/"+adv.ID.String()+"/post",
		map[string]any{"version": adv.Version}, &adv))
	assert.Equal(t, "posted", adv.Status)
	assert.Equal(t, "BYR/2026/10/00001", adv.Number)
	require.NotNil(t, adv.JournalEntryID)
	lines := h.journalLines(*adv.JournalEntryID)
	assert.Equal(t, map[string][2]string{"1401": {"20000000.00", "0.00"}, "1111": {"0.00", "20000000.00"}}, lines)

	// PO dengan uang muka aktif tidak bisa dibatalkan.
	assert.Equal(t, http.StatusConflict, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders/"+po.ID.String()+"/cancel",
		map[string]any{"version": po.Version, "reason": "batal"}, nil))

	// 6. API untuk Penerimaan Barang & Tagihan: terima sebagian, potong uang muka FIFO.
	sys := h.as(h.purchasing)
	ro, err := m.ReceivableOrder(sys, po.ID)
	require.NoError(t, err)
	require.Len(t, ro.Lines, 2)
	received, err := m.RecordReceipt(sys, po.ID, "GR/2026/10/00001", []procurement.QtyDelta{
		{LineID: ro.Lines[0].ID, Qty: decimal.NewFromInt(400)},
	})
	require.NoError(t, err)
	assert.Equal(t, procurement.StatusPartiallyReceived, received.Status)
	_, err = m.RecordReceipt(sys, po.ID, "GR/2026/10/00002", []procurement.QtyDelta{
		{LineID: ro.Lines[0].ID, Qty: decimal.NewFromInt(601)},
	})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "toleransi qty 0%%")
	outstanding, err := m.OutstandingAdvanceAmount(sys, po.ID)
	require.NoError(t, err)
	assert.Equal(t, "20000000.00", outstanding.String())
	applied, err := m.ApplyAdvances(sys, po.ID, money.MustParse("5000000"))
	require.NoError(t, err)
	require.Len(t, applied, 1)
	assert.Equal(t, "5000000.00", applied[0].Amount.String())
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	assert.Equal(t, "20000000.00", po.AdvancePaidAmount)
	assert.Equal(t, "15000000.00", po.AdvanceOutstandingAmount)
	assert.Contains(t, po.AllowedActions, "close")
	// Uang muka yang sudah dipotong tagihan tidak bisa dibatalkan; setelah potongan dikembalikan bisa.
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodGet, "/api/v1/purchase-advances/"+adv.ID.String(), nil, &adv))
	assert.Equal(t, http.StatusConflict, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances/"+adv.ID.String()+"/cancel",
		map[string]any{"version": adv.Version, "reason": "salah"}, nil))
	require.NoError(t, m.ReleaseAdvances(sys, applied))

	// 7. PO kedua menyelesaikan PR; batalkan PO kedua → PR kembali Disetujui.
	var po2 purchaseOrder
	require.Equal(t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders", map[string]any{
		"orderType": "packaging", "supplierId": supplier, "divisionId": h.division, "businessDate": today(),
		"expectedDate": plusDays(3),
		"lines":        []map[string]any{{"productId": pouch, "qty": "100", "unitPrice": "60000", "purchaseRequestLineId": pr.Lines[1].ID}},
	}, &po2))
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-requests/"+pr.ID.String(), nil, &pr))
	assert.Equal(t, "completed", pr.Status)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders/"+po2.ID.String()+"/cancel",
		map[string]any{"version": po2.Version}, &po2))
	assert.Equal(t, "cancelled", po2.Status)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-requests/"+pr.ID.String(), nil, &pr))
	assert.Equal(t, "approved", pr.Status)
	assert.Equal(t, "100.000", pr.Lines[1].RemainingQty)

	// 8. Tutup PO diterima sebagian → qty PR yang terpenuhi = qty diterima.
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+po.ID.String(), nil, &po))
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders/"+po.ID.String()+"/close",
		map[string]any{"version": po.Version, "reason": "Sisa tidak dikirim pemasok"}, &po))
	assert.Equal(t, "closed", po.Status)
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-requests/"+pr.ID.String(), nil, &pr))
	assert.Equal(t, "400.000", pr.Lines[0].FulfilledQty)
	assert.Equal(t, "200.000", pr.Lines[1].RemainingQty)

	// 9. Batalkan uang muka yang belum dipotong → jurnal A-19 dibalik.
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodGet, "/api/v1/purchase-advances/"+adv.ID.String(), nil, &adv))
	require.Equal(t, http.StatusOK, h.call(h.finance, http.MethodPost, "/api/v1/purchase-advances/"+adv.ID.String()+"/cancel",
		map[string]any{"version": adv.Version, "reason": "Salah PO"}, &adv))
	assert.Equal(t, "cancelled", adv.Status)
	var reversals int
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM journal_entries WHERE reversal_of_id = $1`, *adv.JournalEntryID).Scan(&reversals))
	assert.Equal(t, 1, reversals)
}

func TestTollManufacturingAndETA(t *testing.T) {
	h, m := setup(t)
	fg := h.masters.product("FG-WHEY1KG", "finished_good")
	tube := h.masters.product("PKG-TUBE1KG", "packaging")
	gm := h.masters.product("GM-WHEY-RET", "reprocess_material")
	maklon := h.masters.supplier("NUTRI", true)
	noLocation := h.masters.supplier("TANPALOKASI", true)
	tollLoc := h.masters.location("MKL-NUTRI", "toll_manufacturer")
	h.masters.toll[maklon] = tollLoc
	gmLoc := h.masters.location("UTAMA-GM", "goods_material")
	rmLoc := h.masters.location("UTAMA-RM", "raw_material")

	body := func(orderType string, supplier uuid.UUID, material uuid.UUID, source *uuid.UUID) map[string]any {
		return map[string]any{
			"orderType": orderType, "supplierId": supplier, "divisionId": h.division, "businessDate": today(),
			"expectedDate": plusDays(1), "materialSourceLocationId": source,
			"lines":     []map[string]any{{"productId": fg, "qty": "5000", "unitPrice": "1500"}},
			"materials": []map[string]any{{"productId": material, "plannedQty": "5100"}},
		}
	}
	// Maklon tanpa lokasi eksternal ditolak; lokasi asal maklon harus RM.
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders",
		body("toll_manufacturing", noLocation, tube, nil), nil))
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders",
		body("toll_manufacturing", maklon, tube, &gmLoc), nil))
	var toll purchaseOrder
	require.Equal(t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders",
		body("toll_manufacturing", maklon, tube, &rmLoc), &toll))
	require.NotNil(t, toll.TollLocation)
	assert.Equal(t, "MKL-NUTRI", toll.TollLocation.Code)
	require.Len(t, toll.Materials, 1)
	assert.Equal(t, "5100.000", toll.Materials[0].PlannedQty)

	// Reprocess: bahan GM dari lokasi Gudang GM – GM.
	assert.Equal(t, http.StatusUnprocessableEntity, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders",
		body("reprocess", maklon, gm, &rmLoc), nil))
	var reprocess purchaseOrder
	require.Equal(t, http.StatusCreated, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders",
		body("reprocess", maklon, gm, &gmLoc), &reprocess))

	// PO maklon Rp7,5 jt → satu level Head F&A.
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodPost, "/api/v1/purchase-orders/"+toll.ID.String()+"/submit",
		map[string]any{"version": toll.Version}, &toll))
	decided := h.approve(h.hfa, "purchase_order", toll.ID)
	assert.Equal(t, 1, decided.TotalLevels)
	assert.Equal(t, "approved", decided.Status)

	// ETA H-1: PO disetujui yang tiba besok diberitahukan sekali ke Logistik Leader (in-app + email).
	n, err := m.NotifyETA(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 1, h.notificationCount(h.leader, "procurement.eta"))
	var emails int
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM notification_emails WHERE recipient = $1 AND body_text LIKE '%FG-WHEY1KG%'`, h.leader.Email).Scan(&emails))
	assert.Equal(t, 1, emails)
	n, err = m.NotifyETA(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n, "idempotent")

	// Pembuat tidak bisa mengubah jenis PO maupun draft yang sudah diajukan.
	require.Equal(t, http.StatusOK, h.call(h.purchasing, http.MethodGet, "/api/v1/purchase-orders/"+toll.ID.String(), nil, &toll))
	update := body("toll_manufacturing", maklon, tube, &rmLoc)
	update["version"] = toll.Version
	assert.Equal(t, http.StatusConflict, h.call(h.purchasing, http.MethodPut, "/api/v1/purchase-orders/"+toll.ID.String(), update, nil))
	_, err = m.ReceivableOrder(h.as(h.purchasing), reprocess.ID)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition))
}
