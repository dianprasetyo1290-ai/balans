package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// Permission aset tetap (Desain Role & Approval 3.4): Finance RWP, Head F&A R + A.
const (
	PermFixedAssetRead  = "accounting.fixed_asset.read"
	PermFixedAssetWrite = "accounting.fixed_asset.write"
	PermFixedAssetPost  = "accounting.fixed_asset.post"
)

// Source type jurnal A-31 (Desain Finance & Accounting 2.1).
const (
	SourceFixedAsset             = "fixed_asset"
	SourceFixedAssetDepreciation = "fixed_asset_depreciation"
	SourceFixedAssetDisposal     = "fixed_asset_disposal"
	RuleFixedAsset               = "A-31"
)

// Nilai bernama A-31 (posting_rule_accounts).
const (
	AmountAcquisition         = "acquisition_value"
	AmountDepreciation        = "depreciation_value"
	AmountDisposalAccumulated = "disposal_accumulated"
	AmountDisposalProceeds    = "disposal_proceeds"
	AmountDisposalLoss        = "disposal_loss"
	AmountDisposalCost        = "disposal_cost"
	AmountDisposalGain        = "disposal_gain"
)

// MaxUsefulLifeMonths membatasi umur manfaat (50 tahun).
const MaxUsefulLifeMonths = 600

var (
	assetClassCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,49}$`)
	assetCodePattern      = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._/-]{0,39}$`)
)

// AssetClass adalah kelas aset tetap: umur manfaat default dan akun aset/akumulasi (A-31).
type AssetClass struct {
	ID                     uuid.UUID
	Code                   string
	Name                   string
	UsefulLifeMonths       int
	AssetAccountID         uuid.UUID
	AssetAccountCode       string
	AccumulatedAccountID   uuid.UUID
	AccumulatedAccountCode string
	IsActive               bool
	UpdatedAt              time.Time
	Version                int
}

// AssetClassInput adalah isian kelas aset.
type AssetClassInput struct {
	Code                 string
	Name                 string
	UsefulLifeMonths     int
	AssetAccountID       uuid.UUID
	AccumulatedAccountID uuid.UUID
	IsActive             bool
}

func validLife(months int) bool { return months >= 1 && months <= MaxUsefulLifeMonths }

// NewAssetClass memvalidasi isian kelas aset. Akun diperiksa service (harus akun aset yang bisa diposting).
func NewAssetClass(in AssetClassInput) (AssetClass, error) {
	c := AssetClass{Code: strings.TrimSpace(in.Code)}
	fields := map[string]string{}
	if !assetClassCodePattern.MatchString(c.Code) {
		fields["code"] = "Kode huruf kecil, angka, atau garis bawah (2–50 karakter), diawali huruf"
	}
	if err := c.Apply(in); err != nil {
		for k, v := range apperr.As(err).Fields {
			fields[k] = v
		}
	}
	if len(fields) > 0 {
		return AssetClass{}, apperr.Validation(fields)
	}
	return c, nil
}

