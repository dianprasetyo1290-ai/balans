package pricing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/approval"
	"balans/internal/audit"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/money"
	"balans/internal/pricing"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type recordedApprovals struct{ subjects []approval.Subject }

func (a *recordedApprovals) Request(_ context.Context, s approval.Subject) (approval.Request, error) {
	a.subjects = append(a.subjects, s)
	return approval.Request{}, nil
}

func (a *recordedApprovals) Cancel(context.Context, approval.SubjectType, uuid.UUID) error {
	return nil
}

type roleNames struct{}

func (roleNames) RoleName(_ context.Context, code string) (string, bool, error) {
	return strings.ToUpper(code), code != "unknown", nil
}

type moduleHarness struct {
	t         *testing.T
	pool      *pgxpool.Pool
	m         *pricing.Module
	router    chi.Router
	approvals *recordedApprovals
	actor     actor.Actor
}

func setupModule(t *testing.T, elig pricing.Eligibility) *moduleHarness {
	t.Helper()
	pool := dbtest.Pool(t)
	ctx := context.Background()
	// Data Harga Dasar & daftar karyawan dipakai bersama; bersihkan versinya supaya test bisa diulang.
	for _, q := range []string{
		`DELETE FROM price_list_items WHERE price_list_version_id IN (SELECT v.id FROM price_list_versions v
			JOIN price_lists l ON l.id = v.price_list_id WHERE l.price_list_type IN ('base', 'employee'))`,
		`DELETE FROM price_list_versions WHERE price_list_id IN (SELECT id FROM price_lists WHERE price_list_type IN ('base', 'employee'))`,
		`UPDATE price_lists SET deleted_at = now(), deleted_by = created_by WHERE price_list_type = 'employee' AND deleted_at IS NULL`,
		`DELETE FROM discount_limits WHERE role_code = 'head_sales'`,
	} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err)
	}
	fixed := clock.NewFixed(time.Date(2026, 10, 23, 2, 0, 0, 0, time.UTC))
	h := &moduleHarness{t: t, pool: pool, approvals: &recordedApprovals{}}
	rs := httpx.NewResponder(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	h.m = pricing.New(pricing.Deps{
		Pool: pool, Tx: db.NewTxManager(pool), Clock: fixed, Responder: rs, Audit: audit.New(audit.Deps{Pool: pool}),
		Roles: roleNames{}, Approvals: h.approvals, Eligibility: elig,
	})
	perms := map[string]struct{}{}
	for _, res := range []string{"price_list", "promotion", "special_price", "discount_limit"} {
		for _, a := range []string{"read", "write", "submit", "approve"} {
			perms["pricing."+res+"."+a] = struct{}{}
		}
	}
	h.actor = actor.Actor{UserID: uuid.New(), Roles: []string{"head_sales"}, Permissions: perms}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(actor.With(req.Context(), h.actor)))
		})
	})
	h.m.RegisterRoutes(r)
	h.router = r
	assert.Equal(t, pricing.Name, h.m.Name())
	assert.Nil(t, h.m.PeriodicJobs())
	h.m.RegisterWorkers(nil)
	return h
}

