package domain_test

import (
	"strings"
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

func TestNewAccount(t *testing.T) {
	tests := []struct {
		name   string
		in     domain.AccountInput
		fields []string
	}{
		{"valid", domain.AccountInput{Code: "1113", Name: "Bank — rekening 3", NormalBalance: domain.NormalDebit, IsPostable: true, AllowManualPosting: true, IsActive: true}, nil},
		{"kode 3 digit", domain.AccountInput{Code: "111", Name: "x", NormalBalance: domain.NormalDebit}, []string{"code"}},
		{"kelompok 8", domain.AccountInput{Code: "8101", Name: "x", NormalBalance: domain.NormalDebit}, []string{"code"}},
		{"nama kosong", domain.AccountInput{Code: "1113", Name: "  ", NormalBalance: domain.NormalDebit}, []string{"name"}},
		{"nama terlalu panjang", domain.AccountInput{Code: "1113", Name: strings.Repeat("a", 151), NormalBalance: domain.NormalDebit}, []string{"name"}},
		{"saldo normal salah", domain.AccountInput{Code: "1113", Name: "x", NormalBalance: "both"}, []string{"normalBalance"}},
		{"induk manual", domain.AccountInput{Code: "1113", Name: "x", NormalBalance: domain.NormalDebit, AllowManualPosting: true}, []string{"allowManualPosting"}},
		{"keterangan panjang", domain.AccountInput{Code: "1113", Name: "x", NormalBalance: domain.NormalDebit, Description: strings.Repeat("a", 501)}, []string{"description"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := domain.NewAccount(tt.in)
			if tt.fields == nil {
				require.NoError(t, err)
				assert.Equal(t, domain.GroupAsset, a.Group)
				return
			}
			fields := apperr.As(err).Fields
			for _, f := range tt.fields {
				assert.Contains(t, fields, f)
			}
		})
	}
}

func TestGroups(t *testing.T) {
	for g := domain.GroupAsset; g <= domain.GroupOther; g++ {
		assert.NotEmpty(t, g.Label())
	}
	assert.Equal(t, domain.NormalCredit, domain.GroupLiability.DefaultNormalBalance())
	assert.Equal(t, domain.NormalCredit, domain.GroupRevenue.DefaultNormalBalance())
	assert.Equal(t, domain.NormalDebit, domain.GroupExpense.DefaultNormalBalance())
	assert.True(t, domain.GroupRevenue.IsProfitLoss())
	assert.False(t, domain.GroupEquity.IsProfitLoss())
	assert.Equal(t, domain.Group(0), domain.GroupOfCode(""))
	assert.Equal(t, domain.GroupOther, domain.GroupOfCode("7301"))
}

func TestAccountUpdate(t *testing.T) {
	base := domain.Account{ID: uuid.New(), Code: "6306", Group: domain.GroupExpense, NormalBalance: domain.NormalDebit, IsPostable: true, IsActive: true}
	in := domain.AccountInput{Name: "Beban Umum", NormalBalance: domain.NormalDebit, IsPostable: true, AllowManualPosting: true, IsActive: true}

	a := base
	require.NoError(t, a.Update(in, domain.AccountUsage{}))
	assert.Equal(t, "Beban Umum", a.Name)
	assert.True(t, a.AllowManualPosting)

	tests := []struct {
		name  string
		in    domain.AccountInput
		usage domain.AccountUsage
		field string
	}{
		{"saldo normal akun terpakai", domain.AccountInput{Name: "x", NormalBalance: domain.NormalCredit, IsPostable: true}, domain.AccountUsage{JournalLines: 1}, "normalBalance"},
		{"jadi induk saat terpakai", domain.AccountInput{Name: "x", NormalBalance: domain.NormalDebit}, domain.AccountUsage{RuleMappings: 1}, "isPostable"},
		{"induk dengan anak jadi postable", domain.AccountInput{Name: "x", NormalBalance: domain.NormalDebit, IsPostable: true}, domain.AccountUsage{Children: 2}, "isPostable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := base
			if tt.name == "induk dengan anak jadi postable" {
				a.IsPostable = false
			}
			assert.Contains(t, apperr.As(a.Update(tt.in, tt.usage)).Fields, tt.field)
		})
	}
}

func TestAccountDeleteAndParent(t *testing.T) {
	a := domain.Account{ID: uuid.New(), Code: "1113", Group: domain.GroupAsset, IsPostable: true}
	assert.NoError(t, a.CheckDelete(domain.AccountUsage{}))
	for _, u := range []domain.AccountUsage{{JournalLines: 1}, {RuleMappings: 1}, {Children: 1}} {
		assert.True(t, apperr.HasCode(a.CheckDelete(u), apperr.CodeInvalidStateTransition))
	}

	header := domain.Account{ID: uuid.New(), Code: "1100", Group: domain.GroupAsset}
	assert.NoError(t, domain.ValidateParent(a, header, nil))
	assert.Contains(t, apperr.As(domain.ValidateParent(a, domain.Account{ID: uuid.New(), Group: domain.GroupAsset, IsPostable: true}, nil)).Fields, "parentId")
	assert.Contains(t, apperr.As(domain.ValidateParent(a, domain.Account{ID: uuid.New(), Group: domain.GroupLiability}, nil)).Fields, "parentId")
	assert.Contains(t, apperr.As(domain.ValidateParent(a, a, nil)).Fields, "parentId")
	assert.Contains(t, apperr.As(domain.ValidateParent(a, header, []uuid.UUID{a.ID})).Fields, "parentId")
}

