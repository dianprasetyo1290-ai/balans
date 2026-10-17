package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

var oct20 = clock.NewBusinessDate(2026, time.October, 20)

func rp(n int64) money.Amount { return money.FromInt(n) }

func TestDocStatusLabel(t *testing.T) {
	assert.Equal(t, "Menunggu Pembayaran", domain.StatusAwaitingPayment.Label())
	assert.Equal(t, "Lunas", domain.StatusPaid.Label())
	assert.Equal(t, "x", domain.DocStatus("x").Label())
}

func TestNewBankAccount(t *testing.T) {
	gl := uuid.New()
	ok := domain.BankAccountInput{
		Code: " bca-ops ", Name: "BCA Operasional", BankName: "BCA", AccountNumber: "123-456 7890", AccountHolder: "PT Balans",
		AccountID: gl, IsActive: true,
	}
	b, err := domain.NewBankAccount(ok)
	require.NoError(t, err)
	assert.Equal(t, "BCA-OPS", b.Code)
	assert.Equal(t, domain.BankKindBank, b.Kind)
	assert.Equal(t, "1234567890", b.AccountNumber)
	assert.Equal(t, "••••7890", b.MaskedNumber())
	assert.Equal(t, "7890", b.Last4())
	assert.Equal(t, "BCA Operasional ••••7890", b.Label())
	assert.Empty(t, b.UsableError())

	cash, err := domain.NewBankAccount(domain.BankAccountInput{
		Code: "KAS", Name: "Kas Besar", Kind: domain.BankKindCash, BankName: "x", AccountNumber: "99", AccountID: gl, IsActive: true,
	})
	require.NoError(t, err)
	assert.Empty(t, cash.AccountNumber)
	assert.Equal(t, "Kas Besar", cash.Label())
	assert.Empty(t, cash.MaskedNumber())

	tests := []struct {
		name  string
		mod   func(in *domain.BankAccountInput)
		field string
	}{
		{"kode", func(in *domain.BankAccountInput) { in.Code = "a" }, "code"},
		{"nama", func(in *domain.BankAccountInput) { in.Name = "" }, "name"},
		{"jenis", func(in *domain.BankAccountInput) { in.Kind = "giro" }, "kind"},
		{"bank", func(in *domain.BankAccountInput) { in.BankName = "" }, "bankName"},
		{"nomor", func(in *domain.BankAccountInput) { in.AccountNumber = "12ab" }, "accountNumber"},
		{"akun", func(in *domain.BankAccountInput) { in.AccountID = uuid.Nil }, "accountId"},
		{"pemilik", func(in *domain.BankAccountInput) { in.AccountHolder = string(make([]byte, 151)) }, "accountHolder"},
		{"keterangan", func(in *domain.BankAccountInput) { in.Description = string(make([]byte, 501)) }, "description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ok
			tt.mod(&in)
			_, err := domain.NewBankAccount(in)
			assert.Contains(t, fieldsOf(t, err), tt.field)
		})
	}

	other := uuid.New()
	upd := ok
	upd.AccountID, upd.Name = other, "BCA Baru"
	b.AccountCode = "1111"
	require.NoError(t, b.Update(upd))
	assert.Equal(t, "BCA Baru", b.Name)
	assert.Empty(t, b.AccountCode, "akun GL berubah → kode tampilan dikosongkan")
	upd.Name = ""
	assert.Error(t, b.Update(upd))

	b.IsActive = false
	assert.Contains(t, b.UsableError(), "tidak aktif")
	now := time.Now()
	b.DeletedAt = &now
	assert.Contains(t, b.UsableError(), "dihapus")
	assert.True(t, apperr.HasCode(b.Update(ok), apperr.CodeInvalidStateTransition))

	assert.Equal(t, "••••", domain.MaskAccountNumber("12"))
	short := domain.BankAccount{AccountNumber: "12"}
	assert.Equal(t, "12", short.Last4())
}

func TestValidateCashBankAccount(t *testing.T) {
	good := domain.Account{Code: "1111", IsPostable: true, IsActive: true}
	require.NoError(t, domain.ValidateCashBankAccount(good))
	for _, a := range []domain.Account{
		{Code: "6101", IsPostable: true, IsActive: true},
		{Code: "1102", IsPostable: true, IsActive: true},
		{Code: "1100", IsPostable: false, IsActive: true},
		{Code: "1112", IsPostable: true, IsActive: false},
	} {
		assert.Contains(t, fieldsOf(t, domain.ValidateCashBankAccount(a)), "accountId", a.Code)
	}
}

func TestValidateCounterAccount(t *testing.T) {
	bank := uuid.New()
	good := domain.Account{ID: uuid.New(), Code: "7202", IsPostable: true, IsActive: true, AllowManualPosting: true}
	require.NoError(t, domain.ValidateCounterAccount(good, bank))
	for _, a := range []domain.Account{
		{ID: bank, Code: "1111", IsPostable: true, IsActive: true, AllowManualPosting: true},
		{ID: uuid.New(), Code: "7000", IsActive: true, AllowManualPosting: true},
		{ID: uuid.New(), Code: "7201", IsPostable: true, AllowManualPosting: true},
		{ID: uuid.New(), Code: "1201", IsPostable: true, IsActive: true},
	} {
		assert.Contains(t, fieldsOf(t, domain.ValidateCounterAccount(a, bank)), "counterAccountId", a.Code)
	}
}

