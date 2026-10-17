package service_test

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/audit"
	"balans/internal/party/domain"
	"balans/internal/party/postgres"
	"balans/internal/party/service"
	"balans/internal/platform/actor"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/db/dbtest"
	"balans/internal/platform/paging"
	"balans/internal/platform/pii"
)

func TestMain(m *testing.M) {
	os.Exit(dbtest.Main(m))
}

type users struct{ known map[uuid.UUID]service.UserRef }

func (u users) UsersByID(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]service.UserRef, error) {
	out := map[uuid.UUID]service.UserRef{}
	for _, id := range ids {
		if r, ok := u.known[id]; ok {
			out[id] = r
		}
	}
	return out, nil
}

func (u users) SearchUsers(_ context.Context, q, role string, _ int) ([]service.UserRef, error) {
	var out []service.UserRef
	for _, r := range u.known {
		if strings.Contains(strings.ToLower(r.Name), strings.ToLower(q)) && (role == "" || (len(r.Roles) > 0 && r.Roles[0] == role)) {
			out = append(out, r)
		}
	}
	return out, nil
}

// employees adalah pengganti modul hris: karyawan aktif sampai tanggal resign.
type employees struct {
	mu     sync.Mutex
	resign map[uuid.UUID]*clock.BusinessDate
}

func (e *employees) add(resign *clock.BusinessDate) uuid.UUID {
	e.mu.Lock()
	defer e.mu.Unlock()
	id := uuid.New()
	e.resign[id] = resign
	return id
}

func (e *employees) active(id uuid.UUID, on clock.BusinessDate) (bool, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, ok := e.resign[id]
	return ok, ok && (r == nil || on.Before(*r))
}

func (e *employees) Employees(_ context.Context, ids []uuid.UUID, on clock.BusinessDate) (map[uuid.UUID]service.EmployeeRef, error) {
	out := map[uuid.UUID]service.EmployeeRef{}
	for _, id := range ids {
		if ok, active := e.active(id, on); ok {
			out[id] = service.EmployeeRef{ID: id, Number: "BN-" + id.String()[:4], Name: "Karyawan", IsActive: active}
		}
	}
	return out, nil
}

func (e *employees) IsActiveEmployee(_ context.Context, id uuid.UUID, on clock.BusinessDate) (bool, error) {
	_, active := e.active(id, on)
	return active, nil
}

type files struct {
	mu       sync.Mutex
	attached map[uuid.UUID]uuid.UUID
}

func (f *files) Attach(_ context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ownerType != service.FileOwnerAgreement {
		return apperr.Validation(map[string]string{"fileId": "jenis salah"})
	}
	if cur, ok := f.attached[fileID]; ok && cur != ownerID {
		return apperr.Validation(map[string]string{"fileId": "File sudah dipakai data lain"})
	}
	f.attached[fileID] = ownerID
	return nil
}

func (f *files) Names(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	for _, id := range ids {
		out[id] = "PKS.pdf"
	}
	return out, nil
}

type harness struct {
	t         *testing.T
	pool      *pgxpool.Pool
	svc       *service.Service
	clock     *clock.Fixed
	employees *employees
	files     *files
	sales     service.UserRef
	merch     service.UserRef
	actorID   uuid.UUID
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.Pool(t)
	key := bytes.Repeat([]byte{7}, 32)
	cipher, err := pii.New("k1", map[string][]byte{"k1": key}, bytes.Repeat([]byte{9}, 32))
	require.NoError(t, err)
	// Senin 19 Okt 2026 09.00 WIB.
	fixed := clock.NewFixed(time.Date(2026, 10, 19, 2, 0, 0, 0, time.UTC))
	sales := service.UserRef{ID: uuid.New(), Name: "Sinta Sales", Email: "sinta@balans.test", Roles: []string{"sales_mt"}}
	merch := service.UserRef{ID: uuid.New(), Name: "Maman Merchandiser", Email: "maman@balans.test", Roles: []string{"merchandiser"}}
	h := &harness{
		t: t, pool: pool, clock: fixed, employees: &employees{resign: map[uuid.UUID]*clock.BusinessDate{}},
		files: &files{attached: map[uuid.UUID]uuid.UUID{}}, sales: sales, merch: merch, actorID: uuid.New(),
	}
	h.svc = service.New(service.Deps{
		Store: postgres.NewStore(pool, cipher), Tx: db.NewTxManager(pool), Audit: audit.New(audit.Deps{Pool: pool}),
		Clock: fixed, Users: users{known: map[uuid.UUID]service.UserRef{sales.ID: sales, merch.ID: merch}},
		Files: h.files, Employees: h.employees,
	})
	return h
}

