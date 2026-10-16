package identity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/identity"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/httpx"
	"balans/internal/platform/pii"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type fakeMailer struct {
	mu   sync.Mutex
	sent map[string]string
}

func (f *fakeMailer) QueueEmail(_ context.Context, to, _, text string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent[to] = text
	return uuid.New(), nil
}

var tokenPattern = regexp.MustCompile(`token=([A-Za-z0-9_-]+)`)

func (f *fakeMailer) token(t *testing.T, to string) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	m := tokenPattern.FindStringSubmatch(f.sent[to])
	require.Len(t, m, 2, "email ke %s memuat tautan", to)
	return m[1]
}

type harness struct {
	t      *testing.T
	srv    *httptest.Server
	clock  *clock.Fixed
	mailer *fakeMailer
}

type response struct {
	status  int
	body    map[string]any
	list    []any
	cookies []*http.Cookie
}

func (r response) code() string {
	c, _ := r.body["code"].(string)
	return c
}

func (h *harness) do(method, path, token string, body any, cookie *http.Cookie) response {
	h.t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(h.t, err)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, h.srv.URL+path, buf)
	require.NoError(h.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(h.t, err)
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	require.NoError(h.t, err)
	out := response{status: res.StatusCode, cookies: res.Cookies()}
	if len(raw) > 0 {
		if raw[0] == '[' {
			require.NoError(h.t, json.Unmarshal(raw, &out.list))
		} else {
			require.NoError(h.t, json.Unmarshal(raw, &out.body))
		}
	}
	return out
}

func refreshCookie(r response) *http.Cookie {
	for _, c := range r.cookies {
		if c.Name == "balans_refresh" {
			return c
		}
	}
	return nil
}

func (h *harness) login(email, password, code string) response {
	h.t.Helper()
	body := map[string]any{"email": email, "password": password}
	if code != "" {
		body["totpCode"] = code
	}
	return h.do(http.MethodPost, "/api/v1/auth/login", "", body, nil)
}

func newHarness(t *testing.T) *harness {
	pool := dbtest.Pool(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	rs := httpx.NewResponder(log, nil)
	c := clock.NewFixed(time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC))
	cipher, err := pii.New("k1", map[string][]byte{"k1": bytes.Repeat([]byte{1}, 32)}, bytes.Repeat([]byte{2}, 32))
	require.NoError(t, err)
	mailer := &fakeMailer{sent: map[string]string{}}
	auditMod := audit.New(audit.Deps{Pool: pool, Responder: rs})
	mod, err := identity.New(identity.Deps{
		Pool: pool, Tx: db.NewTxManager(pool), Clock: c, Log: log, Responder: rs, Audit: auditMod,
		Mailer: mailer, Cipher: cipher, BaseURL: "https://erp.test",
	})
	require.NoError(t, err)
	require.NoError(t, mod.Start(context.Background(), "admin@balans.id"))
	require.NoError(t, mod.Start(context.Background(), "lain@balans.id"), "bootstrap hanya sekali")

	r := chi.NewRouter()
	r.Use(mod.Middleware())
	mod.RegisterRoutes(r)
	auditMod.RegisterRoutes(r)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &harness{t: t, srv: srv, clock: c, mailer: mailer}
}