func paymentInput() domain.PaymentInput {
	return domain.PaymentInput{
		Direction: domain.PaymentOutgoing, BankAccountID: uuid.New(), PaymentDate: oct20, Amount: rp(250_000),
		CounterAccountID: uuid.New(), PartyName: " Vendor ", Reference: " TRF-1 ",
	}
}

func TestPaymentLifecycle(t *testing.T) {
	p, err := domain.NewPayment(paymentInput())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDraft, p.Status)
	assert.Equal(t, "Vendor", p.PartyName)
	assert.Equal(t, "TRF-1", p.Reference)
	assert.Equal(t, []domain.Action{domain.ActionEdit, domain.ActionPost, domain.ActionCancel}, p.Actions())
	assert.Equal(t, "Keluar", p.Direction.Label())
	assert.Equal(t, "Masuk", domain.PaymentIncoming.Label())

	p.BankGLCode, p.CounterCode = "1111", "7202"
	assert.Equal(t, map[string]money.Amount{domain.AmountOutgoing: rp(250_000)}, p.Amounts())
	assert.Equal(t, "7202", p.Accounts()[domain.ResolverPayable])
	assert.Equal(t, "-250000.00", p.Signed().String())

	in := paymentInput()
	in.Amount = rp(300_000)
	require.NoError(t, p.Update(in))
	assert.Equal(t, rp(300_000), p.Amount)

	require.NoError(t, p.Post("BYR/2026/10/00001", uuid.New(), time.Now(), uuid.New()))
	assert.Equal(t, domain.StatusPosted, p.Status)
	assert.Equal(t, []domain.Action{domain.ActionCancel}, p.Actions())
	assert.True(t, apperr.HasCode(p.Update(in), apperr.CodeInvalidStateTransition))
	assert.Error(t, p.Post("x", uuid.New(), time.Now(), uuid.New()))

	p.Reconciled = true
	assert.Empty(t, p.Actions())
	assert.True(t, apperr.HasCode(p.Cancel("salah"), apperr.CodeInvalidStateTransition))
	p.Reconciled = false
	assert.Contains(t, fieldsOf(t, p.Cancel(" ")), "reason")
	require.NoError(t, p.Cancel("salah input"))
	assert.Equal(t, domain.StatusCancelled, p.Status)
	assert.Error(t, p.Cancel("lagi"))
	assert.Empty(t, p.Actions())

	src := uuid.New()
	in = paymentInput()
	in.Direction, in.SourceID, in.SourceType, in.SourceLabel = domain.PaymentIncoming, &src, "invoice", "INV/1"
	q, err := domain.NewPayment(in)
	require.NoError(t, err)
	q.BankGLCode, q.CounterCode = "1111", "1201"
	assert.Equal(t, "1201", q.Accounts()[domain.ResolverReceivable])
	assert.Equal(t, rp(250_000), q.Amounts()[domain.AmountIncoming])
	assert.Equal(t, "250000.00", q.Signed().String())
	require.NoError(t, q.Post("BYR/2", uuid.New(), time.Now(), uuid.New()))
	assert.Empty(t, q.Actions(), "pembayaran dokumen dibatalkan dari dokumen sumber")
	assert.True(t, apperr.HasCode(q.Cancel("x"), apperr.CodeInvalidStateTransition))

	// Tanpa source ID, jenis & label sumber dikosongkan; tanpa party ID, jenis pihak dikosongkan.
	in = paymentInput()
	in.SourceType, in.PartyType = "invoice", "customer"
	r, err := domain.NewPayment(in)
	require.NoError(t, err)
	assert.Empty(t, r.SourceType)
	assert.Empty(t, r.PartyType)
}

func TestNewPaymentValidation(t *testing.T) {
	tests := []struct {
		name  string
		mod   func(in *domain.PaymentInput)
		field string
	}{
		{"arah", func(in *domain.PaymentInput) { in.Direction = "x" }, "direction"},
		{"rekening", func(in *domain.PaymentInput) { in.BankAccountID = uuid.Nil }, "bankAccountId"},
		{"tanggal", func(in *domain.PaymentInput) { in.PaymentDate = clock.BusinessDate{} }, "paymentDate"},
		{"nominal nol", func(in *domain.PaymentInput) { in.Amount = money.Zero }, "amount"},
		{"nominal desimal", func(in *domain.PaymentInput) { in.Amount = money.MustParse("1.234") }, "amount"},
		{"nominal besar", func(in *domain.PaymentInput) { in.Amount = money.MustParse("12345678901234567") }, "amount"},
		{"akun lawan", func(in *domain.PaymentInput) { in.CounterAccountID = uuid.Nil }, "counterAccountId"},
		{"pihak", func(in *domain.PaymentInput) { id := uuid.New(); in.PartyID = &id }, "partyType"},
		{"nama pihak", func(in *domain.PaymentInput) { in.PartyName = string(make([]rune, 151)) }, "partyName"},
		{"referensi", func(in *domain.PaymentInput) { in.Reference = string(make([]byte, 101)) }, "reference"},
		{"keterangan", func(in *domain.PaymentInput) { in.Description = string(make([]byte, 501)) }, "description"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := paymentInput()
			tt.mod(&in)
			_, err := domain.NewPayment(in)
			assert.Contains(t, fieldsOf(t, err), tt.field)
		})
	}
	p, err := domain.NewPayment(paymentInput())
	require.NoError(t, err)
	bad := paymentInput()
	bad.Amount = money.Zero
	assert.Error(t, p.Update(bad))
}
