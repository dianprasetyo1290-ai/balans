package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/party/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
)

func fields(t *testing.T, err error) map[string]string {
	t.Helper()
	require.Error(t, err)
	e := apperr.As(err)
	require.Equal(t, apperr.CodeValidationFailed, e.Code, err.Error())
	return e.Fields
}

func ptr[T any](v T) *T { return &v }

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0812-3456-7890", "+6281234567890", true},
		{"081234567890", "+6281234567890", true},
		{"6281234567890", "+6281234567890", true},
		{"+62 812 3456 7890", "+6281234567890", true},
		{"+62(812)3456.7890", "+6281234567890", true},
		{"81234567890", "+6281234567890", true},
		{"021-5551234", "+62215551234", true},
		{"+1 202 555 0100", "", false},
		{"0812", "", false},
		{"08123456789012345", "", false},
		{"0812abc", "", false},
		{"00812345678", "", false},
		{"", "", false},
		{"62+812345678", "", false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := domain.NormalizePhone(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestPhoneSearchDigits(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0812 34", "81234", true},
		{"+62812", "812", false},
		{"628123", "8123", true},
		{"4567", "4567", true},
		{"Budi", "", false},
		{"12", "", false},
	}
	for _, c := range cases {
		got, ok := domain.PhoneSearchDigits(c.in)
		assert.Equal(t, c.ok, ok, c.in)
		if ok {
			assert.Equal(t, c.want, got, c.in)
		}
	}
}

func TestNormalizeNPWPAndAccount(t *testing.T) {
	n, ok := domain.NormalizeNPWP("01.234.567.8-901.000")
	assert.True(t, ok)
	assert.Equal(t, "012345678901000", n)
	n, ok = domain.NormalizeNPWP("3201 0101 0101 0001")
	assert.True(t, ok)
	assert.Len(t, n, 16)
	_, ok = domain.NormalizeNPWP("12345")
	assert.False(t, ok)
	_, ok = domain.NormalizeNPWP("01.234.567.8-901.00X")
	assert.False(t, ok)

	a, ok := domain.NormalizeAccountNumber("123-456 7890")
	assert.True(t, ok)
	assert.Equal(t, "1234567890", a)
	_, ok = domain.NormalizeAccountNumber("1234")
	assert.False(t, ok)
}

var now = time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)

func customerInput() domain.CustomerInput {
	return domain.CustomerInput{
		FullName: "  Siti Aminah ", Phone: "0812-3456-7890", Email: "Siti@Example.com", Type: domain.CustomerRetail,
		Addresses: []domain.AddressInput{
			{Label: "Rumah", RecipientName: "Siti", AddressLine: "Jl. Mawar 1", City: "Bandung", Latitude: ptr("-6.9175"), Longitude: ptr("107.6191")},
			{RecipientName: "Siti", AddressLine: "Jl. Melati 2", City: "Jakarta", Phone: "0813 1111 2222"},
		},
	}
}

func TestNewCustomer(t *testing.T) {
	c, err := domain.NewCustomer(customerInput(), now)
	require.NoError(t, err)
	assert.Equal(t, "Siti Aminah", c.FullName)
	assert.Equal(t, "+6281234567890", c.Phone)
	assert.Equal(t, "siti@example.com", c.Email)
	assert.True(t, c.Addresses[0].IsDefault, "alamat pertama otomatis jadi alamat utama")
	assert.Equal(t, "+6281311112222", c.Addresses[1].Phone)
	assert.Equal(t, "-6.917500", c.Addresses[0].Latitude.StringFixed(6))
	assert.False(t, c.HasMarketingConsent())
	assert.Nil(t, c.MarketingConsentAt)
}

func TestNewCustomerValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.CustomerInput)
		field  string
	}{
		{"nama kosong", func(in *domain.CustomerInput) { in.FullName = " " }, "fullName"},
		{"hp kosong", func(in *domain.CustomerInput) { in.Phone = "" }, "phone"},
		{"hp salah", func(in *domain.CustomerInput) { in.Phone = "12" }, "phone"},
		{"email salah", func(in *domain.CustomerInput) { in.Email = "bukan email" }, "email"},
		{"tipe salah", func(in *domain.CustomerInput) { in.Type = "vip" }, "customerType"},
		{"karyawan tanpa id", func(in *domain.CustomerInput) { in.Type = domain.CustomerEmployee }, "employeeId"},
		{"id tanpa tipe karyawan", func(in *domain.CustomerInput) { in.EmployeeID = ptr(uuid.New()) }, "customerType"},
		{"dua alamat utama", func(in *domain.CustomerInput) {
			in.Addresses[0].IsDefault, in.Addresses[1].IsDefault = true, true
		}, "addresses"},
		{"alamat tanpa kota", func(in *domain.CustomerInput) { in.Addresses[0].City = "" }, "addresses.0.city"},
		{"koordinat salah", func(in *domain.CustomerInput) { in.Addresses[0].Latitude = ptr("200") }, "addresses.0.latitude"},
		{"koordinat setengah", func(in *domain.CustomerInput) { in.Addresses[0].Longitude = nil }, "addresses.0.latitude"},
		{"hp alamat salah", func(in *domain.CustomerInput) { in.Addresses[1].Phone = "abc" }, "addresses.1.phone"},
		{"alamat ganda", func(in *domain.CustomerInput) {
			id := uuid.New()
			in.Addresses[0].ID, in.Addresses[1].ID = id, id
		}, "addresses.1.id"},
		{"terlalu banyak alamat", func(in *domain.CustomerInput) {
			in.Addresses = make([]domain.AddressInput, domain.MaxAddresses+1)
		}, "addresses"},
		{"catatan panjang", func(in *domain.CustomerInput) { in.Notes = string(make([]rune, 1001)) }, "notes"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := customerInput()
			c.mutate(&in)
			_, err := domain.NewCustomer(in, now)
			assert.Contains(t, fields(t, err), c.field)
		})
	}
}

func TestCustomerMarketingConsent(t *testing.T) {
	in := customerInput()
	in.MarketingConsent, in.MarketingConsentVersion = true, "v1"
	c, err := domain.NewCustomer(in, now)
	require.NoError(t, err)
	require.NotNil(t, c.MarketingConsentAt)
	assert.Equal(t, now, *c.MarketingConsentAt)

	// Persetujuan yang sama tidak mengubah waktu.
	later := now.Add(time.Hour)
	require.NoError(t, c.Update(in, later))
	assert.Equal(t, now, *c.MarketingConsentAt)

	// Versi teks baru dicatat ulang waktunya.
	in.MarketingConsentVersion = "v2"
	require.NoError(t, c.Update(in, later))
	assert.Equal(t, later, *c.MarketingConsentAt)
	assert.Equal(t, "v2", c.MarketingConsentVersion)

	// Penarikan persetujuan menghapus waktu dan versi.
	in.MarketingConsent = false
	require.NoError(t, c.Update(in, later))
	assert.Nil(t, c.MarketingConsentAt)
	assert.Empty(t, c.MarketingConsentVersion)
}

func TestCustomerUpdateAddresses(t *testing.T) {
	c, err := domain.NewCustomer(customerInput(), now)
	require.NoError(t, err)
	c.Addresses[0].ID, c.Addresses[1].ID = uuid.New(), uuid.New()
	before := append([]domain.Address(nil), c.Addresses...)

	in := customerInput()
	in.Addresses = []domain.AddressInput{{ID: before[1].ID, RecipientName: "Siti", AddressLine: "Jl. Melati 2", City: "Jakarta", IsDefault: true}}
	require.NoError(t, c.Update(in, now))
	assert.Equal(t, []uuid.UUID{before[0].ID}, domain.RemovedAddresses(before, c.Addresses))

	in.Addresses[0].ID = uuid.New()
	assert.Contains(t, fields(t, c.Update(in, now)), "addresses.0.id")
}

func TestCustomerTypeEmployee(t *testing.T) {
	in := customerInput()
	in.Type, in.EmployeeID = domain.CustomerEmployee, ptr(uuid.New())
	c, err := domain.NewCustomer(in, now)
	require.NoError(t, err)
	assert.Equal(t, domain.CustomerEmployee, c.Type)
	for _, typ := range []domain.CustomerType{domain.CustomerRetail, domain.CustomerB2B, domain.CustomerReseller, domain.CustomerEmployee} {
		assert.True(t, typ.Valid())
	}
}