func TestIdentityEndToEnd(t *testing.T) {
	h := newHarness(t)
	const adminPass = "Kopi-Pagi-2026"

	// Undangan admin pertama → atur password.
	_, sentToOther := h.mailer.sent["lain@balans.id"]
	assert.False(t, sentToOther)
	invite := h.mailer.token(t, "admin@balans.id")
	info := h.do(http.MethodGet, "/api/v1/auth/password/token?token="+invite, "", nil, nil)
	require.Equal(t, http.StatusOK, info.status)
	assert.Equal(t, "invite", info.body["purpose"])

	weak := h.do(http.MethodPost, "/api/v1/auth/password/setup", "", map[string]any{"token": invite, "password": "1234567890"}, nil)
	assert.Equal(t, "VALIDATION_FAILED", weak.code())
	ok := h.do(http.MethodPost, "/api/v1/auth/password/setup", "", map[string]any{"token": invite, "password": adminPass}, nil)
	require.Equal(t, http.StatusNoContent, ok.status)
	reused := h.do(http.MethodPost, "/api/v1/auth/password/setup", "", map[string]any{"token": invite, "password": adminPass}, nil)
	assert.Equal(t, "AUTH_PASSWORD_TOKEN_INVALID", reused.code())

	// Password salah selalu pesan yang sama, termasuk email tidak terdaftar.
	assert.Equal(t, "AUTH_INVALID_CREDENTIALS", h.login("admin@balans.id", "salah-password", "").code())
	assert.Equal(t, "AUTH_INVALID_CREDENTIALS", h.login("tidakada@balans.id", "salah-password", "").code())

	// Admin wajib MFA: login berhasil tetapi menu lain terkunci sampai MFA aktif.
	login := h.login("ADMIN@balans.id ", adminPass, "")
	require.Equal(t, http.StatusOK, login.status, login.body)
	assert.NotNil(t, refreshCookie(login))
	token := login.body["accessToken"].(string)
	me := h.do(http.MethodGet, "/api/v1/me", token, nil, nil)
	require.Equal(t, http.StatusOK, me.status)
	assert.Equal(t, true, me.body["mfaEnrollmentRequired"])
	assert.Equal(t, "AUTH_MFA_ENROLLMENT_REQUIRED", h.do(http.MethodGet, "/api/v1/users", token, nil, nil).code())

	setup := h.do(http.MethodPost, "/api/v1/auth/mfa/setup", token, nil, nil)
	require.Equal(t, http.StatusOK, setup.status, setup.body)
	secret := setup.body["secret"].(string)
	assert.True(t, strings.HasPrefix(setup.body["qrCodeDataUrl"].(string), "data:image/png;base64,"))
	assert.Equal(t, "AUTH_MFA_INVALID", h.do(http.MethodPost, "/api/v1/auth/mfa/activate", token, map[string]any{"code": "000000"}, nil).code())
	code, err := totp.GenerateCode(secret, h.clock.Now())
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, h.do(http.MethodPost, "/api/v1/auth/mfa/activate", token, map[string]any{"code": code}, nil).status)
	assert.Equal(t, http.StatusOK, h.do(http.MethodGet, "/api/v1/users", token, nil, nil).status)

	// Login berikutnya meminta kode TOTP.
	assert.Equal(t, "AUTH_MFA_REQUIRED", h.login("admin@balans.id", adminPass, "").code())
	h.clock.Advance(time.Minute)
	code, err = totp.GenerateCode(secret, h.clock.Now())
	require.NoError(t, err)
	login = h.login("admin@balans.id", adminPass, code)
	require.Equal(t, http.StatusOK, login.status)
	token = login.body["accessToken"].(string)

	// Memberi role butuh re-autentikasi.
	newUser := map[string]any{"email": "gudang@balans.id", "fullName": "Staf Gudang", "roles": []map[string]any{{"code": "warehouse_staff"}}}
	assert.Equal(t, "AUTH_REAUTH_REQUIRED", h.do(http.MethodPost, "/api/v1/users", token, newUser, nil).code())
	reauth := h.do(http.MethodPost, "/api/v1/auth/reauth", token, map[string]any{"password": adminPass}, nil)
	require.Equal(t, http.StatusOK, reauth.status)
	stepUp := reauth.body["accessToken"].(string)
	created := h.do(http.MethodPost, "/api/v1/users", stepUp, newUser, nil)
	require.Equal(t, http.StatusCreated, created.status, created.body)
	staffID := created.body["id"].(string)
	assert.Equal(t, false, created.body["hasPassword"])
	dup := h.do(http.MethodPost, "/api/v1/users", stepUp, newUser, nil)
	assert.Equal(t, "VALIDATION_FAILED", dup.code())

	// Reauth kedaluwarsa setelah 5 menit.
	h.clock.Advance(6 * time.Minute)
	assert.Equal(t, "AUTH_REAUTH_REQUIRED", h.do(http.MethodPost, "/api/v1/users", stepUp,
		map[string]any{"email": "x@balans.id", "fullName": "X", "roles": []map[string]any{{"code": "finance"}}}, nil).code())

	// Staf gudang hanya punya hak sesuai matriks.
	const staffPass = "Rak-Rapi-Selalu-2026"
	staffInvite := h.mailer.token(t, "gudang@balans.id")
	require.Equal(t, http.StatusNoContent, h.do(http.MethodPost, "/api/v1/auth/password/setup", "", map[string]any{"token": staffInvite, "password": staffPass}, nil).status)
	staffLogin := h.login("gudang@balans.id", staffPass, "")
	require.Equal(t, http.StatusOK, staffLogin.status)
	staffToken := staffLogin.body["accessToken"].(string)
	staffMe := h.do(http.MethodGet, "/api/v1/me", staffToken, nil, nil)
	assert.Contains(t, staffMe.body["permissions"], "warehouse.inbound.post")
	assert.NotContains(t, staffMe.body["permissions"], "warehouse.outbound_qc.post")
	assert.Equal(t, "FORBIDDEN", h.do(http.MethodGet, "/api/v1/users", staffToken, nil, nil).code())
	assert.Equal(t, "AUTH_SESSION_EXPIRED", h.do(http.MethodGet, "/api/v1/me", "", nil, nil).code())

	// Rotasi refresh token dan deteksi pemakaian ulang.
	first := refreshCookie(staffLogin)
	rotated := h.do(http.MethodPost, "/api/v1/auth/refresh", "", nil, first)
	require.Equal(t, http.StatusOK, rotated.status)
	second := refreshCookie(rotated)
	require.NotNil(t, second)
	assert.NotEqual(t, first.Value, second.Value)
	graceful := h.do(http.MethodPost, "/api/v1/auth/refresh", "", nil, first)
	assert.Equal(t, http.StatusOK, graceful.status, "refresh bersamaan dari tab lain dalam masa tenggang")
	assert.Nil(t, refreshCookie(graceful))
	h.clock.Advance(11 * time.Second)
	assert.Equal(t, "AUTH_SESSION_EXPIRED", h.do(http.MethodPost, "/api/v1/auth/refresh", "", nil, first).code())
	assert.Equal(t, "AUTH_SESSION_EXPIRED", h.do(http.MethodPost, "/api/v1/auth/refresh", "", nil, second).code(),
		"pemakaian ulang mencabut seluruh keluarga sesi")

	// Kunci akun setelah 5 kali gagal dalam 15 menit.
	for range 5 {
		assert.Equal(t, "AUTH_INVALID_CREDENTIALS", h.login("gudang@balans.id", "password-salah", "").code())
	}
	locked := h.login("gudang@balans.id", staffPass, "")
	assert.Equal(t, http.StatusLocked, locked.status)
	assert.Equal(t, "AUTH_ACCOUNT_LOCKED", locked.code())
	h.clock.Advance(16 * time.Minute)
	staffLogin = h.login("gudang@balans.id", staffPass, "")
	require.Equal(t, http.StatusOK, staffLogin.status)
	staffToken = staffLogin.body["accessToken"].(string)

	// Nonaktif memutus akses seketika.
	code, err = totp.GenerateCode(secret, h.clock.Now())
	require.NoError(t, err)
	login = h.login("admin@balans.id", adminPass, code)
	token = login.body["accessToken"].(string)
	staff := h.do(http.MethodGet, "/api/v1/users/"+staffID, token, nil, nil)
	require.Equal(t, http.StatusOK, staff.status)
	deact := h.do(http.MethodPost, "/api/v1/users/"+staffID+"/deactivate", token, map[string]any{"version": staff.body["version"]}, nil)
	require.Equal(t, http.StatusOK, deact.status, deact.body)
	assert.Equal(t, "AUTH_SESSION_EXPIRED", h.do(http.MethodGet, "/api/v1/me", staffToken, nil, nil).code())
	stale := h.do(http.MethodPost, "/api/v1/users/"+staffID+"/activate", token, map[string]any{"version": staff.body["version"]}, nil)
	assert.Equal(t, "VERSION_CONFLICT", stale.code())

	// Audit trail mencatat pembuatan, login, dan status pengguna; field sensitif tidak tampil.
	logs := h.do(http.MethodGet, "/api/v1/audit-logs?entityType=user&entityId="+staffID, token, nil, nil)
	require.Equal(t, http.StatusOK, logs.status, logs.body)
	actions := map[string]bool{}
	for _, it := range logs.body["items"].([]any) {
		actions[it.(map[string]any)["action"].(string)] = true
	}
	assert.True(t, actions["create"])
	assert.True(t, actions["login"])
	assert.True(t, actions["login_failed"])
	assert.True(t, actions["status_change"])

	// Role & permission: Admin tidak bisa mengunci dirinya sendiri dari pengaturan role.
	roles := h.do(http.MethodGet, "/api/v1/roles", token, nil, nil)
	require.Equal(t, http.StatusOK, roles.status)
	var adminRole map[string]any
	for _, r := range roles.list {
		if r.(map[string]any)["code"] == "system_admin" {
			adminRole = r.(map[string]any)
		}
	}
	require.NotNil(t, adminRole)
	reauth = h.do(http.MethodPost, "/api/v1/auth/reauth", token, map[string]any{"totpCode": code}, nil)
	require.Equal(t, http.StatusOK, reauth.status)
	stepUp = reauth.body["accessToken"].(string)
	lockout := h.do(http.MethodPut, "/api/v1/roles/"+adminRole["id"].(string)+"/permissions", stepUp,
		map[string]any{"permissions": []string{"identity.user.read"}, "version": adminRole["version"]}, nil)
	assert.Equal(t, "VALIDATION_FAILED", lockout.code())

	// Logout mencabut sesi.
	out := h.do(http.MethodPost, "/api/v1/auth/logout", "", nil, refreshCookie(login))
	assert.Equal(t, http.StatusNoContent, out.status)
	assert.Equal(t, "AUTH_SESSION_EXPIRED", h.do(http.MethodGet, "/api/v1/me", token, nil, nil).code())
}
