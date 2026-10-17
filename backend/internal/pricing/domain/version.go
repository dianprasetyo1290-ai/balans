package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"balans/internal/platform/apperr"
	"balans/internal/platform/clock"
	"balans/internal/platform/money"
)

// VersionStatus adalah status versi daftar harga.
type VersionStatus string

const (
	VersionDraft           VersionStatus = "draft"
	VersionPendingApproval VersionStatus = "pending_approval"
	VersionActive          VersionStatus = "active"
	VersionRejected        VersionStatus = "rejected"
	VersionCancelled       VersionStatus = "cancelled"
)

// MaxVersionItems membatasi jumlah produk per versi (batas teknis satu request).
const MaxVersionItems = 2000

// maxPercentOfReference membatasi persen basis percent (mis. 90 = 90% dari harga acuan).
var maxPercentOfReference = decimal.NewFromInt(1000)

// PriceListItem adalah harga satu produk di satu versi.
type PriceListItem struct {
	ProductID uuid.UUID
	Price     money.Amount
	// MaxRetailPrice adalah HET; hanya di daftar Harga Dasar & HET.
	MaxRetailPrice *money.Amount
}

// PriceListVersion adalah versi berlaku daftar harga (effective-dated). Versi aktif tidak pernah diubah;
// perubahan harga selalu lewat versi baru yang disetujui (approval price_list_version).
type PriceListVersion struct {
	ID          uuid.UUID
	PriceListID uuid.UUID
	No          int
	Period
	Status VersionStatus
	// Percent dipakai daftar basis percent.
	Percent     *decimal.Decimal
	Notes       string
	SubmittedAt *time.Time
	SubmittedBy *uuid.UUID
	DecidedAt   *time.Time
	DecidedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
	// ItemCount adalah jumlah produk (diisi list tanpa memuat Items).
	ItemCount int
	Items     []PriceListItem
}

// ItemInput adalah isian satu baris harga. Nominal berupa string desimal.
type ItemInput struct {
	ProductID      uuid.UUID
	Price          string
	MaxRetailPrice string
}

// VersionInput adalah isian versi draft.
type VersionInput struct {
	ValidFrom clock.BusinessDate
	ValidTo   *clock.BusinessDate
	Percent   string
	Notes     string
	Items     []ItemInput
}

// NewVersion menyiapkan versi draft nomor no untuk daftar harga list.
func NewVersion(list PriceList, no int, in VersionInput, today clock.BusinessDate) (PriceListVersion, error) {
	if err := list.EnsureActive(); err != nil {
		return PriceListVersion{}, err
	}
	v := PriceListVersion{PriceListID: list.ID, No: no, Status: VersionDraft}
	if err := v.apply(list, in, today); err != nil {
		return PriceListVersion{}, err
	}
	return v, nil
}

// Editable: hanya versi draft yang boleh diubah.
func (v PriceListVersion) Editable() bool { return v.Status == VersionDraft }

// Update mengubah versi draft.
func (v *PriceListVersion) Update(list PriceList, in VersionInput, today clock.BusinessDate) error {
	if !v.Editable() {
		return apperr.WithMessage(apperr.CodeInvalidStateTransition, "Hanya versi draft yang bisa diubah; buat versi baru untuk perubahan harga.")
	}
	next := *v
	if err := next.apply(list, in, today); err != nil {
		return err
	}
	*v = next
	return nil
}

func (v *PriceListVersion) apply(list PriceList, in VersionInput, today clock.BusinessDate) error {
	f := fieldErrors{}
	v.Period = Period{ValidFrom: in.ValidFrom, ValidTo: in.ValidTo}
	v.validateNew(f, today)
	v.Notes = f.text("notes", in.Notes, 1000, false, "Catatan")
	v.Percent = nil
	if list.Basis == BasisPercent {
		v.Percent = f.percent("percent", in.Percent, "Persentase dari harga acuan", false, maxPercentOfReference)
		if len(in.Items) > 0 {
			f.add("items", "Daftar harga berbasis persentase tidak berisi harga per produk")
		}
	}
	if len(in.Items) > MaxVersionItems {
		f.add("items", fmt.Sprintf("Paling banyak %d produk per versi", MaxVersionItems))
	}
	v.Items = make([]PriceListItem, 0, len(in.Items))
	seen := map[uuid.UUID]int{}
	for i, it := range in.Items {
		prefix := fmt.Sprintf("items.%d.", i)
		if it.ProductID == uuid.Nil {
			f.add(prefix+"productId", "Pilih produk")
		} else if j, dup := seen[it.ProductID]; dup {
			f.add(prefix+"productId", fmt.Sprintf("Produk sama dengan baris %d", j+1))
		}
		seen[it.ProductID] = i
		item := PriceListItem{ProductID: it.ProductID}
		if p := f.amount(prefix+"priceAmount", it.Price, "Harga", true); p != nil {
			item.Price = *p
		}
		if it.MaxRetailPrice != "" {
			if list.Type != TypeBase {
				f.add(prefix+"maxRetailPriceAmount", "HET hanya diisi di daftar Harga Dasar & HET")
			} else if het := f.amount(prefix+"maxRetailPriceAmount", it.MaxRetailPrice, "HET", false); het != nil {
				if het.Cmp(item.Price) < 0 {
					f.add(prefix+"maxRetailPriceAmount", "HET tidak boleh di bawah harga dasar")
				}
				item.MaxRetailPrice = het
			}
		}
		v.Items = append(v.Items, item)
	}
	v.ItemCount = len(v.Items)
	return f.err()
}

