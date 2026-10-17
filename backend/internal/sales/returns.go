package sales

import (
	"context"

	"github.com/google/uuid"

	"balans/internal/approval"
	"balans/internal/sales/domain"
	"balans/internal/sales/service"
)

// Retur Penjualan (S-314, S-317) dan Nota Kredit. Barang retur masuk Inbound modul warehouse (Karantina); hasil
// QC dikembalikan lewat OnReturnInboundQC; QC selesai → jurnal A-13 dan nota kredit/refund.
type (
	// SalesReturn adalah Retur Penjualan (RJ).
	SalesReturn = domain.SalesReturn
	// CreditNote adalah Nota Kredit (CN).
	CreditNote = domain.CreditNote
	// SettlementMethod adalah cara penyelesaian retur (nota kredit, refund, marketplace).
	SettlementMethod = domain.SettlementMethod

	// ReturnDeps adalah dependensi Retur Penjualan yang diisi adapter saat wiring.
	ReturnDeps = service.ReturnDeps
	// ReturnInbounds membuat Inbound barang retur.
	ReturnInbounds = service.ReturnInbounds
	// ReturnInboundRequest adalah permintaan Inbound barang retur.
	ReturnInboundRequest = service.ReturnInboundRequest
	// ReturnInboundLine adalah baris Inbound barang retur (ItemID = source_line_id).
	ReturnInboundLine = service.ReturnInboundLine
	// InboundRef adalah Inbound yang dibuat.
	InboundRef = service.InboundRef
	// ReturnLocations menentukan lokasi Inbound & Gudang Retur gudang asal stok.
	ReturnLocations = service.ReturnLocations
	// ReturnStaff mencari pengguna aktif per role.
	ReturnStaff = service.ReturnStaff
	// StaffRef adalah pengguna aktif.
	StaffRef = service.StaffRef
	// Notifier mengirim notifikasi in-app.
	Notifier = service.Notifier
	// ReturnQCOutcome adalah hasil QC satu baris Inbound barang retur.
	ReturnQCOutcome = service.ReturnQCOutcome
)

// Konstanta Retur Penjualan untuk wiring.
const (
	SettlementCreditNote  = domain.SettlementCreditNote
	SettlementRefund      = domain.SettlementRefund
	SettlementMarketplace = domain.SettlementMarketplace

	DocSalesReturn = service.DocSalesReturn
	DocCreditNote  = service.DocCreditNote

	PermReturnRead     = service.PermReturnRead
	PermReturnWrite    = service.PermReturnWrite
	PermCreditNoteRead = service.PermCreditNoteRead

	// SourceSalesReturn adalah source_type stok/Inbound/jurnal A-13 retur penjualan (= warehouse.SourceSalesReturn).
	SourceSalesReturn = service.SourceSalesReturn
	// FileOwnerSalesReturnPhoto adalah jenis pemilik file foto pickup, foto barang, dan form retur.
	FileOwnerSalesReturnPhoto = service.FileOwnerSalesReturnPhoto
)

// SetReturnDeps memasang dependensi Retur Penjualan (Inbound, lokasi, staf, notifikasi) setelah modul warehouse
// dan identity disusun.
func (m *Module) SetReturnDeps(d ReturnDeps) { m.returns.SetDeps(d) }

// ReturnAuditEntities memetakan entitas audit Retur Penjualan & Nota Kredit ke permission baca (tab Riwayat).
func ReturnAuditEntities() map[string]string {
	return map[string]string{
		service.EntitySalesReturn: service.PermReturnRead,
		service.EntityCreditNote:  service.PermCreditNoteRead,
	}
}

// RegisterReturnApprovalHandlers mendaftarkan handler approval `sales_refund` (refund disetujui Head F&A →
// Menunggu Pembayaran; ditolak → saldo nota kredit). Dipanggil SETELAH handler dev.
func (m *Module) RegisterReturnApprovalHandlers(r ApprovalRegistrar) {
	r.RegisterHandler(approval.SubjectSalesRefund, service.PermCreditNoteRead, approval.Handler{
		OnApproved: m.returns.OnRefundApproved, OnRejected: m.returns.OnRefundRejected,
	})
}

// OnReturnInboundQC mencatat hasil QC Inbound barang retur di transaksi QC (handler warehouse.RegisterInboundHandler
// untuk source_type sales_return). Semua item selesai QC → retur Diposting (jurnal A-13, nota kredit/refund).
func (m *Module) OnReturnInboundQC(ctx context.Context, o ReturnQCOutcome) error {
	return m.returns.OnInboundQC(ctx, o)
}

// SalesReturnByID membaca Retur Penjualan beserta baris dan item tanpa cek permission (laporan retur RPT-09).
func (m *Module) SalesReturnByID(ctx context.Context, id uuid.UUID) (SalesReturn, error) {
	return m.docs.Store.GetReturn(ctx, id, false)
}