// Apply mengubah isian kelas aset (kode tidak bisa diubah).
func (c *AssetClass) Apply(in AssetClassInput) error {
	fields := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 {
		fields["name"] = "Nama kelas wajib diisi (maks. 120 karakter)"
	}
	if !validLife(in.UsefulLifeMonths) {
		fields["usefulLifeMonths"] = fmt.Sprintf("Umur manfaat 1–%d bulan", MaxUsefulLifeMonths)
	}
	if in.AssetAccountID == uuid.Nil {
		fields["assetAccountId"] = "Akun aset wajib dipilih"
	}
	switch in.AccumulatedAccountID {
	case uuid.Nil:
		fields["accumulatedAccountId"] = "Akun akumulasi penyusutan wajib dipilih"
	case in.AssetAccountID:
		fields["accumulatedAccountId"] = "Akun akumulasi harus berbeda dengan akun aset"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	c.Name, c.UsefulLifeMonths, c.IsActive = name, in.UsefulLifeMonths, in.IsActive
	c.AssetAccountID, c.AccumulatedAccountID = in.AssetAccountID, in.AccumulatedAccountID
	return nil
}

// AssetStatus adalah status aset tetap (UI: Draft, Aktif, Dilepas).
type AssetStatus string

const (
	AssetDraft    AssetStatus = "draft"
	AssetActive   AssetStatus = "active"
	AssetDisposed AssetStatus = "disposed"
)

// AssetSource adalah asal aset: pembelian setelah go-live atau saldo awal migrasi.
type AssetSource string

const (
	AssetPurchase AssetSource = "purchase"
	AssetOpening  AssetSource = "opening"
)

// FixedAsset adalah satu aset tetap (ACC-21).
type FixedAsset struct {
	ID            uuid.UUID
	Code          string
	Name          string
	ClassID       uuid.UUID
	ClassName     string
	LocationID    *uuid.UUID
	LocationLabel string
	DivisionID    uuid.UUID
	Source        AssetSource
	// AcquisitionDate adalah tanggal perolehan; untuk aset pembelian juga tanggal jurnal perolehan.
	AcquisitionDate clock.BusinessDate
	Cost            money.Amount
	Salvage         money.Amount
	UsefulLife      int
	// DepreciationStart adalah tanggal 1 bulan pertama yang disusutkan sistem.
	DepreciationStart clock.BusinessDate
	// OpeningAccumulated dan OpeningMonths adalah akumulasi & bulan yang sudah disusutkan sebelum cut-over.
	OpeningAccumulated money.Amount
	OpeningMonths      int
	Accumulated        money.Amount
	PaymentAccountID   *uuid.UUID
	Status             AssetStatus
	AcquisitionJournal *uuid.UUID
	DisposalDate       *clock.BusinessDate
	DisposalProceeds   *money.Amount
	DisposalAccountID  *uuid.UUID
	DisposalReason     string
	DisposalJournal    *uuid.UUID
	Note               string
	DeletedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int
}

// FixedAssetInput adalah isian Tambah/Ubah Aset Tetap.
type FixedAssetInput struct {
	Code               string
	Name               string
	ClassID            uuid.UUID
	LocationID         *uuid.UUID
	LocationLabel      string
	DivisionID         uuid.UUID
	Source             AssetSource
	AcquisitionDate    clock.BusinessDate
	Cost               money.Amount
	Salvage            money.Amount
	UsefulLife         int
	OpeningAccumulated money.Amount
	PaymentAccountID   *uuid.UUID
	Note               string
}

func firstOfMonth(d clock.BusinessDate) clock.BusinessDate {
	return clock.NewBusinessDate(d.Year(), d.Month(), 1)
}

// MonthsBetween menghitung selisih bulan from → to (to − from), mis. Jan → Mar = 2.
func MonthsBetween(from, to PeriodKey) int { return to.Index() - from.Index() }

// NewFixedAsset menyusun aset draft. cutover wajib untuk aset saldo awal: penyusutan sistem mulai bulan
// setelah cut-over dan bulan yang sudah lewat (perolehan s.d. cut-over) dianggap sudah disusutkan.
// Aset pembelian mulai disusutkan pada bulan perolehan.
func NewFixedAsset(in FixedAssetInput, class AssetClass, cutover *clock.BusinessDate) (FixedAsset, error) {
	a := FixedAsset{Code: strings.ToUpper(strings.TrimSpace(in.Code)), Status: AssetDraft}
	fields := map[string]string{}
	if !assetCodePattern.MatchString(a.Code) {
		fields["code"] = "Kode aset huruf besar/angka (maks. 40 karakter), boleh . _ / -"
	}
	if err := a.apply(in, class, cutover); err != nil {
		for k, v := range apperr.As(err).Fields {
			fields[k] = v
		}
	}
	if len(fields) > 0 {
		return FixedAsset{}, apperr.Validation(fields)
	}
	return a, nil
}

// Update mengubah aset draft (kode tidak bisa diubah).
func (a *FixedAsset) Update(in FixedAssetInput, class AssetClass, cutover *clock.BusinessDate) error {
	if a.Status != AssetDraft || a.DeletedAt != nil {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya aset Draft yang bisa diubah.")
	}
	return a.apply(in, class, cutover)
}

func (a *FixedAsset) apply(in FixedAssetInput, class AssetClass, cutover *clock.BusinessDate) error {
	fields := map[string]string{}
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 200 {
		fields["name"] = "Nama aset wajib diisi (maks. 200 karakter)"
	}
	if in.ClassID == uuid.Nil || class.ID != in.ClassID {
		fields["classId"] = "Kelas aset wajib dipilih"
	} else if !class.IsActive && a.ClassID != class.ID {
		fields["classId"] = "Kelas aset tidak aktif"
	}
	if in.DivisionID == uuid.Nil {
		fields["divisionId"] = "Divisi wajib dipilih (dimensi jurnal A-31)"
	}
	if in.AcquisitionDate.IsZero() {
		fields["acquisitionDate"] = "Tanggal perolehan wajib diisi"
	}
	if !in.Cost.IsPositive() || !isWholeCents(in.Cost) {
		fields["cost"] = "Harga perolehan harus lebih dari 0 (maks. 2 desimal)"
	}
	if in.Salvage.IsNegative() || !isWholeCents(in.Salvage) || (in.Cost.IsPositive() && in.Salvage.Cmp(in.Cost) >= 0) {
		fields["salvage"] = "Nilai sisa 0 atau lebih dan lebih kecil dari harga perolehan"
	}
	life := in.UsefulLife
	if life == 0 {
		life = class.UsefulLifeMonths
	}
	if !validLife(life) {
		fields["usefulLife"] = fmt.Sprintf("Umur manfaat 1–%d bulan", MaxUsefulLifeMonths)
	}
	if len(in.LocationLabel) > 120 {
		fields["locationLabel"] = "Lokasi paling panjang 120 karakter"
	}
	if len(in.Note) > 1000 {
		fields["note"] = "Catatan paling panjang 1000 karakter"
	}
	var start clock.BusinessDate
	openingMonths := 0
	switch in.Source {
	case AssetPurchase:
		if !in.OpeningAccumulated.IsZero() {
			fields["openingAccumulated"] = "Akumulasi saldo awal hanya untuk aset saldo awal"
		}
		if in.PaymentAccountID == nil {
			fields["paymentAccountId"] = "Akun pembayaran (Kas/Bank atau Utang Usaha) wajib dipilih"
		}
		if !in.AcquisitionDate.IsZero() {
			start = firstOfMonth(in.AcquisitionDate)
		}
	case AssetOpening:
		if cutover == nil || cutover.IsZero() {
			fields["source"] = "Tanggal cut-over saldo awal belum diisi di Saldo Awal"
			break
		}
		if !in.AcquisitionDate.IsZero() && in.AcquisitionDate.After(*cutover) {
			fields["acquisitionDate"] = "Tanggal perolehan aset saldo awal tidak boleh setelah tanggal cut-over " + cutover.String()
		}
		depreciable := in.Cost.Sub(in.Salvage)
		if in.OpeningAccumulated.IsNegative() || !isWholeCents(in.OpeningAccumulated) ||
			(depreciable.IsPositive() && in.OpeningAccumulated.Cmp(depreciable) > 0) {
			fields["openingAccumulated"] = "Akumulasi penyusutan 0 s.d. harga perolehan dikurangi nilai sisa"
		}
		next := PeriodOf(*cutover).Next()
		start = clock.NewBusinessDate(next.Year, next.Month, 1)
		if !in.AcquisitionDate.IsZero() {
			openingMonths = min(max(MonthsBetween(PeriodOf(in.AcquisitionDate), PeriodOf(*cutover))+1, 0), life)
		}
	default:
		fields["source"] = "Asal aset tidak dikenal"
	}
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	a.Name, a.ClassID, a.ClassName, a.LocationID, a.LocationLabel = name, class.ID, class.Name, in.LocationID, strings.TrimSpace(in.LocationLabel)
	a.DivisionID, a.Source, a.AcquisitionDate, a.Cost, a.Salvage = in.DivisionID, in.Source, in.AcquisitionDate, in.Cost, in.Salvage
	a.UsefulLife, a.DepreciationStart, a.OpeningMonths = life, start, openingMonths
	a.OpeningAccumulated, a.Accumulated, a.Note = in.OpeningAccumulated, in.OpeningAccumulated, strings.TrimSpace(in.Note)
	a.PaymentAccountID = nil
	if in.Source == AssetPurchase {
		a.PaymentAccountID = in.PaymentAccountID
	}
	return nil
}

// Depreciable adalah nilai yang disusutkan: harga perolehan − nilai sisa.
func (a FixedAsset) Depreciable() money.Amount { return a.Cost.Sub(a.Salvage) }

// BookValue adalah nilai buku: harga perolehan − akumulasi.
func (a FixedAsset) BookValue() money.Amount { return a.Cost.Sub(a.Accumulated) }

// MonthlyDepreciation adalah penyusutan garis lurus per bulan dari nilai yang disusutkan (tampilan).
func (a FixedAsset) MonthlyDepreciation() money.Amount {
	if a.UsefulLife <= 0 {
		return money.Zero
	}
	return a.Depreciable().Mul(decimal.NewFromInt(1).Div(decimal.NewFromInt(int64(a.UsefulLife)))).Round()
}

// DepreciationFor menghitung penyusutan garis lurus periode k (A-31): sisa nilai yang belum disusutkan dibagi
// sisa umur bulan, dibulatkan ke sen; bulan terakhir mengambil seluruh sisa agar akumulasi tepat sama dengan
// nilai yang disusutkan. Aset yang belum aktif, dilepas, belum mulai disusutkan, atau sudah habis = 0.
func (a FixedAsset) DepreciationFor(k PeriodKey) money.Amount {
	if a.Status != AssetActive || a.DepreciationStart.IsZero() {
		return money.Zero
	}
	start := PeriodOf(a.DepreciationStart)
	if k.Index() < start.Index() {
		return money.Zero
	}
	remaining := a.Depreciable().Sub(a.Accumulated)
	if !remaining.IsPositive() {
		return money.Zero
	}
	used := a.OpeningMonths + MonthsBetween(start, k)
	left := a.UsefulLife - used
	if left <= 1 {
		return remaining
	}
	amount := money.FromDecimal(remaining.Decimal().Div(decimal.NewFromInt(int64(left)))).Round()
	if amount.Cmp(remaining) > 0 {
		return remaining
	}
	return amount
}

// RecordDepreciation menambah akumulasi setelah jurnal penyusutan diposting.
func (a *FixedAsset) RecordDepreciation(amount money.Amount) {
	a.Accumulated = a.Accumulated.Add(amount)
}

// Activate mengaktifkan aset draft. Aset pembelian dijurnal A-31 perolehan oleh service.
func (a *FixedAsset) Activate() error {
	if a.DeletedAt != nil || a.Status != AssetDraft {
		return apperr.InvalidStateTransition("fixed_asset", string(a.Status), string(AssetActive))
	}
	a.Status = AssetActive
	return nil
}

// Disposal adalah hasil perhitungan pelepasan aset (A-31): akumulasi + hasil + rugi = perolehan + laba.
type Disposal struct {
	Accumulated money.Amount
	Cost        money.Amount
	Proceeds    money.Amount
	Loss        money.Amount
	Gain        money.Amount
}

// Amounts adalah nilai bernama untuk PostingRequest A-31.
func (d Disposal) Amounts() map[string]money.Amount {
	return map[string]money.Amount{
		AmountDisposalAccumulated: d.Accumulated, AmountDisposalCost: d.Cost, AmountDisposalProceeds: d.Proceeds,
		AmountDisposalLoss: d.Loss, AmountDisposalGain: d.Gain,
	}
}

// Dispose melepas aset aktif (dijual/dihapusbukukan) pada tanggal tertentu. Hasil penjualan 0 = dihapusbukukan.
func (a *FixedAsset) Dispose(date clock.BusinessDate, proceeds money.Amount, accountID *uuid.UUID, reason string) (Disposal, error) {
	if a.Status != AssetActive {
		return Disposal{}, apperr.InvalidStateTransition("fixed_asset", string(a.Status), string(AssetDisposed))
	}
	fields := map[string]string{}
	reason = strings.TrimSpace(reason)
	if date.IsZero() {
		fields["disposalDate"] = "Tanggal pelepasan wajib diisi"
	} else if date.Before(a.AcquisitionDate) {
		fields["disposalDate"] = "Tanggal pelepasan tidak boleh sebelum tanggal perolehan"
	}
	if proceeds.IsNegative() || !isWholeCents(proceeds) {
		fields["proceeds"] = "Hasil pelepasan 0 atau lebih (maks. 2 desimal)"
	}
	if proceeds.IsPositive() && accountID == nil {
		fields["disposalAccountId"] = "Akun penerimaan wajib dipilih bila ada hasil pelepasan"
	}
	if reason == "" || len(reason) > 500 {
		fields["reason"] = "Alasan pelepasan wajib diisi (maks. 500 karakter)"
	}
	if len(fields) > 0 {
		return Disposal{}, apperr.Validation(fields)
	}
	d := Disposal{Accumulated: a.Accumulated, Cost: a.Cost, Proceeds: proceeds, Loss: money.Zero, Gain: money.Zero}
	diff := a.BookValue().Sub(proceeds)
	if diff.IsPositive() {
		d.Loss = diff
	} else {
		d.Gain = diff.Neg()
	}
	a.Status, a.DisposalDate, a.DisposalProceeds, a.DisposalReason = AssetDisposed, &date, &proceeds, reason
	a.DisposalAccountID = nil
	if proceeds.IsPositive() {
		a.DisposalAccountID = accountID
	}
	return d, nil
}

// AssetAction adalah aksi yang boleh dilakukan user pada aset.
type AssetAction string

const (
	AssetActionEdit     AssetAction = "edit"
	AssetActionDelete   AssetAction = "delete"
	AssetActionActivate AssetAction = "activate"
	AssetActionDispose  AssetAction = "dispose"
)

// AllowedActions untuk tombol di ERP.
func (a FixedAsset) AllowedActions(has func(string) bool) []AssetAction {
	out := []AssetAction{}
	if a.DeletedAt != nil {
		return out
	}
	switch a.Status {
	case AssetDraft:
		if has(PermFixedAssetWrite) {
			out = append(out, AssetActionEdit, AssetActionDelete)
		}
		if has(PermFixedAssetPost) {
			out = append(out, AssetActionActivate)
		}
	case AssetActive:
		if has(PermFixedAssetPost) {
			out = append(out, AssetActionDispose)
		}
	}
	return out
}

// DepreciationLine adalah penyusutan satu aset pada satu periode.
type DepreciationLine struct {
	ID             uuid.UUID
	RunID          uuid.UUID
	AssetID        uuid.UUID
	AssetCode      string
	AssetName      string
	PeriodID       uuid.UUID
	Period         PeriodKey
	Amount         money.Amount
	Accumulated    money.Amount
	BookValue      money.Amount
	JournalEntryID uuid.UUID
	JournalNumber  string
	CreatedAt      time.Time
}

// DepreciationRun adalah ringkasan penyusutan satu periode.
type DepreciationRun struct {
	ID         uuid.UUID
	PeriodID   uuid.UUID
	Period     PeriodKey
	AssetCount int
	Total      money.Amount
	RunCount   int
	LastRunAt  time.Time
	CreatedAt  time.Time
	Version    int
}