// Submit mengajukan versi draft untuk approval price_list_version.
func (v *PriceListVersion) Submit(list PriceList, today clock.BusinessDate, at time.Time, by uuid.UUID) error {
	if v.Status != VersionDraft {
		return apperr.InvalidStateTransition("price_list_version", string(v.Status), string(VersionPendingApproval))
	}
	if err := list.EnsureActive(); err != nil {
		return err
	}
	f := fieldErrors{}
	v.validateNew(f, today)
	switch {
	case list.Basis == BasisPercent && v.Percent == nil:
		f.add("percent", "Persentase dari harga acuan wajib diisi")
	case list.Basis == BasisFixed && len(v.Items) == 0:
		f.add("items", "Isi harga minimal satu produk sebelum diajukan")
	}
	if err := f.err(); err != nil {
		return err
	}
	v.Status, v.SubmittedAt, v.SubmittedBy = VersionPendingApproval, &at, &by
	return nil
}

// Withdraw menarik kembali pengajuan menjadi draft.
func (v *PriceListVersion) Withdraw() error {
	if v.Status != VersionPendingApproval {
		return apperr.InvalidStateTransition("price_list_version", string(v.Status), string(VersionDraft))
	}
	v.Status, v.SubmittedAt, v.SubmittedBy = VersionDraft, nil, nil
	return nil
}

// Activate memberlakukan versi yang disetujui. Versi yang disetujui setelah tanggal mulainya lewat berlaku mulai
// hari persetujuan, sehingga transaksi sebelum persetujuan tetap memakai versi lama.
func (v *PriceListVersion) Activate(today clock.BusinessDate, at time.Time, by uuid.UUID) error {
	if v.Status != VersionPendingApproval {
		return apperr.InvalidStateTransition("price_list_version", string(v.Status), string(VersionActive))
	}
	if v.ValidFrom.Before(today) {
		v.ValidFrom = today
	}
	if v.ValidTo != nil && v.ValidTo.Before(v.ValidFrom) {
		return apperr.WithMessage(apperr.CodeValidationFailed,
			"Tanggal berakhir versi ini sudah lewat; tolak pengajuan dan buat versi baru.")
	}
	v.Status, v.DecidedAt, v.DecidedBy = VersionActive, &at, &by
	return nil
}

// Reject menandai versi ditolak.
func (v *PriceListVersion) Reject(at time.Time, by uuid.UUID) error {
	if v.Status != VersionPendingApproval {
		return apperr.InvalidStateTransition("price_list_version", string(v.Status), string(VersionRejected))
	}
	v.Status, v.DecidedAt, v.DecidedBy = VersionRejected, &at, &by
	return nil
}

// Cancel membatalkan versi draft.
func (v *PriceListVersion) Cancel() error {
	if v.Status != VersionDraft {
		return apperr.InvalidStateTransition("price_list_version", string(v.Status), string(VersionCancelled))
	}
	v.Status = VersionCancelled
	return nil
}

// Label adalah label dokumen di inbox persetujuan, mis. "HARGA-DASAR v3".
func (v PriceListVersion) Label(list PriceList) string { return fmt.Sprintf("%s v%d", list.Code, v.No) }

// Summary adalah ringkasan pengajuan di inbox persetujuan.
func (v PriceListVersion) Summary(list PriceList) string {
	what := fmt.Sprintf("%d produk", len(v.Items))
	if list.Basis == BasisPercent && v.Percent != nil {
		what = v.Percent.String() + "% dari harga acuan"
	}
	return fmt.Sprintf("%s · %s · berlaku %s", list.Name, what, v.Period.Label())
}

// SupersedeActive menghitung penutupan versi aktif lain saat versi v diberlakukan: versi aktif tanpa tanggal
// berakhir yang mulai sebelum v ditutup sehari sebelum v mulai. Versi aktif lain yang masih beririsan
// menghasilkan VALIDATION_FAILED.
func (v PriceListVersion) SupersedeActive(active []PriceListVersion) ([]PriceListVersion, error) {
	var out []PriceListVersion
	for _, a := range active {
		if a.ID == v.ID || !a.Overlaps(v.Period) {
			continue
		}
		p, ok := a.Supersede(v.ValidFrom)
		if !ok || p.Overlaps(v.Period) {
			return nil, apperr.WithMessage(apperr.CodeValidationFailed, fmt.Sprintf(
				"Periode versi ini beririsan dengan versi aktif v%d (%s). Tolak pengajuan atau ajukan versi dengan tanggal lain.",
				a.No, a.Period.Label()))
		}
		a.Period = p
		out = append(out, a)
	}
	return out, nil
}

// PercentOf menghitung harga basis percent dari harga acuan, dibulatkan ke rupiah penuh.
func PercentOf(reference money.Amount, percent decimal.Decimal) money.Amount {
	return reference.Mul(percent).Mul(decimal.New(1, -2)).RoundRupiah()
}