var allPerms = []string{
	service.PermCustomerRead, service.PermCustomerWrite, service.PermPartnerRead, service.PermPartnerWrite,
	service.PermAgreementRead, service.PermAgreementWrite, service.PermProductCodeRead, service.PermProductCodeWrite,
	service.PermSupplierRead, service.PermSupplierWrite,
}

func (h *harness) ctx(perms ...string) context.Context {
	if perms == nil {
		perms = allPerms
	}
	set := map[string]struct{}{}
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return actor.With(context.Background(), actor.Actor{UserID: h.actorID, FullName: "Penguji", Permissions: set})
}

func code(err error) apperr.Code { return apperr.As(err).Code }

func ptr[T any](v T) *T { return &v }

// phone membuat nomor HP unik agar test tidak bertabrakan dengan data test lain di database bersama.
func phone() string { return fmt.Sprintf("0812%08d", rand.IntN(100_000_000)) }

func uniqueCode(prefix string) string { return strings.ToUpper(prefix + "-" + uuid.NewString()[:8]) }

func (h *harness) auditActions(entityID uuid.UUID) []string {
	h.t.Helper()
	rows, err := h.pool.Query(context.Background(), `SELECT action FROM audit_logs WHERE entity_id = $1 ORDER BY created_at, id`, entityID)
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

func TestCustomerPhoneNormalizedAndUnique(t *testing.T) {
	h := setup(t)
	raw := phone()
	c, err := h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{
		FullName: "Siti Aminah", Phone: raw, Type: domain.CustomerRetail, MarketingConsent: true, MarketingConsentVersion: "v1",
		Addresses: []domain.AddressInput{
			{Label: "Rumah", RecipientName: "Siti", AddressLine: "Jl. Mawar 1", City: "Bandung"},
			{Label: "Kantor", RecipientName: "Siti", AddressLine: "Jl. Asia Afrika 10", City: "Bandung", IsDefault: true},
		},
	})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(c.Code, "PLG-"))
	assert.Equal(t, "+62"+raw[1:], c.Phone)
	require.Len(t, c.Addresses, 2)
	assert.True(t, c.Addresses[1].IsDefault)
	require.NotNil(t, c.MarketingConsentAt)
	assert.Equal(t, h.clock.Now(), c.MarketingConsentAt.UTC())
	assert.Equal(t, []string{"create"}, h.auditActions(c.ID))

	// Nomor yang sama dalam format lain ditolak dengan menyebut pelanggan pemiliknya.
	_, err = h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{FullName: "Siti Lain", Phone: "+62 " + raw[1:], Type: domain.CustomerRetail})
	assert.Contains(t, apperr.As(err).Fields["phone"], c.Code)

	// Pencarian dengan nomor format apa pun dan nama.
	for _, q := range []string{"62" + raw[1:], raw[len(raw)-6:], "siti amin"} {
		res, err := h.svc.ListCustomers(h.ctx(), service.CustomerQuery{Search: q}, paging.New(nil, nil))
		require.NoError(t, err)
		assert.True(t, containsCustomer(res.Items, c.ID), q)
	}
	res, err := h.svc.ListCustomers(h.ctx(), service.CustomerQuery{Phone: "+62" + raw[1:]}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Len(t, res.Items[0].Addresses, 2)
	found, ok, err := h.svc.CustomerByPhone(context.Background(), "62"+raw[1:])
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, c.ID, found.ID)

	// Hapus pelanggan: nomor HP boleh dipakai pelanggan baru, data lama tidak tampil di list.
	require.NoError(t, h.svc.DeleteCustomer(h.ctx(), c.ID, c.Version))
	_, err = h.svc.GetCustomer(h.ctx(), c.ID)
	assert.Equal(t, apperr.CodeNotFound, code(err))
	again, err := h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{FullName: "Siti Baru", Phone: raw, Type: domain.CustomerRetail})
	require.NoError(t, err)
	assert.NotEqual(t, c.Code, again.Code)
	assert.Equal(t, []string{"create", "delete"}, h.auditActions(c.ID))
}

