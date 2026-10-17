// Package document adalah API publik modul document: penomoran dokumen KODE/TAHUN/BULAN/NOMOR (U-15,
// Technical Guideline 5.12). Modul dokumen memanggil Next di dalam transaksi submit/posting-nya.
package document

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"balans/internal/document/domain"
	documenthttp "balans/internal/document/http"
	"balans/internal/document/postgres"
	"balans/internal/document/service"
	"balans/internal/platform/clock"
	"balans/internal/platform/httpx"
)

// Name adalah nama modul di ENABLED_MODULES.
const Name = "document"

// Type adalah kode dokumen, mis. `JV`.
type Type = domain.Type

// Kode dokumen (Technical Guideline 5.12, Desain Penomoran bagian 1).
const (
	TypePurchaseRequest           = domain.TypePurchaseRequest
	TypePurchaseOrder             = domain.TypePurchaseOrder
	TypeGoodsReceipt              = domain.TypeGoodsReceipt
	TypeDeliveryOrder             = domain.TypeDeliveryOrder
	TypeProformaInvoice           = domain.TypeProformaInvoice
	TypeInvoice                   = domain.TypeInvoice
	TypeCreditNote                = domain.TypeCreditNote
	TypeDebitNote                 = domain.TypeDebitNote
	TypeSalesReturn               = domain.TypeSalesReturn
	TypePurchaseReturn            = domain.TypePurchaseReturn
	TypeStockTransfer             = domain.TypeStockTransfer
	TypeStockCount                = domain.TypeStockCount
	TypeConsignmentReconciliation = domain.TypeConsignmentReconciliation
	TypeExpenseClaim              = domain.TypeExpenseClaim
	TypePettyCash                 = domain.TypePettyCash
	TypeJournal                   = domain.TypeJournal
	TypeSalesOrder                = domain.TypeSalesOrder
	TypeInbound                   = domain.TypeInbound
	TypeOutbound                  = domain.TypeOutbound
	TypeInternalUsage             = domain.TypeInternalUsage
	TypeDisposal                  = domain.TypeDisposal
	TypePayment                   = domain.TypePayment
	TypeResellerDeposit           = domain.TypeResellerDeposit
	TypeVendingRefill             = domain.TypeVendingRefill
	TypeVendingReconciliation     = domain.TypeVendingReconciliation
)

// ErrNoTransaction: Next dipanggil di luar transaksi bisnis.
var ErrNoTransaction = service.ErrNoTransaction

// Deps adalah dependensi modul.
type Deps struct {
	Pool      *pgxpool.Pool
	Clock     clock.Clock
	Responder *httpx.Responder
}

// Module adalah modul document.
type Module struct {
	svc *service.Service
	rs  *httpx.Responder
	h   *documenthttp.DocumentHandler
}

// New menyusun modul.
func New(d Deps) *Module {
	svc := service.New(postgres.NewStore(d.Pool), d.Clock)
	return &Module{svc: svc, rs: d.Responder, h: documenthttp.NewHandler(svc, d.Clock)}
}

// Name mengembalikan nama modul.
func (m *Module) Name() string { return Name }

// RegisterRoutes mendaftarkan GET /api/v1/document-sequences.
func (m *Module) RegisterRoutes(r chi.Router) { documenthttp.Register(r, m.h, m.rs) }

// RegisterWorkers tidak ada.
func (m *Module) RegisterWorkers(*river.Workers) {}

// PeriodicJobs tidak ada.
func (m *Module) PeriodicJobs() []*river.PeriodicJob { return nil }

// Next mengambil nomor dokumen berikutnya, mis. `JV/2027/01/00102`, di transaksi aktif (wajib; tanpa
// transaksi mengembalikan ErrNoTransaction). Rollback transaksi mengembalikan nomor sehingga tidak loncat.
func (m *Module) Next(ctx context.Context, t Type, businessDate clock.BusinessDate) (string, error) {
	return m.svc.Next(ctx, t, businessDate)
}
