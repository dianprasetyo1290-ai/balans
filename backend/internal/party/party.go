// Package party adalah API publik master pelanggan, mitra & PKS, kode produk mitra, dan pemasok & maklon
// (BRD MD-06, MD-07, MD-08). Modul lain membaca data lewat fungsi di sini, misalnya logistics memeriksa
// RequireActiveAgreement sebelum DO konsinyasi dan pricing memeriksa EmployeePriceEligible.
package party

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/audit"
	"balans/internal/party/domain"
	partyhttp "balans/internal/party/http"
	"balans/internal/party/postgres"
	"balans/internal/party/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/db"
	"balans/internal/platform/httpx"
	"balans/internal/platform/tabular"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "party"

type (
	// Customer adalah master pelanggan beserta alamatnya.
	Customer = domain.Customer
	// Address adalah alamat pengiriman pelanggan.
	Address = domain.Address
	// CustomerType adalah tipe pelanggan.
	CustomerType = domain.CustomerType
	// Partner adalah master mitra MT/konsinyasi/B2B.
	Partner = domain.Partner
	// Agreement adalah PKS mitra (effective-dated).
	Agreement = domain.Agreement
	// AgreementType adalah jenis PKS.
	AgreementType = domain.AgreementType
	// BorneBy adalah penanggung selisih rekonsiliasi konsinyasi.
	BorneBy = domain.BorneBy
	// ProductCode adalah kode artikel & barcode retailer per SKU per mitra.
	ProductCode = domain.ProductCode
	// Supplier adalah master pemasok atau maklon.
	Supplier = domain.Supplier
	// SupplierType adalah jenis pemasok.
	SupplierType = domain.SupplierType
	// WithholdingType adalah PPh default pemasok.
	WithholdingType = domain.WithholdingType
	// BankAccount adalah rekening pemasok (nomor terdekripsi).
	BankAccount = domain.BankAccount

	// EmployeeRef adalah ringkasan karyawan dari modul hris.
	EmployeeRef = service.EmployeeRef
	// Employees membaca karyawan dari modul hris.
	Employees = service.Employees
	// UserRef adalah ringkasan pengguna dari modul identity.
	UserRef = service.UserRef
	// Users membaca pengguna dari modul identity.
	Users = service.Users
	// Files menautkan file PDF PKS di modul filestore.
	Files = service.Files
	// ProductRef adalah ringkasan produk dari modul catalog.
	ProductRef = service.ProductRef
	// Products membaca produk dari modul catalog.
	Products = service.Products
	// Cipher mengenkripsi NPWP dan nomor rekening.
	Cipher = postgres.Cipher
)

// Nilai enum yang dipakai modul lain.
const (
	CustomerRetail   = domain.CustomerRetail
	CustomerB2B      = domain.CustomerB2B
	CustomerReseller = domain.CustomerReseller
	CustomerEmployee = domain.CustomerEmployee

	AgreementConsignment = domain.AgreementConsignment
	AgreementModernTrade = domain.AgreementModernTrade

	BorneByPartner = domain.BorneByPartner
	BorneByBalans  = domain.BorneByBalans

	SupplierRegular          = domain.SupplierRegular
	SupplierTollManufacturer = domain.SupplierTollManufacturer

	WithholdingNone  = domain.WithholdingNone
	WithholdingPPh23 = domain.WithholdingPPh23
	WithholdingPPh21 = domain.WithholdingPPh21
	WithholdingPPh42 = domain.WithholdingPPh42
)

// Permission dan jenis pemilik file yang dipakai saat wiring.
const (
	PermAgreementRead  = service.PermAgreementRead
	PermAgreementWrite = service.PermAgreementWrite
	FileOwnerAgreement = service.FileOwnerAgreement
)