func containsCustomer(list []domain.Customer, id uuid.UUID) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestCustomerUpdateAddressesAndVersion(t *testing.T) {
	h := setup(t)
	c, err := h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{
		FullName: "Budi", Phone: phone(), Type: domain.CustomerB2B,
		Addresses: []domain.AddressInput{
			{RecipientName: "Budi", AddressLine: "Jl. A", City: "Jakarta"},
			{RecipientName: "Budi", AddressLine: "Jl. B", City: "Depok"},
		},
	})
	require.NoError(t, err)
	keep := c.Addresses[1]
	in := domain.CustomerInput{
		FullName: "Budi Santoso", Phone: c.Phone, Type: domain.CustomerB2B,
		Addresses: []domain.AddressInput{
			{ID: keep.ID, RecipientName: "Budi", AddressLine: "Jl. B No. 2", City: "Depok", IsDefault: true},
			{RecipientName: "Gudang", AddressLine: "Jl. C", City: "Bekasi", Latitude: ptr("-6.2383"), Longitude: ptr("106.9756")},
		},
	}
	updated, err := h.svc.UpdateCustomer(h.ctx(), c.ID, c.Version, in)
	require.NoError(t, err)
	assert.Equal(t, c.Version+1, updated.Version)

	got, err := h.svc.GetCustomer(h.ctx(), c.ID)
	require.NoError(t, err)
	assert.Equal(t, "Budi Santoso", got.FullName)
	require.Len(t, got.Addresses, 2)
	assert.Equal(t, keep.ID, got.Addresses[0].ID, "alamat utama di urutan pertama")
	assert.True(t, got.Addresses[0].IsDefault)
	assert.Equal(t, "Jl. B No. 2", got.Addresses[0].AddressLine)
	assert.Equal(t, "-6.238300", got.Addresses[1].Latitude.StringFixed(6))

	_, err = h.svc.UpdateCustomer(h.ctx(), c.ID, c.Version, in)
	assert.Equal(t, apperr.CodeVersionConflict, code(err))
	assert.Equal(t, apperr.CodeVersionConflict, code(h.svc.DeleteCustomer(h.ctx(), c.ID, c.Version)))
	_, err = h.svc.UpdateCustomer(h.ctx(service.PermCustomerRead), c.ID, got.Version, in)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.ListCustomers(context.Background(), service.CustomerQuery{}, paging.New(nil, nil))
	assert.Equal(t, apperr.CodeAuthSessionExpired, code(err))
}

