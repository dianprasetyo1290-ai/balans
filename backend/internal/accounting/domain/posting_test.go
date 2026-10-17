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

func ptr(id uuid.UUID) *uuid.UUID { return &id }

var (
	channelID  = uuid.New()
	subID      = uuid.New()
	divisionID = uuid.New()
	campaignID = uuid.New()
	date       = clock.NewBusinessDate(2026, time.October, 19)
)

// Rule contoh mengikuti seed migrasi 20261018000500.
var (
	ruleA01 = domain.PostingRule{
		Code: "A-01", IsActive: true, RequiredDimensions: []domain.Dimension{domain.DimensionDivision},
		Accounts: []domain.RuleAccount{
			{AmountKey: "inventory_value", Side: domain.SideDebit, Resolver: domain.ResolverLocationInventory, AccountCode: "1311"},
			{AmountKey: "inventory_value", Side: domain.SideCredit, Resolver: domain.ResolverFixed, AccountCode: "2102"},
		},
	}
	ruleA05 = domain.PostingRule{
		Code: "A-05", IsActive: true, RequiredDimensions: []domain.Dimension{domain.DimensionChannel, domain.DimensionSubChannel},
		Accounts: []domain.RuleAccount{
			{AmountKey: "receivable_value", Side: domain.SideDebit, Resolver: domain.ResolverReceivable, AccountCode: "1201"},
			{AmountKey: "discount_value", Side: domain.SideDebit, Resolver: domain.ResolverFixed, AccountCode: "4102"},
			{AmountKey: "gross_sales", Side: domain.SideCredit, Resolver: domain.ResolverFixed, AccountCode: "4101"},
		},
	}
	ruleA14 = domain.PostingRule{
		Code: "A-14", IsActive: true, RequiredDimensions: []domain.Dimension{domain.DimensionDivision},
		Accounts: []domain.RuleAccount{
			{AmountKey: "shortage_value", Side: domain.SideDebit, Resolver: domain.ResolverFixed, AccountCode: "6401"},
			{AmountKey: "shortage_value", Side: domain.SideCredit, Resolver: domain.ResolverLocationInventory, AccountCode: "1311"},
		},
	}
	ruleA11 = domain.PostingRule{
		Code: "A-11", IsActive: true, RequiredDimensions: []domain.Dimension{domain.DimensionDivision, domain.DimensionCampaign},
		Accounts: []domain.RuleAccount{
			{AmountKey: "usage_value", Side: domain.SideDebit, Resolver: domain.ResolverFixed, AccountCode: "6103"},
			{AmountKey: "usage_value", Side: domain.SideCredit, Resolver: domain.ResolverFixed, AccountCode: "1311"},
		},
	}
	ruleA36 = domain.PostingRule{
		Code: "A-36", IsActive: true,
		Accounts: []domain.RuleAccount{
			{AmountKey: "opening_debit", Side: domain.SideDebit, Resolver: domain.ResolverOpeningBalance},
			{AmountKey: "opening_credit", Side: domain.SideCredit, Resolver: domain.ResolverOpeningBalance},
			{AmountKey: "opening_difference", Side: domain.SideCredit, Resolver: domain.ResolverFixed, AccountCode: "3901"},
		},
	}
)

func request(rule string, amounts map[string]string, mutate ...func(*domain.PostingRequest)) domain.PostingRequest {
	r := domain.PostingRequest{
		RuleCode: rule, SourceType: "goods_receipt", SourceID: uuid.New(), BusinessDate: date,
		Amounts: map[string]money.Amount{},
		Dimensions: domain.Dimensions{
			ChannelID: ptr(channelID), SubChannelID: ptr(subID), DivisionID: ptr(divisionID), CampaignID: ptr(campaignID),
		},
	}
	for k, v := range amounts {
		r.Amounts[k] = money.MustParse(v)
	}
	for _, m := range mutate {
		m(&r)
	}
	return r
}