func partnerInput() domain.PartnerInput {
	return domain.PartnerInput{
		Code: " alfa-01 ", Name: "Alfamart", Type: domain.PartnerConsignment, NPWP: ptr("01.234.567.8-901.000"),
		PICPhone: "0812 0000 1111", SalesUserID: ptr(uuid.New()),
	}
}

func TestNewPartner(t *testing.T) {
	p, err := domain.NewPartner(partnerInput())
	require.NoError(t, err)
	assert.Equal(t, "ALFA-01", p.Code)
	assert.Equal(t, "012345678901000", p.NPWP)
	assert.Equal(t, "+6281200001111", p.PICPhone)

	// NPWP nil = tetap, kosong = dihapus.
	in := partnerInput()
	in.NPWP, in.Name = nil, "Alfamart Baru"
	require.NoError(t, p.Update(in))
	assert.Equal(t, "012345678901000", p.NPWP)
	assert.Equal(t, "ALFA-01", p.Code, "kode tidak berubah")
	in.NPWP = ptr("")
	require.NoError(t, p.Update(in))
	assert.Empty(t, p.NPWP)
	for _, typ := range []domain.PartnerType{domain.PartnerModernTrade, domain.PartnerConsignment, domain.PartnerB2B} {
		assert.True(t, typ.Valid())
	}
}

func TestNewPartnerValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.PartnerInput)
		field  string
	}{
		{"kode kosong", func(in *domain.PartnerInput) { in.Code = "" }, "code"},
		{"kode simbol", func(in *domain.PartnerInput) { in.Code = "AL FA" }, "code"},
		{"kode panjang", func(in *domain.PartnerInput) { in.Code = "A234567890123456789012345678901" }, "code"},
		{"nama kosong", func(in *domain.PartnerInput) { in.Name = "" }, "name"},
		{"jenis salah", func(in *domain.PartnerInput) { in.Type = "retail" }, "partnerType"},
		{"npwp salah", func(in *domain.PartnerInput) { in.NPWP = ptr("123") }, "npwp"},
		{"hp pic salah", func(in *domain.PartnerInput) { in.PICPhone = "x" }, "picPhone"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := partnerInput()
			c.mutate(&in)
			_, err := domain.NewPartner(in)
			assert.Contains(t, fields(t, err), c.field)
		})
	}
}

func date(y int, m time.Month, d int) clock.BusinessDate { return clock.NewBusinessDate(y, m, d) }

func agreementInput() domain.AgreementInput {
	return domain.AgreementInput{
		Number: "PKS/ALFA/2026/01", Type: domain.AgreementConsignment, ValidFrom: date(2026, 1, 1),
		ValidTo: date(2026, 12, 31), MarginPercent: ptr("25"), PaymentTermDays: 30,
	}
}

func TestNewAgreementDefaultsAndStatus(t *testing.T) {
	partner := uuid.New()
	a, err := domain.NewAgreement(partner, agreementInput())
	require.NoError(t, err)
	assert.Equal(t, partner, a.PartnerID)
	assert.Equal(t, "25.0000", a.MarginPercent.StringFixed(4))
	// Default BRD U-02.
	assert.Equal(t, domain.BorneByPartner, a.LossBorneBy)
	assert.Equal(t, domain.BorneByPartner, a.PartnerDamageBorneBy)
	assert.Equal(t, domain.BorneByBalans, a.ProductDefectBorneBy)
	assert.Equal(t, domain.BorneByBalans, a.NearExpiryBorneBy)

	assert.Equal(t, domain.AgreementScheduled, a.StatusOn(date(2025, 12, 31)))
	assert.Equal(t, domain.AgreementActive, a.StatusOn(date(2026, 1, 1)))
	assert.Equal(t, domain.AgreementActive, a.StatusOn(date(2026, 12, 31)), "tanggal berakhir inklusif")
	assert.Equal(t, domain.AgreementExpired, a.StatusOn(date(2027, 1, 1)))
	assert.True(t, a.ActiveOn(date(2026, 6, 1)))

	in := agreementInput()
	in.LossBorneBy, in.MarginPercent = domain.BorneByBalans, nil
	require.NoError(t, a.Update(in))
	assert.Equal(t, domain.BorneByBalans, a.LossBorneBy)
	assert.Nil(t, a.MarginPercent)
}

func TestNewAgreementValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.AgreementInput)
		field  string
	}{
		{"nomor kosong", func(in *domain.AgreementInput) { in.Number = "" }, "agreementNumber"},
		{"jenis salah", func(in *domain.AgreementInput) { in.Type = "x" }, "agreementType"},
		{"mulai kosong", func(in *domain.AgreementInput) { in.ValidFrom = clock.BusinessDate{} }, "validFrom"},
		{"akhir kosong", func(in *domain.AgreementInput) { in.ValidTo = clock.BusinessDate{} }, "validTo"},
		{"akhir sebelum mulai", func(in *domain.AgreementInput) { in.ValidTo = date(2025, 1, 1) }, "validTo"},
		{"margin teks", func(in *domain.AgreementInput) { in.MarginPercent = ptr("abc") }, "consignmentMarginPercent"},
		{"margin 101", func(in *domain.AgreementInput) { in.MarginPercent = ptr("101") }, "consignmentMarginPercent"},
		{"termin negatif", func(in *domain.AgreementInput) { in.PaymentTermDays = -1 }, "paymentTermDays"},
		{"termin 366", func(in *domain.AgreementInput) { in.PaymentTermDays = 366 }, "paymentTermDays"},
		{"penanggung salah", func(in *domain.AgreementInput) { in.NearExpiryBorneBy = "x" }, "nearExpiryBorneBy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := agreementInput()
			c.mutate(&in)
			_, err := domain.NewAgreement(uuid.New(), in)
			assert.Contains(t, fields(t, err), c.field)
		})
	}
}

func TestRequireActive(t *testing.T) {
	a, err := domain.NewAgreement(uuid.New(), agreementInput())
	require.NoError(t, err)
	require.NoError(t, domain.RequireActive(&a, domain.AgreementConsignment, "Alfamart", date(2026, 3, 1)))

	err = domain.RequireActive(&a, domain.AgreementConsignment, "Alfamart", date(2027, 1, 1))
	assert.Contains(t, fields(t, err)["partnerId"], "PKS konsinyasi Alfamart tidak berlaku pada 2027-01-01")
	err = domain.RequireActive(nil, domain.AgreementModernTrade, "Alfamart", date(2026, 3, 1))
	assert.Contains(t, fields(t, err)["partnerId"], "PKS Modern Trade")
	err = domain.RequireActive(&a, domain.AgreementModernTrade, "Alfamart", date(2026, 3, 1))
	assert.Contains(t, fields(t, err), "partnerId")
}

func TestProductCode(t *testing.T) {
	partner, product := uuid.New(), uuid.New()
	c, err := domain.NewProductCode(partner, product, domain.ProductCodeInput{ArticleCode: " 100234 ", Barcode: "8991234567890"})
	require.NoError(t, err)
	assert.Equal(t, "100234", c.ArticleCode)
	require.NoError(t, c.Update(domain.ProductCodeInput{ArticleCode: "100235", Description: "Tube 30 ml"}))
	assert.Equal(t, "100235", c.ArticleCode)
	assert.Empty(t, c.Barcode)

	_, err = domain.NewProductCode(uuid.Nil, uuid.Nil, domain.ProductCodeInput{})
	f := fields(t, err)
	assert.Contains(t, f, "partnerId")
	assert.Contains(t, f, "productId")
	assert.Contains(t, f, "articleCode")
	assert.Contains(t, fields(t, c.Update(domain.ProductCodeInput{ArticleCode: "x", Barcode: string(make([]rune, 41))})), "barcode")
}

func supplierInput() domain.SupplierInput {
	return domain.SupplierInput{
		Code: "mkl-01", Name: "PT Maklon Sehat", Type: domain.SupplierTollManufacturer, IsBusinessEntity: true,
		NPWP: ptr("01.234.567.8-901.000"), DefaultWithholding: domain.WithholdingPPh23, PaymentTermDays: 30,
		Phone: "021 555 1234", Email: "ap@maklon.co.id",
		BankAccounts: []domain.BankAccountInput{
			{BankName: "BCA", AccountNumber: "123-456-7890", AccountName: "PT Maklon Sehat"},
			{BankName: "Mandiri", AccountNumber: "9876543210", AccountName: "PT Maklon Sehat"},
		},
	}
}