func TestCustomerEmployeeLinkAndResign(t *testing.T) {
	h := setup(t)
	resignOn := clock.NewBusinessDate(2026, 11, 1)
	emp := h.employees.add(&resignOn)
	in := domain.CustomerInput{FullName: "Rudi Karyawan", Phone: phone(), Type: domain.CustomerEmployee, EmployeeID: &emp}
	c, err := h.svc.CreateCustomer(h.ctx(), in)
	require.NoError(t, err)

	ok, err := h.svc.EmployeePriceEligible(context.Background(), c.ID, clock.NewBusinessDate(2026, 10, 31))
	require.NoError(t, err)
	assert.True(t, ok, "harga karyawan berlaku sebelum resign")
	ok, err = h.svc.EmployeePriceEligible(context.Background(), c.ID, resignOn)
	require.NoError(t, err)
	assert.False(t, ok, "resign menghapus harga karyawan otomatis")

	// Satu karyawan hanya tertaut ke satu pelanggan.
	_, err = h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{FullName: "Rudi 2", Phone: phone(), Type: domain.CustomerEmployee, EmployeeID: &emp})
	assert.Contains(t, apperr.As(err).Fields, "employeeId")

	// Karyawan yang sudah resign tidak bisa ditautkan.
	past := clock.NewBusinessDate(2026, 1, 1)
	gone := h.employees.add(&past)
	_, err = h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{FullName: "Mantan", Phone: phone(), Type: domain.CustomerEmployee, EmployeeID: &gone})
	assert.Contains(t, apperr.As(err).Fields["employeeId"], "resign")

	refs, err := h.svc.EmployeeRefs(context.Background(), []uuid.UUID{emp})
	require.NoError(t, err)
	assert.True(t, refs[emp].IsActive)

	retail, err := h.svc.CreateCustomer(h.ctx(), domain.CustomerInput{FullName: "Umum", Phone: phone(), Type: domain.CustomerRetail})
	require.NoError(t, err)
	ok, err = h.svc.EmployeePriceEligible(context.Background(), retail.ID, past)
	require.NoError(t, err)
	assert.False(t, ok)
}

func (h *harness) partner(mutate ...func(*domain.PartnerInput)) domain.Partner {
	h.t.Helper()
	in := domain.PartnerInput{
		Code: uniqueCode("alfa"), Name: "Alfamart", Type: domain.PartnerConsignment, NPWP: ptr("01.234.567.8-901.000"),
		SalesUserID: &h.sales.ID, MerchandiserUserID: &h.merch.ID,
	}
	for _, m := range mutate {
		m(&in)
	}
	p, err := h.svc.CreatePartner(h.ctx(), in)
	require.NoError(h.t, err)
	return p
}

func TestPartnerCodeNPWPAndMerchandiser(t *testing.T) {
	h := setup(t)
	p := h.partner()
	assert.Equal(t, "012345678901000", p.NPWP)

	var enc string
	require.NoError(t, h.pool.QueryRow(context.Background(), `SELECT npwp_enc FROM partners WHERE id = $1`, p.ID).Scan(&enc))
	assert.True(t, strings.HasPrefix(enc, "k1:"), "NPWP terenkripsi di database")
	assert.NotContains(t, enc, "012345678901000")

	_, err := h.svc.CreatePartner(h.ctx(), domain.PartnerInput{Code: strings.ToLower(p.Code), Name: "X", Type: domain.PartnerModernTrade})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))
	require.NoError(t, h.svc.DeletePartner(h.ctx(), p.ID, p.Version))
	_, err = h.svc.CreatePartner(h.ctx(), domain.PartnerInput{Code: p.Code, Name: "X", Type: domain.PartnerModernTrade})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err), "kode mitra tidak dipakai ulang walau dihapus")

	_, err = h.svc.CreatePartner(h.ctx(), domain.PartnerInput{Code: uniqueCode("b"), Name: "B", Type: domain.PartnerB2B, MerchandiserUserID: ptr(uuid.New())})
	assert.Contains(t, apperr.As(err).Fields, "merchandiserUserId")

	q := h.partner()
	in := domain.PartnerInput{Code: q.Code, Name: "Alfamart Midi", Type: domain.PartnerModernTrade, SalesUserID: &h.sales.ID}
	updated, err := h.svc.UpdatePartner(h.ctx(), q.ID, q.Version, in)
	require.NoError(t, err)
	assert.Equal(t, "012345678901000", updated.NPWP, "NPWP tidak dikirim = tetap")
	assert.Nil(t, updated.MerchandiserUserID)
	_, err = h.svc.UpdatePartner(h.ctx(), q.ID, q.Version, in)
	assert.Equal(t, apperr.CodeVersionConflict, code(err))

	mine := h.partner()
	list, err := h.svc.PartnersForMerchandiser(context.Background(), h.merch.ID)
	require.NoError(t, err)
	var codes []string
	for _, p := range list {
		codes = append(codes, p.Code)
	}
	assert.Contains(t, codes, mine.Code)
	assert.NotContains(t, codes, q.Code)

	res, err := h.svc.ListPartners(h.ctx(), service.PartnerQuery{Search: mine.Code}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	opts, err := h.svc.SearchUsers(h.ctx(service.PermPartnerRead), "maman", "merchandiser")
	require.NoError(t, err)
	require.Len(t, opts, 1)
	_, err = h.svc.SearchUsers(h.ctx(service.PermCustomerRead), "", "")
	assert.Equal(t, apperr.CodeForbidden, code(err))
}

