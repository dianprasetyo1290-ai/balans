package service_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting"
	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/catalog"
	"balans/internal/document"
	"balans/internal/inventory/domain"
	"balans/internal/inventory/postgres"
	"balans/internal/inventory/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/paging"
	"balans/internal/warehouse"
)

type journalNumberer struct{ docs *document.Module }

func (n journalNumberer) NextJournalNumber(ctx context.Context, d clock.BusinessDate) (string, error) {
	return n.docs.Next(ctx, document.TypeJournal, d)
}

type dimensions struct{ acc *accounting.Module }

func (d dimensions) DimensionOptions(ctx context.Context, kind string, includeInactive bool) ([]service.DimensionOption, error) {
	vals, err := d.acc.DimensionValues(ctx, accounting.DimensionKind(kind), includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]service.DimensionOption, len(vals))
	for i, v := range vals {
		o := service.DimensionOption{ID: v.ID, Code: v.Code, Name: v.Name, IsActive: v.IsActive}
		if kind == service.DimensionCampaign {
			from, to := v.ValidFrom, v.ValidTo
			o.ValidFrom, o.ValidTo = &from, &to
		}
		out[i] = o
	}
	return out, nil
}

type fakeApprovals struct {
	mu       sync.Mutex
	subjects []approval.Subject
}

func (f *fakeApprovals) Request(_ context.Context, s approval.Subject) (approval.Request, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subjects = append(f.subjects, s)
	return approval.Request{}, nil
}

func (f *fakeApprovals) last() approval.Subject {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subjects[len(f.subjects)-1]
}

type fakeReasons struct {
	m map[uuid.UUID]service.ReturnReason
}

func (f fakeReasons) ReturnReason(_ context.Context, id uuid.UUID) (service.ReturnReason, error) {
	r, ok := f.m[id]
	if !ok {
		return service.ReturnReason{}, apperr.New(apperr.CodeNotFound)
	}
	return r, nil
}

type fakeFiles struct{ attached map[uuid.UUID]uuid.UUID }

func (f *fakeFiles) Attach(_ context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	if ownerType != service.FileOwnerDisposal {
		return fmt.Errorf("owner type %s", ownerType)
	}
	f.attached[fileID] = ownerID
	return nil
}

func (f *fakeFiles) Names(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		out[id] = "foto.jpg"
	}
	return out, nil
}

type fakeNotifier struct {
	mu    sync.Mutex
	calls [][]uuid.UUID
}

func (f *fakeNotifier) Notify(_ context.Context, ids []uuid.UUID, _, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, ids)
	return nil
}

type fakeMembers struct{ ids []uuid.UUID }

func (f fakeMembers) UserIDsWithRole(context.Context, string) ([]uuid.UUID, error) { return f.ids, nil }

type docHarness struct {
	*harness
	acc       *accounting.Module
	approvals *fakeApprovals
	files     *fakeFiles
	notifier  *fakeNotifier
	leader    uuid.UUID
	reasons   map[uuid.UUID]service.ReturnReason
	warehouse uuid.UUID
	locs      map[warehouse.LocationType]uuid.UUID
	marketing uuid.UUID
	campaign  uuid.UUID
}

var allDocPerms = []string{
	service.PermStockRead,
	domain.PermUsageRead, domain.PermUsageWrite, domain.PermUsageSubmit, domain.PermUsagePost,
	domain.PermRepackRead, domain.PermRepackWrite, domain.PermRepackPost,
	domain.PermDisposalRead, domain.PermDisposalWrite, domain.PermDisposalSubmit, domain.PermDisposalPost,
	domain.PermDispositionRead, domain.PermDispositionWrite, domain.PermDispositionSubmit,
}