func (h *moduleHarness) do(method, path string, body any, out any) int {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(h.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	if out != nil && w.Code < 300 {
		require.NoError(h.t, json.Unmarshal(w.Body.Bytes(), out), w.Body.String())
	}
	if w.Code >= 300 {
		h.t.Logf("%s %s → %d %s", method, path, w.Code, w.Body.String())
	}
	return w.Code
}

type idVersion struct {
	ID      uuid.UUID `json:"id"`
	Version int       `json:"version"`
}

// publish membuat versi lewat API, mengajukannya, dan menyetujuinya lewat handler approval terdaftar.
func (h *moduleHarness) publish(listID uuid.UUID, body map[string]any) uuid.UUID {
	h.t.Helper()
	var v idVersion
	require.Equal(h.t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/price-lists/"+listID.String()+"/versions", body, &v))
	require.Equal(h.t, http.StatusOK, h.do(http.MethodPost, "/api/v1/price-list-versions/"+v.ID.String()+"/submit", map[string]any{"version": v.Version}, &v))
	handler := h.m.ApprovalHandler()
	err := db.NewTxManager(h.pool).WithTx(actor.With(context.Background(), h.actor), func(ctx context.Context) error {
		return handler.OnApproved(ctx, approval.Decided{Subject: approval.Subject{Type: approval.SubjectPriceListVersion, ID: v.ID}, DecidedBy: uuid.New()})
	})
	require.NoError(h.t, err)
	return v.ID
}

type employeeOnly struct{ customer uuid.UUID }

func (e employeeOnly) EmployeePriceEligible(_ context.Context, id uuid.UUID, _ clock.BusinessDate) (bool, error) {
	return id == e.customer, nil
}

func (employeeOnly) PartnerAgreementPriceList(context.Context, uuid.UUID, clock.BusinessDate) (*uuid.UUID, error) {
	return nil, nil
}

func TestModuleEndToEnd(t *testing.T) {
	employee := uuid.New()
	h := setupModule(t, employeeOnly{customer: employee})
	channel, product := uuid.New(), uuid.New()
	date := "2026-10-23"

	var bases []idVersion
	require.Equal(t, http.StatusOK, h.do(http.MethodGet, "/api/v1/price-lists?priceListType=base", nil, &bases))
	require.Len(t, bases, 1)
	baseVersion := h.publish(bases[0].ID, map[string]any{
		"validFrom": date, "items": []map[string]any{{"productId": product, "priceAmount": "50000", "maxRetailPriceAmount": "55000"}},
	})

	var web idVersion
	require.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/price-lists", map[string]any{
		"code": "WEB-" + uuid.NewString()[:8], "name": "Website", "priceListType": "channel", "channelId": channel, "isActive": true,
	}, &web))
	webVersion := h.publish(web.ID, map[string]any{"validFrom": date, "items": []map[string]any{{"productId": product, "priceAmount": "48000"}}})

	var emp idVersion
	require.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/price-lists", map[string]any{
		"code": "KRY-" + uuid.NewString()[:8], "name": "Karyawan", "priceListType": "employee", "basis": "percent",
		"referencePriceListId": web.ID, "isActive": true,
	}, &emp))
	h.publish(emp.ID, map[string]any{"validFrom": date, "percent": "90", "items": []any{}})
	require.Len(t, h.approvals.subjects, 3)

	ctx := context.Background()
	on := clock.NewBusinessDate(2026, 10, 23)
	q, err := h.m.Resolve(ctx, pricing.PriceRequest{ProductID: product, Qty: decimal.NewFromInt(1), Date: on})
	require.NoError(t, err)
	assert.Equal(t, pricing.RuleBasePrice, q.Rule)
	assert.Equal(t, baseVersion, *q.PriceListVersionID)
	assert.Equal(t, "55000.00", q.MaxRetailPrice.String())

	q, err = h.m.Resolve(ctx, pricing.PriceRequest{ProductID: product, Qty: decimal.NewFromInt(1), ChannelID: &channel, Date: on})
	require.NoError(t, err)
	assert.Equal(t, pricing.RulePriceList, q.Rule)
	assert.Equal(t, "48000.00", q.UnitPrice.String())
	assert.Equal(t, webVersion, q.SourceID)

	q, err = h.m.Resolve(ctx, pricing.PriceRequest{ProductID: product, Qty: decimal.NewFromInt(1), ChannelID: &channel, CustomerID: &employee, Date: on})
	require.NoError(t, err)
	assert.Equal(t, "43200.00", q.UnitPrice.String(), "harga karyawan 90% dari harga website")
	assert.Equal(t, emp.ID, *q.PriceListID)

	_, err = h.m.Resolve(ctx, pricing.PriceRequest{ProductID: product, Qty: decimal.NewFromInt(1), Date: on.AddDays(-1)})
	assert.True(t, apperr.HasCode(err, apperr.CodeNotFound), "sebelum versi berlaku belum ada harga")

	// Harga bertingkat, promo, dan harga khusus lewat API Aturan Harga.
	require.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/tiered-prices", map[string]any{
		"priceListId": web.ID, "productId": product, "minQty": "12", "priceAmount": "46000", "validFrom": date,
	}, nil))
	var quote struct {
		UnitPrice string `json:"unitPrice"`
		Rule      string `json:"rule"`
	}
	require.Equal(t, http.StatusOK, h.do(http.MethodPost, "/api/v1/pricing/quote", map[string]any{
		"productId": product, "qty": "12", "channelId": channel, "date": date,
	}, &quote))
	assert.Equal(t, "tiered_price", quote.Rule)
	assert.Equal(t, "46000.00", quote.UnitPrice)

	require.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/promotions", map[string]any{
		"code": "PROMO-" + uuid.NewString()[:8], "name": "Promo", "channelId": channel, "validFrom": date, "validTo": "2026-10-30",
		"isActive": true, "items": []map[string]any{{"productId": product, "discountPercent": "10"}},
	}, nil))
	require.Equal(t, http.StatusOK, h.do(http.MethodPost, "/api/v1/pricing/quote", map[string]any{
		"productId": product, "qty": "12", "channelId": channel, "date": date,
	}, &quote))
	assert.Equal(t, "promotion", quote.Rule)
	assert.Equal(t, "41400.00", quote.UnitPrice, "promo 10% dari harga bertingkat 46.000")

	customer := uuid.New()
	require.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/special-prices", map[string]any{
		"partyType": "customer", "partyId": customer, "productId": product, "priceAmount": "39000", "validFrom": date,
	}, nil))
	q, err = h.m.Resolve(ctx, pricing.PriceRequest{ProductID: product, Qty: decimal.NewFromInt(1), ChannelID: &channel, CustomerID: &customer, Date: on})
	require.NoError(t, err)
	assert.Equal(t, pricing.RuleSpecialPrice, q.Rule)

	for _, path := range []string{
		"/api/v1/price-lists", "/api/v1/price-lists/options", "/api/v1/price-lists/" + web.ID.String(),
		"/api/v1/price-list-versions/" + webVersion.String(), "/api/v1/special-prices?includeEnded=true", "/api/v1/promotions",
		"/api/v1/tiered-prices?productId=" + product.String(), "/api/v1/discount-limits", "/api/v1/discount-limits/roles",
	} {
		assert.Equal(t, http.StatusOK, h.do(http.MethodGet, path, nil, nil), path)
	}
}