func date(y int, m time.Month, d int) clock.BusinessDate { return clock.NewBusinessDate(y, m, d) }

func TestAgreementPeriodsAndConsignmentBlock(t *testing.T) {
	h := setup(t)
	p := h.partner()
	file := uuid.New()
	in := domain.AgreementInput{
		Number: "PKS/2026/01", Type: domain.AgreementConsignment, ValidFrom: date(2026, 1, 1), ValidTo: date(2026, 10, 31),
		FileID: &file, MarginPercent: ptr("20"), PaymentTermDays: 30,
	}
	first, err := h.svc.CreateAgreement(h.ctx(), p.ID, in)
	require.NoError(t, err)
	assert.Equal(t, first.ID, h.files.attached[file], "PDF PKS ditautkan ke PKS")

	// Periode tumpang tindih dengan jenis sama ditolak; jenis lain (MT) boleh.
	overlap := in
	overlap.Number, overlap.FileID, overlap.ValidFrom, overlap.ValidTo = "PKS/2026/02", nil, date(2026, 10, 1), date(2027, 9, 30)
	_, err = h.svc.CreateAgreement(h.ctx(), p.ID, overlap)
	assert.Contains(t, apperr.As(err).Fields, "validFrom")
	mt := overlap
	mt.Type = domain.AgreementModernTrade
	_, err = h.svc.CreateAgreement(h.ctx(), p.ID, mt)
	require.NoError(t, err)

	// PKS file yang sudah dipakai PKS lain ditolak.
	dup := overlap
	dup.ValidFrom, dup.Number, dup.FileID = date(2026, 11, 1), "PKS/2026/03", &file
	_, err = h.svc.CreateAgreement(h.ctx(), p.ID, dup)
	assert.Contains(t, apperr.As(err).Fields, "fileId")

	// Pada 19 Okt 2026 PKS konsinyasi berlaku; DO konsinyasi lolos.
	got, err := h.svc.RequireActiveAgreement(context.Background(), p.ID, domain.AgreementConsignment, h.svc.Today())
	require.NoError(t, err)
	assert.Equal(t, first.ID, got.ID)
	active, err := h.svc.ActiveAgreementsByPartner(h.ctx(), []uuid.UUID{p.ID})
	require.NoError(t, err)
	assert.Len(t, active[p.ID], 2)

	// Setelah 31 Okt PKS kedaluwarsa: DO konsinyasi diblokir.
	_, err = h.svc.RequireActiveAgreement(context.Background(), p.ID, domain.AgreementConsignment, date(2026, 11, 1))
	assert.Contains(t, apperr.As(err).Fields["partnerId"], "tidak berlaku")
	_, ok, err := h.svc.ActiveAgreement(context.Background(), p.ID, domain.AgreementConsignment, date(2026, 11, 1))
	require.NoError(t, err)
	assert.False(t, ok)

	// Perpanjangan dicatat sebagai PKS baru mulai 1 Nov.
	renewal := in
	renewal.Number, renewal.FileID, renewal.ValidFrom, renewal.ValidTo = "PKS/2026/04", nil, date(2026, 11, 1), date(2027, 10, 31)
	renewed, err := h.svc.CreateAgreement(h.ctx(), p.ID, renewal)
	require.NoError(t, err)
	got, err = h.svc.RequireActiveAgreement(context.Background(), p.ID, domain.AgreementConsignment, date(2026, 11, 1))
	require.NoError(t, err)
	assert.Equal(t, renewed.ID, got.ID)

	// Koreksi PKS & nomor PKS unik per mitra.
	fix := renewal
	fix.Number, fix.PaymentTermDays = first.Number, 45
	_, err = h.svc.UpdateAgreement(h.ctx(), renewed.ID, renewed.Version, fix)
	assert.Contains(t, apperr.As(err).Fields, "agreementNumber")
	fix.Number = renewal.Number
	updated, err := h.svc.UpdateAgreement(h.ctx(), renewed.ID, renewed.Version, fix)
	require.NoError(t, err)
	assert.Equal(t, 45, updated.PaymentTermDays)

	list, err := h.svc.ListAgreements(h.ctx(), p.ID)
	require.NoError(t, err)
	assert.Len(t, list, 3)
	require.NoError(t, h.svc.DeleteAgreement(h.ctx(), updated.ID, updated.Version))
	_, err = h.svc.RequireActiveAgreement(context.Background(), p.ID, domain.AgreementConsignment, date(2026, 11, 1))
	require.Error(t, err)
	assert.Equal(t, []string{"create", "update", "delete"}, h.auditActions(updated.ID))

	_, err = h.svc.CreateAgreement(h.ctx(service.PermAgreementRead), p.ID, renewal)
	assert.Equal(t, apperr.CodeForbidden, code(err))
	_, err = h.svc.CreateAgreement(h.ctx(), uuid.New(), renewal)
	assert.Equal(t, apperr.CodeNotFound, code(err))
}