func TestAccountPostingRules(t *testing.T) {
	expense := domain.Account{Code: "6306", Group: domain.GroupExpense, IsPostable: true, AllowManualPosting: true, IsActive: true}
	assert.Empty(t, expense.ManualPostingError())
	assert.Empty(t, expense.AutoPostingError())
	assert.Equal(t, []domain.Dimension{domain.DimensionDivision}, expense.ManualRequiredDimensions())

	control := domain.Account{Code: "1201", Group: domain.GroupAsset, IsPostable: true, IsActive: true}
	assert.Contains(t, control.ManualPostingError(), "jurnal otomatis")
	assert.Empty(t, control.AutoPostingError())
	assert.Nil(t, control.ManualRequiredDimensions())

	header := domain.Account{Code: "1100", Group: domain.GroupAsset, IsActive: true}
	assert.Contains(t, header.ManualPostingError(), "induk")
	assert.Contains(t, header.AutoPostingError(), "induk")

	inactive := domain.Account{Code: "1112", Group: domain.GroupAsset, IsPostable: true}
	assert.Contains(t, inactive.ManualPostingError(), "tidak aktif")
	assert.Contains(t, inactive.AutoPostingError(), "tidak aktif")

	revenue := domain.Account{Group: domain.GroupRevenue}
	assert.Equal(t, []domain.Dimension{domain.DimensionChannel, domain.DimensionSubChannel}, revenue.ManualRequiredDimensions())

	assert.Equal(t, "30.00", domain.NormalDebit.Balance(money.FromInt(100), money.FromInt(70)).String())
	assert.Equal(t, "-30.00", domain.NormalCredit.Balance(money.FromInt(100), money.FromInt(70)).String())
}

func TestDimensions(t *testing.T) {
	d := domain.Dimensions{DivisionID: ptr(divisionID)}
	assert.True(t, d.Has(domain.DimensionDivision))
	assert.False(t, d.Has(domain.DimensionChannel))
	assert.False(t, d.Has("x"))
	missing := d.Missing([]domain.Dimension{domain.DimensionChannel, domain.DimensionSubChannel, domain.DimensionCampaign}, "lines.0.", "akun 4101")
	assert.Equal(t, "Channel wajib diisi untuk akun 4101", missing["lines.0.channelId"])
	assert.Contains(t, missing, "lines.0.subChannelId")
	assert.Contains(t, missing, "lines.0.campaignId")

	for _, s := range []string{"channel", "sub_channel", "division", "campaign"} {
		dim, ok := domain.ParseDimension(s)
		assert.True(t, ok)
		assert.NotEmpty(t, dim.Label())
		assert.NotEmpty(t, dim.Field())
	}
	_, ok := domain.ParseDimension("x")
	assert.False(t, ok)
	assert.Equal(t, "x", domain.Dimension("x").Field())

	check := domain.DimensionCheck{}
	assert.Len(t, check.Errors("p."), 4)
	assert.Empty(t, domain.DimensionCheck{ChannelOK: true, SubChannelOK: true, DivisionOK: true, CampaignOK: true}.Errors(""))
}