type line struct {
	account string
	side    domain.Side
	amount  string
}

func simplify(lines []domain.PlannedLine) []line {
	out := make([]line, len(lines))
	for i, l := range lines {
		out[i] = line{l.AccountCode, l.Side, l.Amount.String()}
	}
	return out
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name    string
		rule    domain.PostingRule
		req     domain.PostingRequest
		want    []line
		errCode apperr.Code
		errKey  string
	}{
		{
			name: "A-01 penerimaan barang memakai akun default lokasi gudang",
			rule: ruleA01, req: request("A-01", map[string]string{"inventory_value": "1500000.00"}),
			want: []line{{"1311", domain.SideDebit, "1500000.00"}, {"2102", domain.SideCredit, "1500000.00"}},
		},
		{
			name: "A-01 akun persediaan packaging dari dokumen sumber",
			rule: ruleA01,
			req: request("A-01", map[string]string{"inventory_value": "250000"}, func(r *domain.PostingRequest) {
				r.Accounts = map[domain.Resolver]string{domain.ResolverLocationInventory: "1301"}
			}),
			want: []line{{"1301", domain.SideDebit, "250000.00"}, {"2102", domain.SideCredit, "250000.00"}},
		},
		{
			name: "A-05 penjualan dengan diskon balance",
			rule: ruleA05,
			req:  request("A-05", map[string]string{"receivable_value": "900000", "discount_value": "100000", "gross_sales": "1000000"}),
			want: []line{
				{"1201", domain.SideDebit, "900000.00"}, {"4102", domain.SideDebit, "100000.00"}, {"4101", domain.SideCredit, "1000000.00"},
			},
		},
		{
			name: "A-05 nilai nol dilewati",
			rule: ruleA05,
			req:  request("A-05", map[string]string{"receivable_value": "1000000", "discount_value": "0", "gross_sales": "1000000"}),
			want: []line{{"1201", domain.SideDebit, "1000000.00"}, {"4101", domain.SideCredit, "1000000.00"}},
		},
		{
			name: "A-14 selisih lebih (negatif) membalik sisi",
			rule: ruleA14, req: request("A-14", map[string]string{"shortage_value": "-75000"}),
			want: []line{{"6401", domain.SideCredit, "75000.00"}, {"1311", domain.SideDebit, "75000.00"}},
		},
		{
			name: "A-36 saldo awal dipecah per akun dengan selisih ke 3901",
			rule: ruleA36,
			req: request("A-36", nil, func(r *domain.PostingRequest) {
				r.Amounts = map[string]money.Amount{"opening_difference": money.MustParse("50")}
				r.Splits = []domain.PostingSplit{
					{AmountKey: "opening_debit", AccountCode: "1101", Amount: money.MustParse("100")},
					{AmountKey: "opening_debit", AccountCode: "1111", Amount: money.MustParse("200")},
					{AmountKey: "opening_credit", AccountCode: "2101", Amount: money.MustParse("250")},
					{AmountKey: "opening_credit", AccountCode: "2102", Amount: money.Zero},
				}
			}),
			want: []line{
				{"1101", domain.SideDebit, "100.00"}, {"1111", domain.SideDebit, "200.00"},
				{"2101", domain.SideCredit, "250.00"}, {"3901", domain.SideCredit, "50.00"},
			},
		},
		{
			name: "tidak balance ditolak",
			rule: ruleA05, req: request("A-05", map[string]string{"receivable_value": "900000", "gross_sales": "1000000"}),
			errCode: apperr.CodeValidationFailed, errKey: "amounts",
		},
		{
			name: "dimensi wajib channel kosong ditolak",
			rule: ruleA05,
			req: request("A-05", map[string]string{"receivable_value": "1", "gross_sales": "1"}, func(r *domain.PostingRequest) {
				r.Dimensions.ChannelID, r.Dimensions.SubChannelID = nil, nil
			}),
			errCode: apperr.CodeValidationFailed, errKey: "dimensions.channelId",
		},
		{
			name: "kampanye wajib A-11",
			rule: ruleA11,
			req: request("A-11", map[string]string{"usage_value": "10"}, func(r *domain.PostingRequest) {
				r.Dimensions.CampaignID = nil
			}),
			errCode: apperr.CodeValidationFailed, errKey: "dimensions.campaignId",
		},
		{
			name: "sub-channel tanpa channel ditolak",
			rule: ruleA01,
			req: request("A-01", map[string]string{"inventory_value": "10"}, func(r *domain.PostingRequest) {
				r.Dimensions.ChannelID = nil
			}),
			errCode: apperr.CodeValidationFailed, errKey: "dimensions.channelId",
		},
		{
			name: "nilai tidak dikenal ditolak", rule: ruleA01,
			req:     request("A-01", map[string]string{"inventory_value": "10", "grni": "10"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "akun resolver yang tidak dipakai rule ditolak", rule: ruleA01,
			req: request("A-01", map[string]string{"inventory_value": "10"}, func(r *domain.PostingRequest) {
				r.Accounts = map[domain.Resolver]string{domain.ResolverBank: "1111"}
			}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "lebih dari 2 desimal ditolak", rule: ruleA01,
			req:     request("A-01", map[string]string{"inventory_value": "10.005"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "semua nol ditolak", rule: ruleA01, req: request("A-01", map[string]string{"inventory_value": "0"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "rule nonaktif ditolak", rule: func() domain.PostingRule { r := ruleA01; r.IsActive = false; return r }(),
			req:     request("A-01", map[string]string{"inventory_value": "10"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "rule tanpa jurnal (A-20) ditolak", rule: domain.PostingRule{Code: "A-20", IsActive: true},
			req:     request("A-20", map[string]string{"x": "1"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "akun resolver tanpa default wajib dikirim", rule: ruleA36,
			req:     request("A-36", map[string]string{"opening_debit": "10", "opening_credit": "10"}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "nilai di Amounts dan Splits sekaligus ditolak", rule: ruleA36,
			req: request("A-36", map[string]string{"opening_debit": "10"}, func(r *domain.PostingRequest) {
				r.Splits = []domain.PostingSplit{{AmountKey: "opening_debit", AccountCode: "1101", Amount: money.MustParse("10")}}
			}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "split dengan key tidak dikenal ditolak", rule: ruleA36,
			req: request("A-36", nil, func(r *domain.PostingRequest) {
				r.Splits = []domain.PostingSplit{{AmountKey: "unknown", AccountCode: "1101", Amount: money.MustParse("10")}}
			}),
			errCode: apperr.CodeValidationFailed, errKey: "postingRule",
		},
		{
			name: "dokumen sumber wajib", rule: ruleA01,
			req: request("A-01", map[string]string{"inventory_value": "10"}, func(r *domain.PostingRequest) {
				r.SourceID, r.SourceType, r.RuleCode = uuid.Nil, "", ""
				r.BusinessDate = clock.BusinessDate{}
				r.Party = &domain.Party{}
			}),
			errCode: apperr.CodeValidationFailed, errKey: "sourceId",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines, err := tt.rule.Plan(tt.req)
			if tt.errCode != "" {
				require.Error(t, err)
				ae := apperr.As(err)
				assert.Equal(t, tt.errCode, ae.Code)
				assert.Contains(t, ae.Fields, tt.errKey, ae.Fields)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, simplify(lines))
		})
	}
}

func TestSideAndRule(t *testing.T) {
	assert.Equal(t, domain.SideCredit, domain.SideDebit.Opposite())
	assert.Equal(t, domain.SideDebit, domain.SideCredit.Opposite())
	assert.True(t, ruleA05.HasDimension(domain.DimensionChannel))
	assert.False(t, ruleA05.HasDimension(domain.DimensionDivision))
}
