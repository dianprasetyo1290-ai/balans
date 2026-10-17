package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
	"balans/internal/tax"
)

// --- PPh potong, bukti potong, penyetoran, PPh Final (S-406, S-407) ---------------------------

// Rule dan nilai bernama jurnal pajak (posting_rule_accounts A-29, A-30).
const (
	RuleTaxDeposit    = "A-29"
	RuleFinalTax      = "A-30"
	AmountTax         = "tax_value"
	SourceTaxDeposit  = "tax_deposit"
	SourceFinalTaxRun = "final_tax_run"
)

// TaxPayableCode mengembalikan akun utang pajak per jenis pajak (2301 – 2304).
func TaxPayableCode(t tax.Type) string {
	if t == tax.FinalUMKM {
		return FinalTaxPayableCode
	}
	return WithholdingAccountCode(WithholdingType(t))
}

// TaxTypeOf memetakan jenis PPh jenis biaya/pemasok ke jenis pajak kalkulator ("" untuk none).
func TaxTypeOf(w WithholdingType) tax.Type {
	switch w {
	case WithholdingPPh21:
		return tax.PPh21
	case WithholdingPPh23:
		return tax.PPh23
	case WithholdingPPh42:
		return tax.PPh42
	}
	return ""
}

// RateVersion adalah satu baris withholding_tax_rates.
type RateVersion struct {
	ID uuid.UUID
	tax.Rate
	IsDefault bool
	Note      string
	UpdatedAt time.Time
	Version   int
}

// NewRateVersion memvalidasi versi tarif baru.
func NewRateVersion(r tax.Rate, isDefault bool, note string) (RateVersion, error) {
	r.ObjectCode, r.Name = strings.TrimSpace(r.ObjectCode), strings.TrimSpace(r.Name)
	if err := r.Validate(); err != nil {
		return RateVersion{}, err
	}
	note = strings.TrimSpace(note)
	if len(note) > 300 {
		return RateVersion{}, apperr.Validation(map[string]string{"note": "Catatan maks. 300 karakter"})
	}
	return RateVersion{Rate: r, IsDefault: isDefault, Note: note}, nil
}

// SupersededBy mengembalikan tanggal akhir versi lama saat versi baru berlaku mulai from (sehari sebelumnya),
// atau error bila versi baru tidak setelah versi lama dimulai.
func (v RateVersion) SupersededBy(from clock.BusinessDate) (clock.BusinessDate, error) {
	if !from.After(v.ValidFrom) {
		return clock.BusinessDate{}, apperr.Validation(map[string]string{
			"validFrom": "Tanggal berlaku harus setelah " + v.ValidFrom.String() + " (versi tarif sebelumnya)",
		})
	}
	return from.AddDays(-1), nil
}

// DefaultObject memilih kode objek default jenis pajak yang berlaku pada tanggal tertentu.
func DefaultObject(rates []RateVersion, t tax.Type, on clock.BusinessDate) (string, bool) {
	fallback := ""
	for _, r := range rates {
		if r.Type != t || !r.EffectiveOn(on) {
			continue
		}
		if r.IsDefault {
			return r.ObjectCode, true
		}
		if fallback == "" {
			fallback = r.ObjectCode
		}
	}
	return fallback, fallback != ""
}

// FindRate mencari versi tarif yang berlaku beserta ID-nya (tax.SelectRate).
func FindRate(rates []RateVersion, t tax.Type, objectCode string, on clock.BusinessDate) (RateVersion, error) {
	plain := make([]tax.Rate, len(rates))
	for i, r := range rates {
		plain[i] = r.Rate
	}
	sel, err := tax.SelectRate(plain, t, objectCode, on)
	if err != nil {
		return RateVersion{}, err
	}
	for _, r := range rates {
		if r.Type == sel.Type && r.ObjectCode == sel.ObjectCode && r.ValidFrom == sel.ValidFrom {
			return r, nil
		}
	}
	return RateVersion{}, apperr.New(apperr.CodeNotFound)
}

// WithholdingStatus adalah status data bukti potong.
type WithholdingStatus string

const (
	WithholdingRecorded  WithholdingStatus = "recorded"
	WithholdingDeposited WithholdingStatus = "deposited"
	WithholdingCancelled WithholdingStatus = "cancelled"
)