func TestPartnerProductCodes(t *testing.T) {
	h := setup(t)
	p := h.partner()
	product := uuid.New()
	c, err := h.svc.CreateProductCode(h.ctx(), p.ID, product, domain.ProductCodeInput{ArticleCode: "100234", Barcode: "8991234567890"})
	require.NoError(t, err)

	_, err = h.svc.CreateProductCode(h.ctx(), p.ID, product, domain.ProductCodeInput{ArticleCode: "100999"})
	assert.Contains(t, apperr.As(err).Fields, "productId", "satu kode per mitra–produk")
	_, err = h.svc.CreateProductCode(h.ctx(), p.ID, uuid.New(), domain.ProductCodeInput{ArticleCode: "100234"})
	assert.Contains(t, apperr.As(err).Fields, "articleCode", "kode artikel unik per mitra")
	other := h.partner()
	_, err = h.svc.CreateProductCode(h.ctx(), other.ID, product, domain.ProductCodeInput{ArticleCode: "100234"})
	require.NoError(t, err, "mitra lain boleh memakai kode yang sama")
	_, err = h.svc.CreateProductCode(h.ctx(), uuid.New(), product, domain.ProductCodeInput{ArticleCode: "1"})
	assert.Contains(t, apperr.As(err).Fields, "partnerId")

	got, ok, err := h.svc.ProductCodeFor(context.Background(), p.ID, product)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "8991234567890", got.Barcode)

	updated, err := h.svc.UpdateProductCode(h.ctx(), c.ID, c.Version, domain.ProductCodeInput{ArticleCode: "100235", Description: "Tube 30 ml"})
	require.NoError(t, err)
	res, err := h.svc.ListProductCodes(h.ctx(), service.ProductCodeQuery{PartnerID: &p.ID}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "100235", res.Items[0].ArticleCode)
	refs, err := h.svc.PartnerRefs(context.Background(), []uuid.UUID{p.ID})
	require.NoError(t, err)
	assert.Equal(t, p.Code, refs[p.ID].Code)

	require.NoError(t, h.svc.DeleteProductCode(h.ctx(), updated.ID, updated.Version))
	_, ok, err = h.svc.ProductCodeFor(context.Background(), p.ID, product)
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = h.svc.CreateProductCode(h.ctx(), p.ID, product, domain.ProductCodeInput{ArticleCode: "100235"})
	require.NoError(t, err, "kode yang dihapus boleh dibuat ulang")
}