func TestDimensionValues(t *testing.T) {
	jan1 := clock.NewBusinessDate(2027, time.January, 1)
	jan31 := clock.NewBusinessDate(2027, time.January, 31)
	budget := money.FromInt(5_000_000)
	negative := money.FromInt(-1)
	tests := []struct {
		name   string
		kind   domain.DimensionKind
		in     domain.DimensionInput
		fields []string
	}{
		{"channel valid", domain.KindChannel, domain.DimensionInput{Code: " Modern_Trade2 ", Name: "MT 2", IsActive: true}, nil},
		{"channel kode salah", domain.KindChannel, domain.DimensionInput{Code: "1x", Name: "x"}, []string{"code"}},
		{"sub-channel valid", domain.KindSubChannel, domain.DimensionInput{Code: "alfamart-01", Name: "Alfamart", ChannelID: ptr(channelID)}, nil},
		{"sub-channel tanpa channel", domain.KindSubChannel, domain.DimensionInput{Code: "x1", Name: "x"}, []string{"channelId"}},
		{"sub-channel kode salah", domain.KindSubChannel, domain.DimensionInput{Code: "!", Name: "x", ChannelID: ptr(channelID)}, []string{"code"}},
		{"sub-channel pihak tanpa id", domain.KindSubChannel, domain.DimensionInput{Code: "x1", Name: "x", ChannelID: ptr(channelID), PartyType: "partner"}, []string{"partyId"}},
		{"divisi valid", domain.KindDivision, domain.DimensionInput{Code: "ops", Name: "Operasional"}, nil},
		{"divisi kode salah", domain.KindDivision, domain.DimensionInput{Code: "O", Name: "x"}, []string{"code"}},
		{"kampanye valid", domain.KindCampaign, domain.DimensionInput{Code: "kmp-202701-ramadan", Name: "Ramadan", ValidFrom: jan1, ValidTo: jan31, Budget: &budget}, nil},
		{"kampanye format salah", domain.KindCampaign, domain.DimensionInput{Code: "KMP-202713-X1", Name: "x", ValidFrom: jan1, ValidTo: jan31}, []string{"code"}},
		{"kampanye periode terbalik", domain.KindCampaign, domain.DimensionInput{Code: "KMP-202701-AB", Name: "x", ValidFrom: jan31, ValidTo: jan1}, []string{"validTo"}},
		{"kampanye tanpa tanggal", domain.KindCampaign, domain.DimensionInput{Code: "KMP-202701-AB", Name: "x"}, []string{"validFrom", "validTo"}},
		{"kampanye anggaran negatif", domain.KindCampaign, domain.DimensionInput{Code: "KMP-202701-AB", Name: "x", ValidFrom: jan1, ValidTo: jan31, Budget: &negative, Description: strings.Repeat("a", 501)}, []string{"budgetAmount", "description"}},
		{"nama kosong", domain.KindDivision, domain.DimensionInput{Code: "OPS"}, []string{"name"}},
		{"nama panjang", domain.KindDivision, domain.DimensionInput{Code: "OPS", Name: strings.Repeat("a", 151)}, []string{"name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := domain.NewDimension(tt.kind, tt.in)
			if tt.fields == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.kind, v.Kind)
				assert.NotEmpty(t, tt.kind.Label())
				return
			}
			fields := apperr.As(err).Fields
			for _, f := range tt.fields {
				assert.Contains(t, fields, f)
			}
		})
	}

	ch, err := domain.NewDimension(domain.KindChannel, domain.DimensionInput{Code: " Modern_Trade2 ", Name: "MT"})
	require.NoError(t, err)
	assert.Equal(t, "modern_trade2", ch.Code)

	c, err := domain.NewDimension(domain.KindCampaign, domain.DimensionInput{Code: "kmp-202701-ramadan", Name: "Ramadan", ValidFrom: jan1, ValidTo: jan31, IsActive: true})
	require.NoError(t, err)
	assert.Equal(t, "KMP-202701-RAMADAN", c.Code)
	assert.True(t, c.CampaignActiveOn(jan1))
	assert.True(t, c.CampaignActiveOn(jan31))
	assert.False(t, c.CampaignActiveOn(jan31.AddDays(1)))
	require.NoError(t, c.Update(domain.DimensionInput{Code: "ignored", Name: "Ramadan 2027", ValidFrom: jan1, ValidTo: jan31.AddDays(10)}))
	assert.Equal(t, "KMP-202701-RAMADAN", c.Code, "kode tidak berubah")
	assert.False(t, c.IsActive)
	assert.Error(t, c.Update(domain.DimensionInput{Name: ""}))

	sub, err := domain.NewDimension(domain.KindSubChannel, domain.DimensionInput{Code: "alfa", Name: "Alfa", ChannelID: ptr(channelID)})
	require.NoError(t, err)
	require.NoError(t, sub.Update(domain.DimensionInput{Name: "Alfamart", ChannelID: ptr(uuid.New())}))
	assert.Equal(t, &channelID, sub.ChannelID, "sub-channel tidak pindah channel")

	assert.NoError(t, ch.CheckDelete(false, 0))
	assert.True(t, apperr.HasCode(ch.CheckDelete(true, 0), apperr.CodeInvalidStateTransition))
	assert.True(t, apperr.HasCode(ch.CheckDelete(false, 1), apperr.CodeInvalidStateTransition))
	assert.Equal(t, "x", domain.DimensionKind("x").Label())
}

func TestPeriods(t *testing.T) {
	k := domain.PeriodOf(clock.NewBusinessDate(2026, time.December, 15))
	assert.Equal(t, "Des 2026", k.Label())
	assert.Equal(t, domain.PeriodKey{Year: 2027, Month: time.January}, k.Next())
	assert.Equal(t, domain.PeriodKey{Year: 2026, Month: time.November}, domain.PeriodKey{Year: 2026, Month: time.October}.Next())
	assert.Less(t, k.Index(), k.Next().Index())
	assert.Equal(t, "13/2026", domain.PeriodKey{Year: 2026, Month: 13}.Label())

	p := domain.Period{Year: 2026, Month: time.February, Status: domain.PeriodOpen}
	assert.NoError(t, p.CheckOpen())
	assert.Equal(t, "Feb 2026", p.Label())
	assert.Equal(t, "2026-02-01", p.Start().String())
	assert.Equal(t, "2026-02-28", p.End().String())
	p.Status = domain.PeriodClosed
	err := p.CheckOpen()
	assert.True(t, apperr.HasCode(err, apperr.CodePeriodLocked))
	assert.Contains(t, apperr.As(err).Message, "Feb 2026")
}