func setupDocs(t *testing.T) *docHarness {
	t.Helper()
	h := &docHarness{harness: setup(t), approvals: &fakeApprovals{}, files: &fakeFiles{attached: map[uuid.UUID]uuid.UUID{}},
		notifier: &fakeNotifier{}, leader: uuid.New(), reasons: map[uuid.UUID]service.ReturnReason{}}
	ctx := context.Background()
	_, err := h.pool.Exec(ctx, `UPDATE fiscal_periods SET status = 'open', closed_at = NULL`)
	require.NoError(t, err)
	tx := db.NewTxManager(h.pool)
	docs := document.New(document.Deps{Pool: h.pool, Clock: h.clock})
	rec := audit.New(audit.Deps{Pool: h.pool})
	h.acc = accounting.New(accounting.Deps{Pool: h.pool, Tx: tx, Clock: h.clock, Audit: rec, Numberer: journalNumberer{docs: docs}})
	h.svc = service.New(service.Deps{
		Store: postgres.NewStore(h.pool), Tx: tx, Clock: h.clock, Products: h.products, Locations: h.harness.locs,
		Periods: h.periods, Audit: rec, Approvals: h.approvals, Poster: h.acc, Numberer: docs,
		Dimensions: dimensions{acc: h.acc}, ReturnReasons: fakeReasons{m: h.reasons}, Files: h.files,
		Notifier: h.notifier, Members: fakeMembers{ids: []uuid.UUID{h.leader}},
	})
	set := map[string]struct{}{}
	for _, p := range allDocPerms {
		set[p] = struct{}{}
	}
	h.ctx = actor.With(context.Background(), actor.Actor{UserID: uuid.New(), FullName: "Pengguna Uji", Permissions: set})

	h.warehouse = uuid.New()
	h.locs = map[warehouse.LocationType]uuid.UUID{}
	for _, lt := range []warehouse.LocationType{
		warehouse.LocationInbound, warehouse.LocationFinishedGoods, warehouse.LocationReturns, warehouse.LocationGoodsMaterial,
		warehouse.LocationRawMaterial, warehouse.LocationEvent,
	} {
		h.locs[lt] = h.typedLocation(lt)
	}
	divs, err := dimensions{acc: h.acc}.DimensionOptions(ctx, service.DimensionDivision, false)
	require.NoError(t, err)
	for _, d := range divs {
		if d.Code == "MKT" {
			h.marketing = d.ID
		}
	}
	require.NotEqual(t, uuid.Nil, h.marketing)
	code := "KMP-202610-" + strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
	err = h.pool.QueryRow(ctx, `INSERT INTO campaigns (code, name, valid_from, valid_to, is_active, created_by, updated_by)
		VALUES ($1, 'Kampanye Uji', '2026-10-01', '2026-12-31', true, $2, $2) RETURNING id`, code, uuid.Nil).Scan(&h.campaign)
	require.NoError(t, err)
	return h
}

func (h *docHarness) typedLocation(lt warehouse.LocationType) uuid.UUID {
	id := uuid.New()
	wh := h.warehouse
	h.harness.locs.mu.Lock()
	defer h.harness.locs.mu.Unlock()
	h.harness.locs.m[id] = warehouse.Location{
		ID: id, Code: string(lt) + "-" + id.String()[:4], Name: lt.Label(), WarehouseID: &wh, Type: lt,
		IsSellable: lt.Sellable(), IsActive: true, IsStandard: true,
	}
	return id
}

func (h *docHarness) typedProduct(pt catalog.ProductType) uuid.UUID {
	id := uuid.New()
	shelf := 365
	p := catalog.Product{ID: id, SKU: "SKU-" + id.String()[:6], Name: "Produk", UOM: "pcs", Type: pt, IsActive: true}
	if pt == catalog.TypeFinishedGood {
		p.HasBatch, p.ShelfLifeDays = true, &shelf
	}
	h.products.mu.Lock()
	defer h.products.mu.Unlock()
	h.products.m[id] = p
	return id
}

// receive memposting saldo awal satu batch pada tanggal bisnis.
func (h *docHarness) receive(product uuid.UUID, batch string, loc uuid.UUID, st domain.StockStatus, qty, cost string, date clock.BusinessDate) *uuid.UUID {
	h.t.Helper()
	q, c := dec(qty), dec(cost)
	line := domain.Line{Kind: domain.KindReceive, ProductID: product, Qty: q, UnitCost: &c, To: &domain.Place{LocationID: loc, Status: st}}
	if batch != "" {
		exp := date.AddDays(300)
		line.Batch = &domain.NewBatch{Number: batch, ExpiryDate: &exp}
	}
	res, err := h.svc.Post(h.ctx, domain.Posting{
		Source: domain.Source{Type: "opening_balance", ID: uuid.New(), Label: "Saldo awal", BusinessDate: date}, Lines: []domain.Line{line},
	})
	require.NoError(h.t, err)
	return res.Movements[0].BatchID
}