// AuditRegistrar mendaftarkan permission baca entitas untuk tab Riwayat (modul audit).
type AuditRegistrar interface {
	RegisterEntity(entityType, readPermission string)
}

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Tx        *db.TxManager
	Clock     clock.Clock
	Log       *slog.Logger
	Responder *httpx.Responder
	Audit     audit.Recorder
	Cipher    Cipher
	Users     Users
	Files     Files
	// Employees nil = modul hris tidak aktif.
	Employees Employees
	// Products nil = modul catalog belum aktif.
	Products Products
}

// Module adalah modul party.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(service.Deps{
		Store: postgres.NewStore(d.Pool, d.Cipher), Tx: d.Tx, Audit: d.Audit, Clock: d.Clock, Log: d.Log,
		Users: d.Users, Files: d.Files, Employees: d.Employees, Products: d.Products,
	})
	return &Module{svc: svc, rs: d.Responder}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan route pelanggan, mitra, PKS, kode produk mitra, dan pemasok.
func (m *Module) RegisterRoutes(r chi.Router) {
	partyhttp.Register(r, partyhttp.NewHandler(m.svc), m.rs)
}

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// RegisterAuditEntities mendaftarkan permission baca entitas party untuk tab Riwayat.
func (m *Module) RegisterAuditEntities(a AuditRegistrar) {
	a.RegisterEntity(service.EntityCustomer, service.PermCustomerRead)
	a.RegisterEntity(service.EntityPartner, service.PermPartnerRead)
	a.RegisterEntity(service.EntityAgreement, service.PermAgreementRead)
	a.RegisterEntity(service.EntityProductCode, service.PermProductCodeRead)
	a.RegisterEntity(service.EntitySupplier, service.PermSupplierRead)
}

// SetEmployees memasang pembaca karyawan setelah modul hris (tier lebih tinggi) disusun.
func (m *Module) SetEmployees(e Employees) { m.svc.Employees = e }

// CustomerByID mengembalikan pelanggan beserta alamat (termasuk EmployeeID untuk harga karyawan).
func (m *Module) CustomerByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return m.svc.CustomerByID(ctx, id)
}

// CustomerByPhone mencari pelanggan dari nomor HP format apa pun (08…, 628…, +628…); ok=false jika tidak ada.
func (m *Module) CustomerByPhone(ctx context.Context, phone string) (Customer, bool, error) {
	return m.svc.CustomerByPhone(ctx, phone)
}

// EmployeePriceEligible bernilai true jika pelanggan tertaut ke karyawan yang aktif pada tanggal transaksi.
// Karyawan yang resign otomatis kehilangan hak harga karyawan (HR-03).
func (m *Module) EmployeePriceEligible(ctx context.Context, customerID uuid.UUID, on clock.BusinessDate) (bool, error) {
	return m.svc.EmployeePriceEligible(ctx, customerID, on)
}

// PartnerByID mengembalikan mitra.
func (m *Module) PartnerByID(ctx context.Context, id uuid.UUID) (Partner, error) {
	return m.svc.PartnerByID(ctx, id)
}

// PartnersForMerchandiser mengembalikan mitra yang ditugaskan ke merchandiser.
func (m *Module) PartnersForMerchandiser(ctx context.Context, userID uuid.UUID) ([]Partner, error) {
	return m.svc.PartnersForMerchandiser(ctx, userID)
}

// ActiveAgreement mengembalikan PKS jenis t yang berlaku pada tanggal; ok=false jika tidak ada.
func (m *Module) ActiveAgreement(ctx context.Context, partnerID uuid.UUID, t AgreementType, on clock.BusinessDate) (Agreement, bool, error) {
	return m.svc.ActiveAgreement(ctx, partnerID, t, on)
}

// RequireActiveAgreement menolak transaksi dengan VALIDATION_FAILED pada field `partnerId` bila mitra tidak
// punya PKS jenis t yang berlaku pada tanggal transaksi (mis. DO konsinyasi saat PKS kedaluwarsa).
func (m *Module) RequireActiveAgreement(ctx context.Context, partnerID uuid.UUID, t AgreementType, on clock.BusinessDate) (Agreement, error) {
	return m.svc.RequireActiveAgreement(ctx, partnerID, t, on)
}

