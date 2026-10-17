package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"balans/internal/accounting/domain"
	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/tax"
)

var jan1 = clock.NewBusinessDate(2026, time.January, 1)

func rateVersion(t tax.Type, object string, rate string, from clock.BusinessDate, isDefault bool) domain.RateVersion {
	return domain.RateVersion{
		ID: uuid.New(), IsDefault: isDefault,
		Rate: tax.Rate{Type: t, ObjectCode: object, Name: object, RatePercent: decimal.RequireFromString(rate), ValidFrom: from},
	}
}

func TestTaxMapping(t *testing.T) {
	assert.Equal(t, "2301", domain.TaxPayableCode(tax.PPh21))
	assert.Equal(t, "2302", domain.TaxPayableCode(tax.PPh23))
	assert.Equal(t, "2303", domain.TaxPayableCode(tax.PPh42))
	assert.Equal(t, "2304", domain.TaxPayableCode(tax.FinalUMKM))
	assert.Equal(t, tax.PPh21, domain.TaxTypeOf(domain.WithholdingPPh21))
	assert.Equal(t, tax.PPh23, domain.TaxTypeOf(domain.WithholdingPPh23))
	assert.Equal(t, tax.PPh42, domain.TaxTypeOf(domain.WithholdingPPh42))
	assert.Equal(t, tax.Type(""), domain.TaxTypeOf(domain.WithholdingNone))
}

func TestRateVersions(t *testing.T) {
	v, err := domain.NewRateVersion(tax.Rate{
		Type: tax.PPh23, ObjectCode: " 24-104-14 ", Name: " Jasa ", RatePercent: decimal.NewFromInt(2), ValidFrom: jan1,
	}, true, " catatan ")
	require.NoError(t, err)
	assert.Equal(t, "24-104-14", v.ObjectCode)
	assert.Equal(t, "catatan", v.Note)
	_, err = domain.NewRateVersion(tax.Rate{Type: tax.PPh23}, false, "")
	assert.Error(t, err)
	_, err = domain.NewRateVersion(v.Rate, false, string(make([]byte, 301)))
	assert.Contains(t, fieldsOf(t, err), "note")

	end, err := v.SupersededBy(clock.NewBusinessDate(2026, time.July, 1))
	require.NoError(t, err)
	assert.Equal(t, "2026-06-30", end.String())
	_, err = v.SupersededBy(jan1)
	assert.Contains(t, fieldsOf(t, err), "validFrom")

	rates := []domain.RateVersion{
		rateVersion(tax.PPh23, "A", "2", jan1, false),
		rateVersion(tax.PPh23, "B", "3", jan1, true),
		rateVersion(tax.PPh42, "C", "10", jan1, false),
	}
	code, ok := domain.DefaultObject(rates, tax.PPh23, oct20)
	assert.True(t, ok)
	assert.Equal(t, "B", code)
	code, ok = domain.DefaultObject(rates, tax.PPh42, oct20)
	assert.True(t, ok)
	assert.Equal(t, "C", code, "tanpa default → objek pertama")
	_, ok = domain.DefaultObject(rates, tax.PPh21, oct20)
	assert.False(t, ok)

	found, err := domain.FindRate(rates, tax.PPh23, "A", oct20)
	require.NoError(t, err)
	assert.Equal(t, rates[0].ID, found.ID)
	_, err = domain.FindRate(rates, tax.PPh23, "Z", oct20)
	assert.Error(t, err)
}