func (h *docHarness) qty(product, loc uuid.UUID, st domain.StockStatus) decimal.Decimal {
	h.t.Helper()
	var q decimal.Decimal
	err := h.pool.QueryRow(context.Background(),
		`SELECT coalesce(sum(qty), 0) FROM stock_balances WHERE product_id = $1 AND location_id = $2 AND stock_status = $3`,
		product, loc, string(st)).Scan(&q)
	require.NoError(h.t, err)
	return q
}

type journalLine struct {
	account       string
	debit, credit decimal.Decimal
	division      *uuid.UUID
	campaign      *uuid.UUID
}

// journal membaca baris jurnal dan memastikan balance.
func (h *docHarness) journal(id *uuid.UUID) (string, []journalLine) {
	h.t.Helper()
	require.NotNil(h.t, id, "jurnal belum diposting")
	ctx := context.Background()
	var rule string
	require.NoError(h.t, h.pool.QueryRow(ctx, `SELECT posting_rule_code FROM journal_entries WHERE id = $1`, *id).Scan(&rule))
	rows, err := h.pool.Query(ctx, `SELECT a.code, l.debit_amount, l.credit_amount, l.division_id, l.campaign_id
		FROM journal_lines l JOIN accounts a ON a.id = l.account_id WHERE l.journal_entry_id = $1 ORDER BY l.line_no`, *id)
	require.NoError(h.t, err)
	defer rows.Close()
	var out []journalLine
	debit, credit := decimal.Zero, decimal.Zero
	for rows.Next() {
		var l journalLine
		require.NoError(h.t, rows.Scan(&l.account, &l.debit, &l.credit, &l.division, &l.campaign))
		debit, credit = debit.Add(l.debit), credit.Add(l.credit)
		out = append(out, l)
	}
	require.NoError(h.t, rows.Err())
	require.True(h.t, debit.Equal(credit), "jurnal tidak balance: %s vs %s", debit, credit)
	require.True(h.t, debit.IsPositive())
	return rule, out
}

func amountOn(lines []journalLine, account string) (debit, credit decimal.Decimal) {
	debit, credit = decimal.Zero, decimal.Zero
	for _, l := range lines {
		if l.account == account {
			debit, credit = debit.Add(l.debit), credit.Add(l.credit)
		}
	}
	return debit, credit
}

func (h *docHarness) decide(t approval.SubjectType, id uuid.UUID, approve bool) {
	h.t.Helper()
	for _, r := range h.svc.ApprovalRegistrations() {
		if r.Type != t {
			continue
		}
		fn := r.Handler.OnRejected
		if approve {
			fn = r.Handler.OnApproved
		}
		err := db.NewTxManager(h.pool).WithTx(h.ctx, func(ctx context.Context) error {
			return fn(ctx, approval.Decided{Subject: approval.Subject{Type: t, ID: id}, DecidedBy: uuid.New()})
		})
		require.NoError(h.t, err)
		return
	}
	h.t.Fatalf("handler %s tidak terdaftar", t)
}

func TestInternalUsageMarketingFlow(t *testing.T) {
	h := setupDocs(t)
	fg := h.locs[warehouse.LocationFinishedGoods]
	p := h.typedProduct(catalog.TypeFinishedGood)
	h.receive(p, "B1", fg, domain.StatusAvailable, "10", "1000", today.AddDays(-10))
	h.receive(p, "B2", fg, domain.StatusAvailable, "10", "1200", today.AddDays(-5))

	in := domain.InternalUsageInput{
		Purpose: domain.PurposeMarketing, DivisionID: h.marketing, SourceLocationID: fg, BusinessDate: today,
		Lines: []domain.UsageLineInput{{ProductID: p, Qty: dec("12")}},
	}
	_, err := h.svc.CreateUsage(h.ctx, in)
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "kampanye wajib untuk marketing")
	in.CampaignID = &h.campaign
	u, err := h.svc.CreateUsage(h.ctx, in)
	require.NoError(t, err)

	u, err = h.svc.SubmitUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocPendingApproval, u.Status)
	assert.True(t, strings.HasPrefix(u.Number, "PU/2026/10/"))
	subj := h.approvals.last()
	assert.Equal(t, approval.SubjectInternalUsage, subj.Type)
	assert.Equal(t, "12400.00", subj.Amount.String(), "estimasi HPP FEFO 10×1000 + 2×1200")
	require.NotNil(t, subj.Division)
	assert.Equal(t, h.marketing, *subj.Division)

	_, err = h.svc.ReleaseUsage(h.ctx, u.ID, u.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "release sebelum disetujui")
	h.decide(approval.SubjectInternalUsage, u.ID, true)
	v, err := h.svc.GetUsage(h.ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DocApproved, v.Usage.Status)
	assert.Contains(t, v.AllowedActions, domain.ActionRelease)

	u, err = h.svc.ReleaseUsage(h.ctx, u.ID, v.Usage.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocInProgress, u.Status)
	assert.True(t, h.qty(p, fg, domain.StatusReserved).Equal(dec("12")))

	u, err = h.svc.HandoverUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocPosted, u.Status)
	assert.Equal(t, "12400.00", u.CostAmount.StringFixed(2))
	assert.True(t, h.qty(p, fg, domain.StatusReserved).IsZero())
	assert.True(t, h.qty(p, fg, domain.StatusAvailable).Equal(dec("8")))
	rule, lines := h.journal(u.JournalID)
	assert.Equal(t, "A-11", rule)
	d6103, _ := amountOn(lines, "6103")
	_, c1311 := amountOn(lines, "1311")
	assert.Equal(t, "12400.00", d6103.StringFixed(2))
	assert.Equal(t, "12400.00", c1311.StringFixed(2))
	for _, l := range lines {
		require.NotNil(t, l.campaign)
		assert.Equal(t, h.campaign, *l.campaign)
	}
}

