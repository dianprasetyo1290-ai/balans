package domain

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"balans/internal/platform/apperr"
)

// ReturnScope adalah dokumen yang memakai alasan retur.
type ReturnScope string

const (
	ScopeSalesReturn    ReturnScope = "sales_return"
	ScopePurchaseReturn ReturnScope = "purchase_return"
	ScopeBoth           ReturnScope = "both"
)

var scopeLabels = map[ReturnScope]string{
	ScopeSalesReturn: "Retur penjualan", ScopePurchaseReturn: "Retur pembelian", ScopeBoth: "Retur penjualan & pembelian",
}

// Label mengembalikan nama cakupan untuk UI.
func (s ReturnScope) Label() string { return scopeLabels[s] }

// Valid memeriksa nilai cakupan.
func (s ReturnScope) Valid() bool {
	_, ok := scopeLabels[s]
	return ok
}

// Covers: alasan dengan cakupan s boleh dipakai dokumen jenis doc (sales_return/purchase_return).
func (s ReturnScope) Covers(doc ReturnScope) bool { return s == ScopeBoth || s == doc }

// Disposition adalah disposisi barang retur di Gudang Retur (Desain Data Model return_disposition_lines, U-09).
type Disposition string

const (
	DispositionRestock         Disposition = "restock"
	DispositionRepack          Disposition = "repack"
	DispositionReprocess       Disposition = "reprocess"
	DispositionMarketingSample Disposition = "marketing_sample"
	DispositionDispose         Disposition = "dispose"
)

var dispositionLabels = map[Disposition]string{
	DispositionRestock: "Kembali ke stok", DispositionRepack: "Repacking", DispositionReprocess: "Reprocess maklon",
	DispositionMarketingSample: "Sampel/marketing", DispositionDispose: "Pemusnahan",
}

// Label mengembalikan nama disposisi untuk UI.
func (d Disposition) Label() string { return dispositionLabels[d] }

// Valid memeriksa nilai disposisi.
func (d Disposition) Valid() bool {
	_, ok := dispositionLabels[d]
	return ok
}

var returnReasonCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,29}$`)

// ReturnReason adalah master alasan retur (MD-13).
type ReturnReason struct {
	ID        uuid.UUID
	Code      string
	Name      string
	AppliesTo ReturnScope
	// DefaultDisposition adalah usulan awal disposisi; kosong = tanpa usulan.
	DefaultDisposition Disposition
	Description        string
	IsActive           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Version            int
}

// ReturnReasonInput adalah isian Tambah/Ubah Alasan Retur. Code hanya dipakai saat membuat.
type ReturnReasonInput struct {
	Code               string
	Name               string
	AppliesTo          ReturnScope
	DefaultDisposition Disposition
	Description        string
	IsActive           bool
}

func (in *ReturnReasonInput) normalize() {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
}

func (in ReturnReasonInput) validate(withCode bool) map[string]string {
	fields := map[string]string{}
	if withCode && !returnReasonCodePattern.MatchString(in.Code) {
		fields["code"] = "Kode huruf kapital, angka, atau garis bawah (2–30 karakter), mis. DAMAGED_SHIPPING"
	}
	if in.Name == "" {
		fields["name"] = "Nama wajib diisi"
	} else if len(in.Name) > 150 {
		fields["name"] = "Nama paling panjang 150 karakter"
	}
	if !in.AppliesTo.Valid() {
		fields["appliesTo"] = "Pilih retur penjualan, retur pembelian, atau keduanya"
	}
	if in.DefaultDisposition != "" && !in.DefaultDisposition.Valid() {
		fields["defaultDisposition"] = "Disposisi tidak dikenal"
	}
	if len(in.Description) > 500 {
		fields["description"] = "Keterangan paling panjang 500 karakter"
	}
	return fields
}

// NewReturnReason membuat alasan retur baru.
func NewReturnReason(in ReturnReasonInput) (ReturnReason, error) {
	in.normalize()
	if fields := in.validate(true); len(fields) > 0 {
		return ReturnReason{}, apperr.Validation(fields)
	}
	r := ReturnReason{Code: in.Code}
	r.apply(in)
	return r, nil
}

// Update mengubah alasan retur. Kode tidak berubah (dipakai laporan RPT-09).
func (r *ReturnReason) Update(in ReturnReasonInput) error {
	in.normalize()
	if fields := in.validate(false); len(fields) > 0 {
		return apperr.Validation(fields)
	}
	r.apply(in)
	return nil
}

func (r *ReturnReason) apply(in ReturnReasonInput) {
	r.Name, r.AppliesTo, r.DefaultDisposition = in.Name, in.AppliesTo, in.DefaultDisposition
	r.Description, r.IsActive = in.Description, in.IsActive
}

// UsableError mengembalikan alasan alasan retur tidak boleh dipakai dokumen jenis doc, atau "".
func (r ReturnReason) UsableError(doc ReturnScope) string {
	switch {
	case !r.IsActive:
		return "Alasan retur " + r.Code + " tidak aktif"
	case !r.AppliesTo.Covers(doc):
		return "Alasan retur " + r.Code + " tidak berlaku untuk " + strings.ToLower(doc.Label())
	}
	return ""
}