func TestWithholdingRecord(t *testing.T) {
	subject := domain.WithholdingSubject{
		SourceType: "expense_claim", SourceID: uuid.New(), SourceLabel: "EXP/1", BusinessDate: oct20,
		TaxpayerType: "other", TaxpayerName: " Agensi ", TaxpayerTaxID: "01.234.567.8-901.000",
	}
	require.NoError(t, subject.Validate())
	rate := rateVersion(tax.PPh23, "24-104-14", "2", jan1, true)
	w, err := rate.Withhold(tax.WithholdingInput{Gross: rp(1_000_000), HasTaxID: true})
	require.NoError(t, err)
	rec := domain.NewWithholdingRecord(subject, rate, w)
	assert.Equal(t, "Agensi", rec.TaxpayerName)
	assert.Equal(t, "012345678901000", rec.TaxpayerTaxID)
	assert.True(t, rec.HasTaxID)
	assert.Equal(t, tax.Period{Year: 2026, Month: time.October}, rec.Period)
	assert.Equal(t, "20000.00", rec.TaxAmount.String())
	assert.Equal(t, rate.ID, rec.RateID)
	assert.Equal(t, domain.WithholdingRecorded, rec.Status)

	require.NoError(t, rec.SetSlipNumber(" BP-001 "))
	assert.Equal(t, "BP-001", rec.SlipNumber)
	assert.Contains(t, fieldsOf(t, rec.SetSlipNumber(string(make([]byte, 51)))), "slipNumber")
	rec.Status = domain.WithholdingDeposited
	assert.True(t, apperr.HasCode(rec.Cancel(), apperr.CodeInvalidStateTransition))
	rec.Status = domain.WithholdingRecorded
	require.NoError(t, rec.Cancel())
	assert.Equal(t, domain.WithholdingCancelled, rec.Status)
	assert.Error(t, rec.SetSlipNumber("x"))

	bad := domain.WithholdingSubject{TaxpayerTaxID: "123"}
	assert.ElementsMatch(t, []string{"sourceId", "businessDate", "taxpayerName", "taxpayerTaxId"}, fieldsOf(t, bad.Validate()))
}

func TestTaxDepositInput(t *testing.T) {
	in := domain.TaxDepositInput{
		TaxType: tax.PPh23, Period: tax.Period{Year: 2026, Month: time.September}, BankAccountID: uuid.New(), DepositDate: oct20,
	}
	require.NoError(t, in.Validate())
	bad := domain.TaxDepositInput{
		TaxType: "ppn", Period: tax.Period{Year: 2026, Month: 13}, DepositDate: clock.NewBusinessDate(2026, time.August, 1),
		BillingCode: string(make([]byte, 31)), NTPN: string(make([]byte, 31)),
	}
	assert.ElementsMatch(t, []string{"taxType", "period", "bankAccountId", "depositDate", "billingCode", "ntpn"}, fieldsOf(t, bad.Validate()))
	early := in
	early.DepositDate = clock.NewBusinessDate(2026, time.August, 31)
	assert.Contains(t, fieldsOf(t, early.Validate()), "depositDate")
	early.DepositDate = clock.BusinessDate{}
	assert.Contains(t, fieldsOf(t, early.Validate()), "depositDate")
}

func TestFinalTaxSetting(t *testing.T) {
	year := 2026
	s := domain.FinalTaxSetting{ValidFrom: jan1, IsEnabled: true, OpeningYear: &year, OpeningTurnover: rp(1_000_000_000)}
	require.NoError(t, s.Validate())
	assert.Equal(t, rp(1_000_000_000), s.OpeningFor(2026))
	assert.True(t, s.OpeningFor(2027).IsZero())

	badYear := 1999
	bad := domain.FinalTaxSetting{OpeningYear: &badYear, OpeningTurnover: money.MustParse("-1"), Note: string(make([]byte, 301))}
	assert.ElementsMatch(t, []string{"validFrom", "openingTurnover", "openingYear", "note"}, fieldsOf(t, bad.Validate()))

	sep := tax.Period{Year: 2026, Month: time.September}
	require.NoError(t, domain.CheckFinalTaxRun(&s, sep, oct20))
	assert.Contains(t, fieldsOf(t, domain.CheckFinalTaxRun(&s, tax.Period{Year: 2026, Month: time.October}, oct20)), "period")
	assert.True(t, apperr.HasCode(domain.CheckFinalTaxRun(nil, sep, oct20), apperr.CodeInvalidStateTransition))
	off := s
	off.IsEnabled = false
	assert.True(t, apperr.HasCode(domain.CheckFinalTaxRun(&off, sep, oct20), apperr.CodeInvalidStateTransition))
	assert.Error(t, domain.CheckFinalTaxRun(&s, tax.Period{Year: 2026, Month: 0}, oct20))
}