func TestInternalUsageEmployeeSampleAndReject(t *testing.T) {
	h := setupDocs(t)
	fg := h.locs[warehouse.LocationFinishedGoods]
	p := h.typedProduct(catalog.TypeFinishedGood)
	h.receive(p, "S1", fg, domain.StatusAvailable, "5", "1500", today.AddDays(-3))
	in := domain.InternalUsageInput{
		Purpose: domain.PurposeEmployeeSample, DivisionID: h.marketing, SourceLocationID: fg, BusinessDate: today,
		Lines: []domain.UsageLineInput{{ProductID: p, Qty: dec("2")}},
	}
	u, err := h.svc.CreateUsage(h.ctx, in)
	require.NoError(t, err)
	u, err = h.svc.SubmitUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	h.decide(approval.SubjectInternalUsage, u.ID, false)
	v, err := h.svc.GetUsage(h.ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DocRejected, v.Usage.Status)
	u, err = h.svc.ReviseUsage(h.ctx, u.ID, v.Usage.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocDraft, u.Status)
	u, err = h.svc.SubmitUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	h.decide(approval.SubjectInternalUsage, u.ID, true)
	v, _ = h.svc.GetUsage(h.ctx, u.ID)
	u, err = h.svc.ReleaseUsage(h.ctx, u.ID, v.Usage.Version)
	require.NoError(t, err)
	_, err = h.svc.HandoverUsage(h.ctx, u.ID, u.Version-1)
	assert.True(t, apperr.HasCode(err, apperr.CodeVersionConflict))
	u, err = h.svc.HandoverUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	rule, lines := h.journal(u.JournalID)
	assert.Equal(t, "A-12", rule)
	d, _ := amountOn(lines, "6203")
	assert.Equal(t, "3000.00", d.StringFixed(2))
}

