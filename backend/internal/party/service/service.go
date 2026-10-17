// Package service berisi use case master pelanggan, mitra & PKS, kode produk mitra, dan pemasok & maklon
// (BRD MD-06, MD-07, MD-08). Modul lain (identity, hris, filestore, catalog) dipakai lewat interface yang
// diisi adapter di internal/app.
package service

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"balans/internal/audit"
	"balans/internal/party/domain"
	"balans/internal/party/postgres"
	"balans/internal/platform/clock"
	"balans/internal/platform/paging"
)

// Permission modul party (Desain Role & Approval bagian 3.1).
const (
	PermCustomerRead     = "party.customer.read"
	PermCustomerWrite    = "party.customer.write"
	PermPartnerRead      = "party.partner.read"
	PermPartnerWrite     = "party.partner.write"
	PermAgreementRead    = "party.partner_agreement.read"
	PermAgreementWrite   = "party.partner_agreement.write"
	PermProductCodeRead  = "party.partner_product_code.read"
	PermProductCodeWrite = "party.partner_product_code.write"
	PermSupplierRead     = "party.supplier.read"
	PermSupplierWrite    = "party.supplier.write"
)

// FileOwnerAgreement adalah jenis pemilik file PDF PKS di modul filestore.
const FileOwnerAgreement = "partner_agreement"

// Jenis entitas di audit trail.
const (
	EntityCustomer    = "customer"
	EntityPartner     = "partner"
	EntityAgreement   = "partner_agreement"
	EntityProductCode = "partner_product_code"
	EntitySupplier    = "supplier"
)

// EmployeeRef adalah ringkasan karyawan dari modul hris.
type EmployeeRef struct {
	ID       uuid.UUID
	Number   string
	Name     string
	IsActive bool
}

// Employees membaca karyawan dari modul hris (HR-03).
type Employees interface {
	// Employees mengembalikan karyawan beserta status aktif pada tanggal; ID yang tidak ada dilewati.
	Employees(ctx context.Context, ids []uuid.UUID, on clock.BusinessDate) (map[uuid.UUID]EmployeeRef, error)
	// IsActiveEmployee memeriksa karyawan aktif pada tanggal (belum resign).
	IsActiveEmployee(ctx context.Context, id uuid.UUID, on clock.BusinessDate) (bool, error)
}

// UserRef adalah ringkasan pengguna dari modul identity.
type UserRef struct {
	ID    uuid.UUID
	Name  string
	Email string
	Roles []string
}

// Users membaca pengguna aktif dari modul identity.
type Users interface {
	// UsersByID mengembalikan pengguna aktif; ID yang tidak ada atau nonaktif dilewati.
	UsersByID(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]UserRef, error)
	// SearchUsers mencari pengguna aktif; role kosong = semua role.
	SearchUsers(ctx context.Context, q, role string, limit int) ([]UserRef, error)
}

// Files menautkan dan membaca file di modul filestore.
type Files interface {
	Attach(ctx context.Context, fileID uuid.UUID, ownerType string, ownerID uuid.UUID) error
	Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// ProductRef adalah ringkasan produk dari modul catalog.
type ProductRef struct {
	ID   uuid.UUID
	SKU  string
	Name string
}

// Products membaca produk dari modul catalog. Opsional: nil selama modul catalog belum aktif.
type Products interface {
	Products(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]ProductRef, error)
}

type txRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type store interface {
	InsertCustomer(ctx context.Context, c domain.Customer, rawPhone string, actorID uuid.UUID) (domain.Customer, error)
	GetCustomer(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Customer, error)
	CustomerByPhone(ctx context.Context, phone string) (domain.Customer, bool, error)
	ListCustomers(ctx context.Context, f postgres.CustomerFilter, page paging.Page) (paging.Result[domain.Customer], error)
	UpdateCustomer(ctx context.Context, before, after domain.Customer, rawPhone string, actorID uuid.UUID) (domain.Customer, error)
	DeleteCustomer(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error

	InsertPartner(ctx context.Context, p domain.Partner, actorID uuid.UUID) (domain.Partner, error)
	GetPartner(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Partner, error)
	PartnersByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Partner, error)
	PartnersByMerchandiser(ctx context.Context, userID uuid.UUID) ([]domain.Partner, error)
	ListPartners(ctx context.Context, f postgres.PartnerFilter, page paging.Page) (paging.Result[domain.Partner], error)
	UpdatePartner(ctx context.Context, p domain.Partner, version int, actorID uuid.UUID) error
	DeletePartner(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error

	InsertAgreement(ctx context.Context, a domain.Agreement, actorID uuid.UUID) (domain.Agreement, error)
	GetAgreementForUpdate(ctx context.Context, id uuid.UUID) (domain.Agreement, error)
	ListAgreements(ctx context.Context, partnerID uuid.UUID) ([]domain.Agreement, error)
	AgreementsActiveOn(ctx context.Context, partnerIDs []uuid.UUID, on clock.BusinessDate) ([]domain.Agreement, error)
	ActiveAgreement(ctx context.Context, partnerID uuid.UUID, t domain.AgreementType, on clock.BusinessDate) (domain.Agreement, bool, error)
	UpdateAgreement(ctx context.Context, a domain.Agreement, version int, actorID uuid.UUID) error
	DeleteAgreement(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error

	InsertProductCode(ctx context.Context, c domain.ProductCode, actorID uuid.UUID) (domain.ProductCode, error)
	GetProductCodeForUpdate(ctx context.Context, id uuid.UUID) (domain.ProductCode, error)
	ProductCodeFor(ctx context.Context, partnerID, productID uuid.UUID) (domain.ProductCode, bool, error)
	ListProductCodes(ctx context.Context, f postgres.ProductCodeFilter, page paging.Page) (paging.Result[domain.ProductCode], error)
	UpdateProductCode(ctx context.Context, c domain.ProductCode, version int, actorID uuid.UUID) error
	DeleteProductCode(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error

	InsertSupplier(ctx context.Context, s domain.Supplier, actorID uuid.UUID) (domain.Supplier, error)
	GetSupplier(ctx context.Context, id uuid.UUID, forUpdate bool) (domain.Supplier, error)
	ListSuppliers(ctx context.Context, f postgres.SupplierFilter, page paging.Page) (paging.Result[domain.Supplier], error)
	UpdateSupplier(ctx context.Context, before, after domain.Supplier, actorID uuid.UUID) (domain.Supplier, error)
	DeleteSupplier(ctx context.Context, id uuid.UUID, version int, actorID uuid.UUID) error

	CustomerIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error)
	PartnerIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error)
	SupplierIDByCode(ctx context.Context, code string) (uuid.UUID, bool, error)
}

// Deps adalah dependensi Service.
type Deps struct {
	Store store
	Tx    txRunner
	Audit audit.Recorder
	Clock clock.Clock
	Log   *slog.Logger
	Users Users
	Files Files
	// Employees nil = modul hris belum aktif; pelanggan tidak bisa ditautkan ke karyawan.
	Employees Employees
	// Products nil = modul catalog belum aktif; nama produk tidak ditampilkan.
	Products Products
}

// Service adalah use case modul party.
type Service struct {
	Deps
}

// New membuat Service.
func New(d Deps) *Service { return &Service{Deps: d} }

func (s *Service) today() clock.BusinessDate { return clock.Today(s.Clock) }

// Today mengembalikan tanggal bisnis hari ini (WIB).
func (s *Service) Today() clock.BusinessDate { return s.today() }