// WithholdingRecord adalah data bukti potong satu dokumen sumber per jenis + objek pajak.
type WithholdingRecord struct {
	ID            uuid.UUID
	SourceType    string
	SourceID      uuid.UUID
	SourceLabel   string
	BusinessDate  clock.BusinessDate
	Period        tax.Period
	TaxType       tax.Type
	ObjectCode    string
	ObjectName    string
	RateID        uuid.UUID
	TaxpayerType  string
	TaxpayerID    *uuid.UUID
	TaxpayerName  string
	TaxpayerTaxID string
	HasTaxID      bool
	Gross         money.Amount
	TaxableBase   money.Amount
	RatePercent   decimal.Decimal
	TaxAmount     money.Amount
	Status        WithholdingStatus
	DepositID     *uuid.UUID
	DepositNumber string
	SlipNumber    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Version       int
}

// WithholdingSubject adalah dokumen dan penerima penghasilan yang dipotong.
type WithholdingSubject struct {
	SourceType    string
	SourceID      uuid.UUID
	SourceLabel   string
	BusinessDate  clock.BusinessDate
	TaxpayerType  string
	TaxpayerID    *uuid.UUID
	TaxpayerName  string
	TaxpayerTaxID string
}

// Validate memeriksa dokumen dan penerima.
func (s WithholdingSubject) Validate() error {
	fields := map[string]string{}
	if strings.TrimSpace(s.SourceType) == "" || s.SourceID == uuid.Nil {
		fields["sourceId"] = "Dokumen sumber wajib diisi"
	}
	if s.BusinessDate.IsZero() {
		fields["businessDate"] = "Tanggal dokumen wajib diisi"
	}
	if strings.TrimSpace(s.TaxpayerName) == "" {
		fields["taxpayerName"] = "Nama penerima penghasilan wajib diisi"
	}
	if id := digitsOnly(s.TaxpayerTaxID); s.TaxpayerTaxID != "" && len(id) != 15 && len(id) != 16 {
		fields["taxpayerTaxId"] = "NPWP/NIK 15 atau 16 angka"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// NewWithholdingRecord menyusun data bukti potong dari hasil kalkulator.
func NewWithholdingRecord(s WithholdingSubject, rate RateVersion, w tax.Withholding) WithholdingRecord {
	taxID := digitsOnly(s.TaxpayerTaxID)
	return WithholdingRecord{
		SourceType: s.SourceType, SourceID: s.SourceID, SourceLabel: s.SourceLabel, BusinessDate: s.BusinessDate,
		Period: tax.PeriodOf(s.BusinessDate), TaxType: w.Type, ObjectCode: w.ObjectCode, ObjectName: rate.Name,
		RateID: rate.ID, TaxpayerType: s.TaxpayerType, TaxpayerID: s.TaxpayerID, TaxpayerName: strings.TrimSpace(s.TaxpayerName),
		TaxpayerTaxID: taxID, HasTaxID: taxID != "", Gross: w.Gross, TaxableBase: w.TaxableBase,
		RatePercent: w.RatePercent, TaxAmount: w.Amount, Status: WithholdingRecorded,
	}
}

// Cancel membatalkan data bukti potong yang belum disetor (dokumen sumber dibatalkan).
func (w *WithholdingRecord) Cancel() error {
	if w.Status == WithholdingDeposited {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"PPh "+w.SourceLabel+" sudah disetor; batalkan lewat pembetulan SPT, bukan dari dokumen.")
	}
	w.Status = WithholdingCancelled
	return nil
}

// SetSlipNumber mencatat nomor bukti potong dari Coretax.
func (w *WithholdingRecord) SetSlipNumber(n string) error {
	n = strings.TrimSpace(n)
	if len(n) > 50 {
		return apperr.Validation(map[string]string{"slipNumber": "Nomor bukti potong maks. 50 karakter"})
	}
	if w.Status == WithholdingCancelled {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Bukti potong sudah dibatalkan.")
	}
	w.SlipNumber = n
	return nil
}

// TaxDeposit adalah penyetoran pajak satu jenis satu masa (A-29).
type TaxDeposit struct {
	ID              uuid.UUID
	Number          string
	TaxType         tax.Type
	Period          tax.Period
	BankAccountID   uuid.UUID
	BankAccountName string
	DepositDate     clock.BusinessDate
	Amount          money.Amount
	BillingCode     string
	NTPN            string
	JournalEntryID  uuid.UUID
	JournalNumber   string
	CreatedAt       time.Time
	CreatedBy       uuid.UUID
}

// TaxDepositInput adalah isian Setor Pajak.
type TaxDepositInput struct {
	TaxType       tax.Type
	Period        tax.Period
	BankAccountID uuid.UUID
	DepositDate   clock.BusinessDate
	BillingCode   string
	NTPN          string
}

// Validate memeriksa isian penyetoran. Masa pajak tidak boleh setelah bulan tanggal setor.
func (in TaxDepositInput) Validate() error {
	fields := map[string]string{}
	if !in.TaxType.Valid() {
		fields["taxType"] = "Jenis pajak tidak dikenal"
	}
	if err := in.Period.Validate(); err != nil {
		fields["period"] = "Masa pajak tidak valid"
	}
	if in.BankAccountID == uuid.Nil {
		fields["bankAccountId"] = "Rekening wajib dipilih"
	}
	if in.DepositDate.IsZero() {
		fields["depositDate"] = "Tanggal setor wajib diisi"
	} else if tax.PeriodOf(in.DepositDate).Before(in.Period) {
		fields["depositDate"] = "Tanggal setor tidak boleh sebelum masa pajak"
	}
	if len(strings.TrimSpace(in.BillingCode)) > 30 {
		fields["billingCode"] = "Kode billing maks. 30 karakter"
	}
	if len(strings.TrimSpace(in.NTPN)) > 30 {
		fields["ntpn"] = "NTPN maks. 30 karakter"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// FinalTaxSetting adalah status PPh Final UMKM yang berlaku mulai ValidFrom.
type FinalTaxSetting struct {
	ID              uuid.UUID
	ValidFrom       clock.BusinessDate
	IsEnabled       bool
	OpeningYear     *int
	OpeningTurnover money.Amount
	Note            string
	CreatedAt       time.Time
	CreatedBy       uuid.UUID
}

// Validate memeriksa isian status PPh Final.
func (s FinalTaxSetting) Validate() error {
	fields := map[string]string{}
	if s.ValidFrom.IsZero() {
		fields["validFrom"] = "Tanggal berlaku wajib diisi"
	}
	if s.OpeningTurnover.IsNegative() || !s.OpeningTurnover.Equal(s.OpeningTurnover.Round()) {
		fields["openingTurnover"] = "Omzet awal tidak boleh negatif (maks. 2 desimal)"
	}
	if s.OpeningYear != nil && (*s.OpeningYear < 2000 || *s.OpeningYear > 9999) {
		fields["openingYear"] = "Tahun omzet awal tidak valid"
	}
	if len(strings.TrimSpace(s.Note)) > 300 {
		fields["note"] = "Catatan maks. 300 karakter"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

// OpeningFor mengembalikan omzet awal (cut-over) bila berlaku untuk tahun tersebut.
func (s FinalTaxSetting) OpeningFor(year int) money.Amount {
	if s.OpeningYear != nil && *s.OpeningYear == year {
		return s.OpeningTurnover
	}
	return money.Zero
}

// FinalTaxRun adalah perhitungan PPh Final UMKM satu masa.
type FinalTaxRun struct {
	ID             uuid.UUID
	Period         tax.Period
	GrossRevenue   money.Amount
	RateID         uuid.UUID
	RatePercent    decimal.Decimal
	TaxAmount      money.Amount
	JournalEntryID *uuid.UUID
	JournalNumber  string
	DepositID      *uuid.UUID
	CreatedAt      time.Time
	CreatedBy      uuid.UUID
}

// FinalTaxMonth adalah ringkasan satu bulan di pemantauan PPh Final.
type FinalTaxMonth struct {
	Period       tax.Period
	GrossRevenue money.Amount
	Cumulative   money.Amount
	Run          *FinalTaxRun
}

// CheckFinalTaxRun memeriksa masa yang akan dihitung: PPh Final aktif dan masa sudah berakhir.
func CheckFinalTaxRun(setting *FinalTaxSetting, p tax.Period, today clock.BusinessDate) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if setting == nil || !setting.IsEnabled {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition,
			"PPh Final UMKM tidak aktif untuk masa "+p.String()+"; aktifkan di pengaturan PPh Final.")
	}
	if !p.Before(tax.PeriodOf(today)) {
		return apperr.Validation(map[string]string{"period": "Masa " + p.String() + " belum berakhir"})
	}
	return nil
}