func TestInternalUsageEventFlow(t *testing.T) {
	h := setupDocs(t)
	fg, evt, inb := h.locs[warehouse.LocationFinishedGoods], h.locs[warehouse.LocationEvent], h.locs[warehouse.LocationInbound]
	p := h.typedProduct(catalog.TypeFinishedGood)
	b1 := h.receive(p, "E1", fg, domain.StatusAvailable, "6", "1000", today.AddDays(-10))
	b2 := h.receive(p, "E2", fg, domain.StatusAvailable, "10", "2000", today.AddDays(-5))
	end := today.AddDays(1)
	u, err := h.svc.CreateUsage(h.ctx, domain.InternalUsageInput{
		Purpose: domain.PurposeEvent, DivisionID: h.marketing, CampaignID: &h.campaign, SourceLocationID: fg,
		EventLocationID: &evt, EventEndDate: &end, BusinessDate: today,
		Lines: []domain.UsageLineInput{{ProductID: p, Qty: dec("10")}},
	})
	require.NoError(t, err)
	require.NotNil(t, u.SettleDueDate)
	assert.Equal(t, today.AddDays(2), *u.SettleDueDate)
	u, err = h.svc.SubmitUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	h.decide(approval.SubjectInternalUsage, u.ID, true)
	v, _ := h.svc.GetUsage(h.ctx, u.ID)
	u, err = h.svc.ReleaseUsage(h.ctx, u.ID, v.Usage.Version)
	require.NoError(t, err)
	u, err = h.svc.HandoverUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocAwaitingSettlement, u.Status)
	assert.Nil(t, u.JournalID, "pindah ke lokasi Event tanpa jurnal")
	assert.True(t, h.qty(p, evt, domain.StatusAvailable).Equal(dec("10")))

	// Pengingat: belum lewat batas → tidak ada; setelah H+1 → peminta + Logistik Leader, sekali per hari.
	n, err := h.svc.RemindSettlements(h.ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	h.clock.Advance(48 * time.Hour)
	n, err = h.svc.RemindSettlements(h.ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
	assert.Contains(t, h.notifier.calls[len(h.notifier.calls)-1], h.leader)
	n2, err := h.svc.RemindSettlements(h.ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n2)

	v, _ = h.svc.GetUsage(h.ctx, u.ID)
	_, err = h.svc.PostUsage(h.ctx, u.ID, v.Usage.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeInvalidStateTransition), "posting sebelum sisa dicatat")
	_, err = h.svc.SettleUsage(h.ctx, u.ID, v.Usage.Version, []domain.SettleLine{{LineID: u.Lines[0].ID, ReturnedQty: dec("11")}})
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed))
	u, err = h.svc.SettleUsage(h.ctx, u.ID, v.Usage.Version, []domain.SettleLine{{LineID: u.Lines[0].ID, ReturnedQty: dec("3")}})
	require.NoError(t, err)
	assert.True(t, h.qty(p, inb, domain.StatusQuarantine).Equal(dec("3")), "sisa kembali ke Inbound berstatus Karantina")
	assert.True(t, h.qty(p, evt, domain.StatusAvailable).Equal(dec("7")))

	u, err = h.svc.PostUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocPosted, u.Status)
	l := u.Lines[0]
	assert.True(t, l.Qty.Equal(l.ConsumedQty.Add(l.ReturnedQty)), "qty kirim = terpakai + sisa")
	assert.True(t, l.ConsumedQty.Equal(dec("7")))
	assert.True(t, h.qty(p, evt, domain.StatusAvailable).IsZero())
	// FEFO: E1 (6 @1000) + E2 (4 @2000) dikirim; sisa 3 dianggap dari E2 → terpakai 6×1000 + 1×2000.
	assert.Equal(t, "8000.00", u.CostAmount.StringFixed(2))
	rule, lines := h.journal(u.JournalID)
	assert.Equal(t, "A-11", rule)
	d, _ := amountOn(lines, "6103")
	assert.Equal(t, "8000.00", d.StringFixed(2))
	_ = b1
	_ = b2
}