func TestSupplierEncryptedBankAccounts(t *testing.T) {
	h := setup(t)
	s, err := h.svc.CreateSupplier(h.ctx(), domain.SupplierInput{
		Code: uniqueCode("mkl"), Name: "PT Maklon Sehat", Type: domain.SupplierTollManufacturer, IsBusinessEntity: true,
		NPWP: ptr("012345678901000"), DefaultWithholding: domain.WithholdingPPh23, PaymentTermDays: 30,
		BankAccounts: []domain.BankAccountInput{{BankName: "BCA", AccountNumber: "1234567890", AccountName: "PT Maklon Sehat"}},
	})
	require.NoError(t, err)
	require.Len(t, s.BankAccounts, 1)

	var enc string
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT account_number_enc FROM party_bank_accounts WHERE party_id = $1`, s.ID).Scan(&enc))
	assert.NotContains(t, enc, "1234567890", "nomor rekening terenkripsi")

	// Audit trail tidak memuat NPWP/rekening asli.
	var changes string
	require.NoError(t, h.pool.QueryRow(context.Background(),
		`SELECT changes::text FROM audit_logs WHERE entity_id = $1`, s.ID).Scan(&changes))
	assert.NotContains(t, changes, "1234567890")
	assert.NotContains(t, changes, "012345678901000")

	acc := s.BankAccounts[0]
	updated, err := h.svc.UpdateSupplier(h.ctx(), s.ID, s.Version, domain.SupplierInput{
		Code: s.Code, Name: s.Name, Type: s.Type, IsBusinessEntity: true, DefaultWithholding: domain.WithholdingPPh42,
		BankAccounts: []domain.BankAccountInput{
			{ID: acc.ID, BankName: "BCA KCP Bandung", AccountName: acc.AccountName},
			{BankName: "Mandiri", AccountNumber: "9876543210", AccountName: "PT Maklon Sehat", IsDefault: true},
		},
	})
	require.NoError(t, err)
	got, err := h.svc.SupplierByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Equal(t, updated.Version, got.Version)
	assert.Equal(t, "012345678901000", got.NPWP)
	assert.Equal(t, domain.WithholdingPPh42, got.DefaultWithholding)
	require.Len(t, got.BankAccounts, 2)
	assert.Equal(t, "Mandiri", got.BankAccounts[0].BankName, "rekening utama di urutan pertama")
	assert.Equal(t, "1234567890", got.BankAccounts[1].AccountNumber, "nomor lama dipertahankan")

	_, err = h.svc.CreateSupplier(h.ctx(), domain.SupplierInput{
		Code: s.Code, Name: "X", Type: domain.SupplierRegular, DefaultWithholding: domain.WithholdingNone,
	})
	assert.Equal(t, apperr.CodeDuplicateCode, code(err))

	res, err := h.svc.ListSuppliers(h.ctx(service.PermSupplierRead), service.SupplierQuery{Type: domain.SupplierTollManufacturer, Search: s.Code}, paging.New(nil, nil))
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Len(t, res.Items[0].BankAccounts, 2)

	require.NoError(t, h.svc.DeleteSupplier(h.ctx(), s.ID, got.Version))
	_, err = h.svc.GetSupplier(h.ctx(), s.ID)
	assert.Equal(t, apperr.CodeNotFound, code(err))
	_, err = h.svc.GetSupplier(h.ctx(service.PermCustomerRead), s.ID)
	assert.Equal(t, apperr.CodeForbidden, code(err))
}
