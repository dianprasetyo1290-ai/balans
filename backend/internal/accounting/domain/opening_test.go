package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

func acct(code string, group Group) Account {
	return Account{ID: uuid.New(), Code: code, Name: "Akun " + code, Group: group, IsPostable: true, IsActive: true}
}

func TestValidateCutoverDate(t *testing.T) {
	assert.NoError(t, ValidateCutoverDate(clock.NewBusinessDate(2026, time.December, 31)))
	assert.NoError(t, ValidateCutoverDate(clock.NewBusinessDate(2027, time.February, 28)))
	assert.Error(t, ValidateCutoverDate(clock.NewBusinessDate(2026, time.December, 30)))
	assert.Error(t, ValidateCutoverDate(clock.BusinessDate{}))
}

func TestNewOpeningLine(t *testing.T) {
	bank := acct("1111", GroupAsset)
	l, err := NewOpeningLine(OpeningLineInput{AccountID: bank.ID, Debit: money.FromInt(100), Credit: money.Zero, Note: " x "}, bank)
	require.NoError(t, err)
	assert.Equal(t, "x", l.Note)

	cases := []struct {
		name string
		in   OpeningLineInput
		acc  Account
		key  string
	}{
		{"akun laba rugi", OpeningLineInput{Debit: money.FromInt(1), Credit: money.Zero}, acct("6301", GroupExpense), "accountId"},
		{"akun tidak ada", OpeningLineInput{Debit: money.FromInt(1), Credit: money.Zero}, Account{}, "accountId"},
		{"debit & kredit", OpeningLineInput{Debit: money.FromInt(1), Credit: money.FromInt(1)}, bank, "debit"},
		{"kosong", OpeningLineInput{Debit: money.Zero, Credit: money.Zero}, bank, "debit"},
		{"negatif", OpeningLineInput{Debit: money.FromInt(-1), Credit: money.Zero}, bank, "debit"},
		{"3 desimal", OpeningLineInput{Debit: money.MustParse("1.005"), Credit: money.Zero}, bank, "debit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewOpeningLine(tc.in, tc.acc)
			assert.Contains(t, fieldsOf(err), tc.key)
		})
	}
	inactive := bank
	inactive.IsActive = false
	_, err = NewOpeningLine(OpeningLineInput{Debit: money.FromInt(1), Credit: money.Zero}, inactive)
	assert.Contains(t, fieldsOf(err), "accountId")
}

func TestOpeningPosting(t *testing.T) {
	set := OpeningSet{ID: uuid.New(), CutoverDate: clock.NewBusinessDate(2026, time.December, 31), Status: OpeningDraft}
	assert.NoError(t, set.CheckEditable())
	_, err := OpeningPosting(set, nil)
	assert.Error(t, err)

	lines := []OpeningLine{
		{AccountCode: "1111", Debit: money.FromInt(1000), Credit: money.Zero},
		{AccountCode: "2101", Debit: money.Zero, Credit: money.FromInt(300)},
		{AccountCode: "3101", Debit: money.Zero, Credit: money.FromInt(500)},
	}
	tot := TotalsOf(lines)
	assert.Equal(t, "200.00", tot.Difference.String())
	req, err := OpeningPosting(set, lines)
	require.NoError(t, err)
	assert.Equal(t, RuleOpeningBalance, req.RuleCode)
	assert.Len(t, req.Splits, 3)
	assert.Equal(t, AmountOpeningCredit, req.Splits[1].AmountKey)
	assert.Equal(t, "200.00", req.Amounts[AmountOpeningDifference].String())

	rule := PostingRule{Code: "A-36", IsActive: true, Accounts: []RuleAccount{
		{AmountKey: AmountOpeningDebit, Side: SideDebit, Resolver: ResolverOpeningBalance},
		{AmountKey: AmountOpeningCredit, Side: SideCredit, Resolver: ResolverOpeningBalance},
		{AmountKey: AmountOpeningDifference, Side: SideCredit, Resolver: ResolverFixed, AccountCode: "3901"},
	}}
	planned, err := rule.Plan(req)
	require.NoError(t, err, "selisih ke 3901 membuat jurnal balance")
	assert.Len(t, planned, 4)

	set.Status = OpeningPosted
	assert.Error(t, set.CheckEditable())
}