func TestModuleCheckDiscount(t *testing.T) {
	h := setupModule(t, nil)
	require.Equal(t, http.StatusCreated, h.do(http.MethodPost, "/api/v1/discount-limits", map[string]any{
		"roleCode": "head_sales", "maxLineDiscountPercent": "10", "maxDocumentDiscountPercent": "5", "validFrom": "2026-10-23",
	}, nil))
	ctx := context.Background()
	limit, err := h.m.EffectiveDiscountLimit(ctx, []string{"head_sales"})
	require.NoError(t, err)
	assert.True(t, limit.MaxLinePercent.Equal(decimal.NewFromInt(10)))

	gross := money.FromInt(100000)
	check, err := h.m.CheckDiscount(ctx, []string{"head_sales"},
		[]pricing.LineDiscount{{Gross: gross, Discount: pricing.Discount{Kind: pricing.DiscountPercent, Value: decimal.NewFromInt(10)}}},
		pricing.DocumentDiscount{Subtotal: money.FromInt(90000), Discount: pricing.Discount{Kind: pricing.DiscountAmount, Value: decimal.NewFromInt(4500)}})
	require.NoError(t, err)
	assert.False(t, check.RequiresApproval, "tepat di batas baris dan dokumen")

	check, err = h.m.CheckDiscount(ctx, []string{"sales_mt"},
		[]pricing.LineDiscount{{Gross: gross, Discount: pricing.Discount{Kind: pricing.DiscountAmount, Value: decimal.NewFromInt(1000)}}},
		pricing.DocumentDiscount{Subtotal: money.FromInt(99000)})
	require.NoError(t, err)
	assert.True(t, check.RequiresApproval, "role tanpa batas → setiap diskon butuh approval")
	assert.Equal(t, approval.SubjectSalesOrderDiscount, pricing.ApprovalSubjectDiscount)

	var res struct {
		RequiresApproval bool   `json:"requiresApproval"`
		SubtotalAmount   string `json:"subtotalAmount"`
	}
	require.Equal(t, http.StatusOK, h.do(http.MethodPost, "/api/v1/pricing/discount-check", map[string]any{
		"lines":            []map[string]any{{"grossAmount": "100000", "discount": map[string]any{"kind": "percent", "value": "12"}}},
		"documentDiscount": map[string]any{"kind": "amount", "value": "1000"},
	}, &res))
	assert.True(t, res.RequiresApproval)
	assert.Equal(t, "88000.00", res.SubtotalAmount)
	assert.Equal(t, http.StatusUnprocessableEntity, h.do(http.MethodPost, "/api/v1/pricing/discount-check", map[string]any{
		"lines": []map[string]any{{"grossAmount": "abc"}},
	}, nil))

	entities := pricing.AuditEntities()
	assert.Equal(t, pricing.PermPriceListRead, entities["price_list_version"])
}