func TestNewSupplier(t *testing.T) {
	s, err := domain.NewSupplier(supplierInput())
	require.NoError(t, err)
	assert.Equal(t, "MKL-01", s.Code)
	assert.True(t, s.IsTollManufacturer())
	assert.Equal(t, "+62215551234", s.Phone)
	assert.Equal(t, "1234567890", s.BankAccounts[0].AccountNumber)
	def, ok := s.DefaultBankAccount()
	require.True(t, ok)
	assert.Equal(t, "BCA", def.BankName)
	for _, w := range []domain.WithholdingType{domain.WithholdingNone, domain.WithholdingPPh21, domain.WithholdingPPh23, domain.WithholdingPPh42} {
		assert.True(t, w.Valid())
	}
}

func TestSupplierUpdateKeepsAccountNumber(t *testing.T) {
	s, err := domain.NewSupplier(supplierInput())
	require.NoError(t, err)
	s.BankAccounts[0].ID, s.BankAccounts[1].ID = uuid.New(), uuid.New()
	before := append([]domain.BankAccount(nil), s.BankAccounts...)

	in := supplierInput()
	in.NPWP = nil
	in.BankAccounts = []domain.BankAccountInput{{ID: before[0].ID, BankName: "BCA KCP", AccountName: "PT Maklon Sehat", IsDefault: true}}
	require.NoError(t, s.Update(in))
	assert.Equal(t, "1234567890", s.BankAccounts[0].AccountNumber, "nomor kosong = tetap")
	assert.Equal(t, "012345678901000", s.NPWP)
	assert.Equal(t, []uuid.UUID{before[1].ID}, domain.RemovedBankAccounts(before, s.BankAccounts))

	in.BankAccounts[0].ID = uuid.New()
	assert.Contains(t, fields(t, s.Update(in)), "bankAccounts.0.id")
}

func TestNewSupplierValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.SupplierInput)
		field  string
	}{
		{"kode kosong", func(in *domain.SupplierInput) { in.Code = "" }, "code"},
		{"nama kosong", func(in *domain.SupplierInput) { in.Name = "" }, "name"},
		{"jenis salah", func(in *domain.SupplierInput) { in.Type = "x" }, "supplierType"},
		{"pph salah", func(in *domain.SupplierInput) { in.DefaultWithholding = "pph22" }, "defaultWithholding"},
		{"termin salah", func(in *domain.SupplierInput) { in.PaymentTermDays = 400 }, "paymentTermDays"},
		{"telepon salah", func(in *domain.SupplierInput) { in.Phone = "abc" }, "phone"},
		{"email salah", func(in *domain.SupplierInput) { in.Email = "x@" }, "email"},
		{"npwp salah", func(in *domain.SupplierInput) { in.NPWP = ptr("1") }, "npwp"},
		{"rekening baru tanpa nomor", func(in *domain.SupplierInput) { in.BankAccounts[0].AccountNumber = "" }, "bankAccounts.0.accountNumber"},
		{"rekening salah", func(in *domain.SupplierInput) { in.BankAccounts[0].AccountNumber = "12ab" }, "bankAccounts.0.accountNumber"},
		{"bank kosong", func(in *domain.SupplierInput) { in.BankAccounts[1].BankName = "" }, "bankAccounts.1.bankName"},
		{"dua rekening utama", func(in *domain.SupplierInput) {
			in.BankAccounts[0].IsDefault, in.BankAccounts[1].IsDefault = true, true
		}, "bankAccounts"},
		{"terlalu banyak rekening", func(in *domain.SupplierInput) {
			in.BankAccounts = make([]domain.BankAccountInput, domain.MaxBankAccounts+1)
		}, "bankAccounts"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := supplierInput()
			c.mutate(&in)
			_, err := domain.NewSupplier(in)
			assert.Contains(t, fields(t, err), c.field)
		})
	}
}