func TestNewOpeningItem(t *testing.T) {
	cutover := clock.NewBusinessDate(2026, time.December, 31)
	ar := acct("1201", GroupAsset)
	in := OpeningItemInput{
		Type: OpeningReceivable, PartyType: PartyPartner, PartyID: uuid.New(), PartyCode: "MT-A", DocumentNumber: " INV-1 ",
		DocumentDate: clock.NewBusinessDate(2026, time.December, 15), DueDate: clock.NewBusinessDate(2027, time.January, 14),
		Amount: money.FromInt(4_500_000), OpenAmount: money.FromInt(4_000_000),
	}
	it, err := NewOpeningItem(in, ar, cutover)
	require.NoError(t, err)
	assert.Equal(t, "INV-1", it.DocumentNumber)
	assert.Equal(t, "4000000.00", it.Outstanding().String())

	require.NoError(t, it.Settle(money.FromInt(1_000_000)))
	assert.Equal(t, "3000000.00", it.Outstanding().String())
	assert.Error(t, it.Settle(money.FromInt(3_000_001)), "melebihi sisa")
	assert.Error(t, it.Settle(money.Zero))
	require.NoError(t, it.Settle(money.FromInt(-1_000_000)), "pembatalan pelunasan")
	assert.Error(t, it.Settle(money.FromInt(-1)))

	bad := in
	bad.PartyType, bad.DocumentNumber, bad.OpenAmount = PartySupplier, "", money.FromInt(5_000_000)
	bad.DocumentDate, bad.DueDate = clock.NewBusinessDate(2027, time.January, 2), clock.NewBusinessDate(2027, time.January, 1)
	_, err = NewOpeningItem(bad, acct("2101", GroupLiability), cutover)
	f := fieldsOf(err)
	for _, k := range []string{"partyCode", "accountCode", "documentNumber", "documentDate", "dueDate", "openAmount"} {
		assert.Contains(t, f, k)
	}

	ap := OpeningItemInput{
		Type: OpeningPayable, PartyType: PartySupplier, PartyID: uuid.New(), DocumentNumber: "BILL-1",
		DocumentDate: cutover, DueDate: cutover, Amount: money.FromInt(10), OpenAmount: money.FromInt(10),
	}
	_, err = NewOpeningItem(ap, acct("2101", GroupLiability), cutover)
	require.NoError(t, err)
	ap.PartyType = PartyCustomer
	_, err = NewOpeningItem(ap, acct("1201", GroupAsset), cutover)
	assert.Contains(t, fieldsOf(err), "partyCode")
	assert.Contains(t, fieldsOf(err), "accountCode")
	_, err = NewOpeningItem(OpeningItemInput{Type: "x"}, Account{}, cutover)
	assert.Contains(t, fieldsOf(err), "type")
}

func TestReconciliationCompare(t *testing.T) {
	ok := Compare("receivable", "Piutang", "1201", money.FromInt(10), money.FromInt(10))
	assert.Equal(t, ReconOK, ok.Status)
	bad := Compare("payable", "Utang", "2101", money.FromInt(10), money.FromInt(7))
	assert.Equal(t, ReconFailed, bad.Status)
	assert.Equal(t, "3.00", bad.Difference.String())
	skipped := ReconciliationCheck{Status: ReconSkipped}
	assert.True(t, ReconciliationPassed([]ReconciliationCheck{ok, skipped}))
	assert.False(t, ReconciliationPassed([]ReconciliationCheck{ok, bad}))
}