// ProductCodeFor mengembalikan kode artikel & barcode retailer satu SKU di satu mitra; ok=false jika belum ada.
func (m *Module) ProductCodeFor(ctx context.Context, partnerID, productID uuid.UUID) (ProductCode, bool, error) {
	return m.svc.ProductCodeFor(ctx, partnerID, productID)
}

// CustomerColumns, SupplierColumns, dan ProductCodeColumns adalah kolom template impor/ekspor (S-125, S-134).
func CustomerColumns() []tabular.Column    { return service.CustomerColumns }
func SupplierColumns() []tabular.Column    { return service.SupplierColumns }
func ProductCodeColumns() []tabular.Column { return service.ProductCodeColumns }

// EmployeeNumbers opsional pada Employees: mencari karyawan dari nomor karyawan (impor pelanggan).
type EmployeeNumbers = service.EmployeeNumbers

// ProductSKUs opsional pada Products: mencari produk dari SKU (impor kode produk mitra).
type ProductSKUs = service.ProductSKUs

// SetProducts memasang pembaca produk modul catalog.
func (m *Module) SetProducts(p Products) { m.svc.Products = p }

// ValidateCustomerRow memeriksa satu baris impor pelanggan tanpa menyimpan (kode pelanggan dari sequence).
func (m *Module) ValidateCustomerRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ValidateCustomerRow(ctx, row)
}

// ImportCustomerRow membuat atau memperbarui pelanggan (kunci kode atau nomor HP) dari satu baris Excel.
func (m *Module) ImportCustomerRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportCustomerRow(ctx, row)
}

// ExportCustomers mengalirkan pelanggan sebagai baris Excel.
func (m *Module) ExportCustomers(ctx context.Context, emit func(tabular.Row) error) error {
	return m.svc.ExportCustomers(ctx, emit)
}

// ImportSupplierRow membuat atau memperbarui pemasok/maklon (kunci kode) dari satu baris Excel.
func (m *Module) ImportSupplierRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportSupplierRow(ctx, row)
}

// ExportSuppliers mengalirkan pemasok sebagai baris Excel (NPWP dan rekening tersamar).
func (m *Module) ExportSuppliers(ctx context.Context, emit func(tabular.Row) error) error {
	return m.svc.ExportSuppliers(ctx, emit)
}

// ImportProductCodeRow membuat atau memperbarui kode produk mitra (kunci mitra–SKU) dari satu baris Excel.
func (m *Module) ImportProductCodeRow(ctx context.Context, row tabular.Row) error {
	return m.svc.ImportProductCodeRow(ctx, row)
}

// ExportProductCodes mengalirkan kode produk mitra sebagai baris Excel.
func (m *Module) ExportProductCodes(ctx context.Context, emit func(tabular.Row) error) error {
	return m.svc.ExportProductCodes(ctx, emit)
}

// CustomerIDByCode mencari pelanggan (belum dihapus) dari kode (impor piutang terbuka).
func (m *Module) CustomerIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	return m.svc.Store.CustomerIDByCode(ctx, code)
}

// SupplierIDByCode mencari pemasok dari kode (dipakai impor lokasi maklon).
func (m *Module) SupplierIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	return m.svc.Store.SupplierIDByCode(ctx, code)
}

// PartnerIDByCode mencari mitra dari kode (dipakai impor lokasi konsinyasi).
func (m *Module) PartnerIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	return m.svc.Store.PartnerIDByCode(ctx, code)
}

// SupplierByID mengembalikan pemasok beserta rekening (nomor terdekripsi; tampilkan tersamar).
func (m *Module) SupplierByID(ctx context.Context, id uuid.UUID) (Supplier, error) {
	return m.svc.SupplierByID(ctx, id)
}