func TestRepackingFlow(t *testing.T) {
	h := setupDocs(t)
	ret, rm, fg := h.locs[warehouse.LocationReturns], h.locs[warehouse.LocationRawMaterial], h.locs[warehouse.LocationFinishedGoods]
	p := h.typedProduct(catalog.TypeFinishedGood)
	pkg := h.typedProduct(catalog.TypePackaging)
	b := h.receive(p, "R1", ret, domain.StatusAvailable, "10", "1000", today.AddDays(-5))
	h.receive(pkg, "", rm, domain.StatusAvailable, "100", "50", today.AddDays(-5))
	exp := today.AddDays(200)
	r, err := h.svc.CreateRepacking(h.ctx, domain.RepackingInput{BusinessDate: today, Lines: []domain.RepackLineInput{
		{Direction: domain.RepackInput, ProductID: p, LocationID: ret, BatchID: b, Qty: dec("10")},
		{Direction: domain.RepackInput, ProductID: pkg, LocationID: rm, Qty: dec("10")},
		{Direction: domain.RepackOutput, ProductID: p, LocationID: fg, BatchNumber: "RPK-NEW", ExpiryDate: &exp, Qty: dec("10")},
	}})
	require.NoError(t, err)
	r, err = h.svc.StartRepacking(h.ctx, r.ID, r.Version)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(r.Number, "RPK/2026/10/"))
	assert.True(t, h.qty(p, ret, domain.StatusReserved).Equal(dec("10")))
	r, err = h.svc.PostRepacking(h.ctx, r.ID, r.Version)
	require.NoError(t, err)
	assert.Equal(t, "10500.00", r.CostAmount.StringFixed(2), "HPP retur 10.000 + packaging 500")
	assert.True(t, h.qty(p, fg, domain.StatusAvailable).Equal(dec("10")))
	assert.True(t, h.qty(p, ret, domain.StatusReserved).IsZero())
	out := r.Output()
	require.NotNil(t, out.BatchID)
	var unit decimal.Decimal
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT unit_cost FROM cost_layers WHERE batch_id = $1`, *out.BatchID).Scan(&unit))
	assert.Equal(t, "1050.0000", unit.StringFixed(4))
	rule, lines := h.journal(r.JournalID)
	assert.Equal(t, "A-26", rule)
	d1311, _ := amountOn(lines, "1311")
	_, c1314 := amountOn(lines, "1314")
	_, c1301 := amountOn(lines, "1301")
	assert.Equal(t, "10500.00", d1311.StringFixed(2))
	assert.Equal(t, "10000.00", c1314.StringFixed(2))
	assert.Equal(t, "500.00", c1301.StringFixed(2))
}

func TestDisposalFlow(t *testing.T) {
	h := setupDocs(t)
	fg := h.locs[warehouse.LocationFinishedGoods]
	p := h.typedProduct(catalog.TypeFinishedGood)
	b := h.receive(p, "X1", fg, domain.StatusAvailable, "4", "2500", today.AddDays(-30))
	d, err := h.svc.CreateDisposal(h.ctx, domain.DisposalInput{
		Reason: domain.DisposalExpired, DisposalReport: domain.DisposalReport{DisposalDate: today},
		Lines: []domain.DisposalLineInput{{ProductID: p, BatchID: b, LocationID: fg, Qty: dec("4")}},
	})
	require.NoError(t, err)
	d, err = h.svc.SubmitDisposal(h.ctx, d.ID, d.Version)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(d.Number, "MUS/2026/10/"))
	subj := h.approvals.last()
	assert.Equal(t, approval.SubjectDisposal, subj.Type)
	assert.Equal(t, "10000.00", subj.Amount.String())
	h.decide(approval.SubjectDisposal, d.ID, true)
	v, err := h.svc.GetDisposal(h.ctx, d.ID)
	require.NoError(t, err)
	_, err = h.svc.PostDisposal(h.ctx, d.ID, v.Disposal.Version)
	assert.True(t, apperr.HasCode(err, apperr.CodeValidationFailed), "berita acara wajib lengkap")
	photo := uuid.New()
	d, err = h.svc.UpdateDisposalReport(h.ctx, d.ID, v.Disposal.Version, domain.DisposalReport{
		DisposalDate: today, Method: "Dihancurkan dan dibuang ke TPA", Witnesses: []string{"Budi", "Sari"}, PhotoFileIDs: []uuid.UUID{photo},
	})
	require.NoError(t, err)
	assert.Equal(t, d.ID, h.files.attached[photo])
	d, err = h.svc.PostDisposal(h.ctx, d.ID, d.Version)
	require.NoError(t, err)
	assert.True(t, h.qty(p, fg, domain.StatusAvailable).IsZero())
	rule, lines := h.journal(d.JournalID)
	assert.Equal(t, "A-27", rule)
	d6402, _ := amountOn(lines, "6402")
	assert.Equal(t, "10000.00", d6402.StringFixed(2))
}

func TestReturnDispositionFlow(t *testing.T) {
	h := setupDocs(t)
	ret, fg, gm := h.locs[warehouse.LocationReturns], h.locs[warehouse.LocationFinishedGoods], h.locs[warehouse.LocationGoodsMaterial]
	reasonDispose, reasonRestock := uuid.New(), uuid.New()
	h.reasons[reasonDispose] = service.ReturnReason{ID: reasonDispose, Code: "EXPIRED", Name: "Kedaluwarsa", DefaultDisposition: domain.DispositionDispose}
	h.reasons[reasonRestock] = service.ReturnReason{ID: reasonRestock, Code: "CHANGE", Name: "Berubah pikiran", DefaultDisposition: domain.DispositionRestock}
	prods := make([]uuid.UUID, 5)
	batches := make([]*uuid.UUID, 5)
	for i := range prods {
		prods[i] = h.typedProduct(catalog.TypeFinishedGood)
		batches[i] = h.receive(prods[i], fmt.Sprintf("RT%d", i), ret, domain.StatusAvailable, "10", "1000", today.AddDays(-4))
	}
	line := func(i int, reason *uuid.UUID, disp domain.Disposition) domain.DispositionLineInput {
		return domain.DispositionLineInput{ProductID: prods[i], BatchID: batches[i], Qty: dec("10"), ReasonID: reason, Disposition: disp, QCNote: "Segel utuh"}
	}
	d, err := h.svc.CreateDisposition(h.ctx, domain.ReturnDispositionInput{LocationID: ret, BusinessDate: today, Lines: []domain.DispositionLineInput{
		line(0, &reasonRestock, ""), // default alasan → restock
		line(1, nil, domain.DispositionReprocess),
		line(2, nil, domain.DispositionRepack),
		line(3, nil, domain.DispositionSample),
		line(4, &reasonDispose, ""), // default alasan → dispose
	}})
	require.NoError(t, err)
	assert.Equal(t, domain.DispositionRestock, d.Lines[0].Disposition)
	assert.Equal(t, domain.DispositionDispose, d.Lines[4].Disposition)
	d, err = h.svc.SubmitDisposition(h.ctx, d.ID, d.Version)
	require.NoError(t, err)
	subj := h.approvals.last()
	assert.Equal(t, approval.SubjectReturnDisposition, subj.Type)
	assert.Equal(t, []string{approval.ConditionHasDisposal}, subj.Flags)
	assert.Equal(t, "10000.00", subj.Amount.String(), "nominal = nilai pemusnahan")
	h.decide(approval.SubjectReturnDisposition, d.ID, true)
	v, err := h.svc.GetDisposition(h.ctx, d.ID)
	require.NoError(t, err)
	d, err = h.svc.PostDisposition(h.ctx, d.ID, v.Disposition.Version)
	require.NoError(t, err)
	assert.Equal(t, domain.DocPosted, d.Status)
	assert.True(t, h.qty(prods[0], fg, domain.StatusAvailable).Equal(dec("10")), "restock → FG")
	assert.True(t, h.qty(prods[1], gm, domain.StatusAvailable).Equal(dec("10")), "reprocess → GM")
	rule, lines := h.journal(d.JournalID)
	assert.Equal(t, "A-26", rule)
	d1311, _ := amountOn(lines, "1311")
	d1302, _ := amountOn(lines, "1302")
	_, c1314 := amountOn(lines, "1314")
	assert.Equal(t, "10000.00", d1311.StringFixed(2))
	assert.Equal(t, "10000.00", d1302.StringFixed(2))
	assert.Equal(t, "20000.00", c1314.StringFixed(2))

	// Dokumen lanjutan per baris.
	assert.Equal(t, domain.TargetRepacking, d.Lines[2].TargetType)
	rp, err := h.svc.GetRepacking(h.ctx, *d.Lines[2].TargetID)
	require.NoError(t, err)
	assert.Equal(t, domain.DocDraft, rp.Repacking.Status)

	assert.Equal(t, domain.TargetInternalUsage, d.Lines[3].TargetType)
	uv, err := h.svc.GetUsage(h.ctx, *d.Lines[3].TargetID)
	require.NoError(t, err)
	assert.Equal(t, domain.DocApproved, uv.Usage.Status)
	u, err := h.svc.ReleaseUsage(h.ctx, uv.Usage.ID, uv.Usage.Version)
	require.NoError(t, err)
	u, err = h.svc.HandoverUsage(h.ctx, u.ID, u.Version)
	require.NoError(t, err)
	rule, lines = h.journal(u.JournalID)
	assert.Equal(t, "A-26", rule, "sampel dari Gudang Retur: Dr 6103, Cr 1314")
	d6103, _ := amountOn(lines, "6103")
	_, c1314 = amountOn(lines, "1314")
	assert.Equal(t, "10000.00", d6103.StringFixed(2))
	assert.Equal(t, "10000.00", c1314.StringFixed(2))

	assert.Equal(t, domain.TargetDisposal, d.Lines[4].TargetType)
	dv, err := h.svc.GetDisposal(h.ctx, *d.Lines[4].TargetID)
	require.NoError(t, err)
	assert.Equal(t, domain.DocApproved, dv.Disposal.Status)
	assert.True(t, strings.HasPrefix(dv.Disposal.Number, "MUS/"))
	ds, err := h.svc.UpdateDisposalReport(h.ctx, dv.Disposal.ID, dv.Disposal.Version, domain.DisposalReport{
		DisposalDate: today, Method: "Dibakar", Witnesses: []string{"Andi"}, PhotoFileIDs: []uuid.UUID{uuid.New()},
	})
	require.NoError(t, err)
	ds, err = h.svc.PostDisposal(h.ctx, ds.ID, ds.Version)
	require.NoError(t, err)
	rule, lines = h.journal(ds.JournalID)
	assert.Equal(t, "A-26", rule)
	d6403, _ := amountOn(lines, "6403")
	assert.Equal(t, "10000.00", d6403.StringFixed(2))
	assert.True(t, h.qty(prods[4], ret, domain.StatusAvailable).IsZero())
}

func TestRecostBackdatedReceipt(t *testing.T) {
	h := setupDocs(t)
	fg := h.locs[warehouse.LocationFinishedGoods]
	p := h.typedProduct(catalog.TypeFinishedGood)
	batch := h.receive(p, "RC1", fg, domain.StatusAvailable, "10", "100", today.AddDays(-10))
	// Keluar 5 pada H-5 dengan HPP 100.
	issue := func(qty string, date clock.BusinessDate) {
		_, err := h.svc.Post(h.ctx, domain.Posting{
			Source: domain.Source{Type: "delivery_order", ID: uuid.New(), Label: "DO uji", BusinessDate: date},
			Lines:  []domain.Line{{Kind: domain.KindIssue, ProductID: p, Qty: dec(qty), BatchID: batch, From: &domain.Place{LocationID: fg, Status: domain.StatusAvailable}}},
		})
		require.NoError(t, err)
	}
	issue("5", today.AddDays(-5))
	// Penerimaan mundur H-12 (sebelum layer pertama) HPP 80 → barang keluar H-5 seharusnya 5 × 80.
	exp := today.AddDays(290)
	cost := dec("80")
	srcID := uuid.New()
	_, err := h.svc.Post(h.ctx, domain.Posting{
		Source: domain.Source{Type: "goods_receipt", ID: srcID, Label: "GR mundur", BusinessDate: today.AddDays(-12)},
		Lines: []domain.Line{{Kind: domain.KindReceive, ProductID: p, Qty: dec("10"), UnitCost: &cost,
			Batch: &domain.NewBatch{Number: "RC1", ExpiryDate: &exp}, To: &domain.Place{LocationID: fg, Status: domain.StatusAvailable}}},
	})
	require.NoError(t, err)
	var runID uuid.UUID
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT id FROM recost_runs WHERE trigger_source_id = $1`, srcID).Scan(&runID))
	require.NoError(t, h.svc.Recost(h.ctx, runID))
	run, err := h.svc.RecostRun(h.ctx, runID)
	require.NoError(t, err)
	assert.Equal(t, service.RecostCompleted, run.Status)
	assert.Equal(t, "-100.00", run.AdjustmentAmount.StringFixed(2), "HPP turun 500 → 400")
	lines, err := h.svc.RecostLines(h.ctx, runID)
	require.NoError(t, err)
	require.Len(t, lines, 1)
	rule, jl := h.journal(run.JournalID)
	assert.Equal(t, "A-RC", rule)
	d1311, _ := amountOn(jl, "1311")
	_, c5101 := amountOn(jl, "5101")
	assert.Equal(t, "100.00", d1311.StringFixed(2))
	assert.Equal(t, "100.00", c5101.StringFixed(2))
	// Sisa layer: layer mundur 5 @80, layer awal 10 @100 → nilai persediaan 1.400.
	var value decimal.Decimal
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT sum(remaining_qty * unit_cost) FROM cost_layers WHERE batch_id = $1`, *batch).Scan(&value))
	assert.Equal(t, "1400.00", value.StringFixed(2))
	// Idempotent: run kedua tidak membuat selisih baru.
	require.NoError(t, h.svc.Recost(h.ctx, runID))
	again, err := h.svc.RecostLines(h.ctx, runID)
	require.NoError(t, err)
	assert.Len(t, again, 1)

	// Backdate ke periode tertutup ditolak.
	locked := today.AddDays(-20)
	h.periods.locked = &locked
	_, err = h.svc.Post(h.ctx, domain.Posting{
		Source: domain.Source{Type: "goods_receipt", ID: uuid.New(), Label: "GR tertutup", BusinessDate: today.AddDays(-25)},
		Lines: []domain.Line{{Kind: domain.KindReceive, ProductID: p, Qty: dec("1"), UnitCost: &cost, BatchID: batch,
			To: &domain.Place{LocationID: fg, Status: domain.StatusAvailable}}},
	})
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))
	h.periods.locked = nil

	page, err := h.svc.ListRecostRuns(h.ctx, &p, paging.New(nil, nil))
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	_ = time.Second
}
