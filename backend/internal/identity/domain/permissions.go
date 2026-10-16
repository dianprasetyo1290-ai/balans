package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Kode role awal (Desain Role & Approval bagian 1). Role disemai migrasi; kode stabil dipakai di kode.
const (
	RoleSystemAdmin       = "system_admin"
	RolePurchasing        = "purchasing"
	RoleWarehouseStaff    = "warehouse_staff"
	RoleLogisticsLeader   = "logistics_leader"
	RoleFulfillmentLeader = "fulfillment_leader"
	RoleMerchandiser      = "merchandiser"
	RoleHeadLogistics     = "head_logistics"
	RoleFinance           = "finance"
	RoleHeadFinance       = "head_finance"
	RoleSalesMT           = "sales_mt"
	RoleSalesEcommerce    = "sales_ecommerce"
	RoleCustomerService   = "customer_service"
	RoleHeadSales         = "head_sales"
	RoleMarketing         = "marketing"
	RoleHRGA              = "hrga"
	RoleDivisionHead      = "division_head"
	RoleDirector          = "director"
	RolePresidentDirector = "president_director"
)

// Permission adalah satu hak akses `<modul>.<resource>.<aksi>` beserta role yang mendapatkannya saat pertama disemai.
type Permission struct {
	Code           string
	Module         string
	Description    string
	RequiresReauth bool
	DefaultRoles   []string
}

// matrixRoles adalah urutan kolom matriks Desain Role & Approval bagian 3.1 – 3.4.
var matrixRoles = []string{
	RoleSystemAdmin, RolePurchasing, RoleWarehouseStaff, RoleHeadLogistics, RoleFinance, RoleHeadFinance,
	RoleSalesMT, RoleSalesEcommerce, RoleCustomerService, RoleHeadSales, RoleMarketing, RoleHRGA,
	RoleDivisionHead, RoleDirector,
}

// teamRoles adalah kolom matriks bagian 3.5.
var teamRoles = []string{RoleLogisticsLeader, RoleFulfillmentLeader, RoleMerchandiser}

type matrixRow struct {
	resources []string
	cells     string
}

// Kode sel: R read, W write, S submit, A approve, P post, C cancel, V verify, I import & export, - tidak ada.
var matrix = []matrixRow{
	// 3.1 Master Data & Pengaturan
	{[]string{"catalog.product"}, "RWI R R R R R R R R R R R R R"},
	{[]string{"pricing.price_list", "pricing.promotion", "pricing.special_price"}, "R - - - R A RWS RWS R RWSA R - - R"},
	{[]string{"pricing.discount_limit"}, "RW - - - - A - - - RWS - - - R"},
	{[]string{"party.customer"}, "RWI R - - R R RW RW RW R R - - R"},
	{[]string{"party.partner", "party.partner_agreement", "party.partner_product_code"}, "RWI R - - R R RW - - RW - - - R"},
	{[]string{"party.supplier"}, "RWI RW - R RW R - - - - - - - R"},
	{[]string{"warehouse.warehouse", "warehouse.location"}, "RWI R R RW R R R R - R - - - R"},
	{[]string{"hris.employee"}, "R - - - R R - - - - - RWI - R"},
	{[]string{"accounting.account", "accounting.posting_rule"}, "R - - - RW RWA - - - - - - - R"},
	{[]string{"accounting.dimension"}, "RW - - - RW RW R R - R RW - - R"},
	{[]string{"accounting.expense_type", "sales.return_reason"}, "RW - - R RW RW - - R - R - - R"},
	{[]string{"identity.user", "identity.user_role", "identity.role"}, "RW - - - - - - - - - - - - R"},
	{[]string{"approval.policy"}, "RW - - - R R - - - - - - - RA"},
	{[]string{"setting.setting", "document.template", "document.sequence"}, "RW - - - R RW - - - - - - - R"},
	{[]string{"audit.log"}, "R - - - - R - - - - - - - R"},
	// 3.2 Pengadaan, Gudang, Persediaan
	{[]string{"procurement.purchase_request"}, "R RWS - RWS RWS R RWS RWS - RWS RWS RWS RA RA"},
	{[]string{"procurement.purchase_order"}, "R RWSC R R R RA - - - - - - - RA"},
	{[]string{"procurement.goods_receipt"}, "R RWPC RWP R R R - - - - - - - R"},
	{[]string{"procurement.supplier_bill"}, "R R - - RWPC RA - - - - - - - R"},
	{[]string{"procurement.purchase_advance"}, "R RW - - RWP R - - - - - - - R"},
	{[]string{"procurement.purchase_return"}, "R RWSP RWP RA R R - - - - - - - R"},
	{[]string{"warehouse.inbound"}, "R R RWP RWP R R - - - - - - - R"},
	{[]string{"warehouse.stock_transfer"}, "R RWPC RWP RWP R R R R - - - - - R"},
	{[]string{"warehouse.stock_count"}, "R R RWS RWSA R RA RWS - - - - - - R"},
	{[]string{"warehouse.outbound"}, "R R RWP RWP R - R R R - - - - R"},
	{[]string{"warehouse.outbound_qc"}, "R R R RWP R - R R R - - - - R"},
	{[]string{"warehouse.return_disposition"}, "R R RWS RA RA RA - - - - - - - RA"},
	{[]string{"inventory.stock"}, "R R R R R R R R R R R R R R"},
	{[]string{"inventory.internal_usage"}, "R R RP R R RA RWS - - - RWS RWS RA R"},
	{[]string{"inventory.repacking", "inventory.disposal"}, "R R RWP RWSA RA RA - - - - - - - RA"},
	// 3.3 Penjualan, Logistik
	{[]string{"sales.sales_order"}, "R R R R R R RWSC RWSC RWSC RA R - - R"},
	{[]string{"sales.marketplace_order"}, "R - - - R - - RWI - R - - - R"},
	{[]string{"sales.proforma_invoice", "sales.invoice"}, "R R - - RWPC R RWS R RWS R - - - R"},
	{[]string{"sales.credit_note", "sales.sales_return"}, "R R RW R RWP RA RWS RWS RWS R - - - R"},
	{[]string{"sales.consignment_reconciliation"}, "R R - - RW RA RWSA - - R - - - R"},
	{[]string{"logistics.delivery_order", "logistics.shipment"}, "R RWPC RWP R R - RWS RWS RWS - - - - R"},
	{[]string{"logistics.label"}, "R P P P - - P P P - - - - -"},
	// 3.4 Keuangan
	{[]string{"accounting.journal"}, "R - - - RWPC RWPCA - - - - - - - R"},
	{[]string{"accounting.payment"}, "R - - - RWP RA - - R - - - - R"},
	{[]string{"accounting.bank_reconciliation", "accounting.settlement"}, "R - - - RWPI R - - - - - - - R"},
	{[]string{"accounting.expense_claim"}, "R RWS RWS RWS RWP RA RWS RWS RWS RWS RWS RWS RA RA"},
	{[]string{"accounting.petty_cash"}, "R - - - RWP RA - - - - - - - R"},
	{[]string{"accounting.withholding", "accounting.final_tax"}, "R - - - RWPI RA - - - - - - - R"},
	{[]string{"accounting.fixed_asset"}, "R - - - RWP RA - - - - - - - R"},
	{[]string{"accounting.fiscal_period"}, "R - - - RW RWPA - - - - - - - A"},
	{[]string{"accounting.report"}, "- - - - R R - - - - - - - R"},
	{[]string{"dataexchange.transaction"}, "R RI - - RI RI RI RI - - - RI - -"},
}

// teamMatrix adalah matriks Logistik Leader, Fulfillment Leader, Merchandiser (bagian 3.5).
var teamMatrix = []matrixRow{
	{[]string{"catalog.product", "party.partner_product_code", "warehouse.warehouse", "warehouse.location"}, "R R R"},
	{[]string{"procurement.purchase_order"}, "R - -"},
	{[]string{"procurement.goods_receipt"}, "RWP R RW"},
	{[]string{"warehouse.inbound"}, "RWP R R"},
	{[]string{"warehouse.stock_transfer"}, "RWPC RWS R"},
	{[]string{"warehouse.stock_count"}, "RWS RWS RWS"},
	{[]string{"warehouse.outbound"}, "RWP R R"},
	{[]string{"warehouse.outbound_qc"}, "R RWP RWP"},
	{[]string{"warehouse.return_disposition"}, "RWS - -"},
	{[]string{"inventory.stock"}, "R R R"},
	{[]string{"inventory.internal_usage"}, "RP R R"},
	{[]string{"sales.sales_order"}, "R R -"},
	{[]string{"sales.sales_return"}, "RV RW RW"},
	{[]string{"logistics.delivery_order", "logistics.shipment"}, "RWPC RWP RP"},
	{[]string{"logistics.label"}, "P P -"},
	{[]string{"dataexchange.transaction"}, "RI - -"},
}

// extraPermissions adalah aksi khusus di luar kode sel matriks.
var extraPermissions = []Permission{
	{Code: "accounting.fiscal_period.reopen", Description: "Periode akuntansi: buka kembali", RequiresReauth: true, DefaultRoles: []string{RoleHeadFinance}},
}

// reauthPermissions adalah aksi bertanda ★ (Desain Role & Approval bagian 2).
var reauthPermissions = []string{
	"accounting.fiscal_period.reopen", "accounting.payment.post", "identity.user_role.write", "identity.role.write", "setting.setting.write",
}

var actionLetters = map[rune][]string{
	'R': {"read"}, 'W': {"write"}, 'S': {"submit"}, 'A': {"approve"}, 'P': {"post"},
	'C': {"cancel"}, 'V': {"verify"}, 'I': {"import", "export"},
}

var actionLabels = map[string]string{
	"read": "lihat", "write": "buat & ubah", "submit": "ajukan", "approve": "setujui", "post": "posting/proses",
	"cancel": "batalkan", "verify": "verifikasi", "import": "impor", "export": "ekspor",
}

// Catalog menyusun seluruh permission Fase 1 dari matriks. Urutan stabil berdasarkan kode.
func Catalog() []Permission {
	byCode := map[string]*Permission{}
	add := func(rows []matrixRow, roles []string) {
		for _, row := range rows {
			cells := strings.Fields(row.cells)
			if len(cells) != len(roles) {
				panic(fmt.Sprintf("permission matrix %v: %d sel untuk %d role", row.resources, len(cells), len(roles)))
			}
			for _, res := range row.resources {
				for i, cell := range cells {
					if cell == "-" {
						continue
					}
					for _, letter := range cell {
						actions, ok := actionLetters[letter]
						if !ok {
							panic(fmt.Sprintf("permission matrix %s: kode sel %q tidak dikenal", res, letter))
						}
						for _, action := range actions {
							code := res + "." + action
							p, ok := byCode[code]
							if !ok {
								p = &Permission{Code: code}
								byCode[code] = p
							}
							if !slices.Contains(p.DefaultRoles, roles[i]) {
								p.DefaultRoles = append(p.DefaultRoles, roles[i])
							}
						}
					}
				}
			}
		}
	}
	add(matrix, matrixRoles)
	add(teamMatrix, teamRoles)
	for _, e := range extraPermissions {
		byCode[e.Code] = &Permission{Code: e.Code, Description: e.Description, DefaultRoles: e.DefaultRoles}
	}
	// Direktur Utama memegang hak Direksi (approval tingkat atas termasuk PO > Rp50 jt).
	for _, p := range byCode {
		if slices.Contains(p.DefaultRoles, RoleDirector) {
			p.DefaultRoles = append(p.DefaultRoles, RolePresidentDirector)
		}
	}

	out := make([]Permission, 0, len(byCode))
	for _, p := range byCode {
		module, rest, _ := strings.Cut(p.Code, ".")
		resource, action, _ := strings.Cut(rest, ".")
		p.Module = module
		if p.Description == "" {
			p.Description = resourceLabel(module+"."+resource) + ": " + actionLabels[action]
		}
		p.RequiresReauth = slices.Contains(reauthPermissions, p.Code)
		slices.Sort(p.DefaultRoles)
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b Permission) int { return strings.Compare(a.Code, b.Code) })
	return out
}

var resourceLabels = map[string]string{
	"catalog.product": "Produk", "pricing.price_list": "Price list", "pricing.promotion": "Promo",
	"pricing.special_price": "Harga khusus", "pricing.discount_limit": "Batas diskon", "party.customer": "Pelanggan",
	"party.partner": "Mitra", "party.partner_agreement": "PKS mitra", "party.partner_product_code": "Kode produk mitra",
	"party.supplier": "Pemasok & maklon", "warehouse.warehouse": "Gudang", "warehouse.location": "Lokasi stok",
	"hris.employee": "Karyawan", "accounting.account": "Akun (COA)", "accounting.posting_rule": "Aturan jurnal otomatis",
	"accounting.dimension": "Dimensi", "accounting.expense_type": "Jenis biaya", "sales.return_reason": "Alasan retur",
	"identity.user": "Pengguna", "identity.user_role": "Role pengguna", "identity.role": "Role & hak akses",
	"approval.policy": "Kebijakan persetujuan", "setting.setting": "Pengaturan", "document.template": "Template dokumen",
	"document.sequence": "Penomoran dokumen", "audit.log": "Riwayat perubahan",
	"procurement.purchase_request": "Purchase Request", "procurement.purchase_order": "Purchase Order",
	"procurement.goods_receipt": "Penerimaan barang", "procurement.supplier_bill": "Tagihan pemasok",
	"procurement.purchase_advance": "Uang muka pembelian", "procurement.purchase_return": "Retur pembelian",
	"warehouse.inbound": "Inbound", "warehouse.stock_transfer": "Transfer stok", "warehouse.stock_count": "Stock count",
	"warehouse.outbound": "Outbound", "warehouse.outbound_qc": "QC Outbound", "warehouse.return_disposition": "Disposisi retur",
	"inventory.stock": "Stok", "inventory.internal_usage": "Pemakaian internal", "inventory.repacking": "Repacking",
	"inventory.disposal": "Pemusnahan", "sales.sales_order": "Sales Order", "sales.marketplace_order": "Order marketplace",
	"sales.proforma_invoice": "Proforma Invoice", "sales.invoice": "Invoice", "sales.credit_note": "Nota kredit",
	"sales.sales_return": "Retur penjualan", "sales.consignment_reconciliation": "Rekonsiliasi konsinyasi",
	"logistics.delivery_order": "Delivery Order", "logistics.shipment": "Pengiriman", "logistics.label": "Label",
	"accounting.journal": "Jurnal", "accounting.payment": "Pembayaran", "accounting.bank_reconciliation": "Rekonsiliasi bank",
	"accounting.settlement": "Settlement marketplace", "accounting.expense_claim": "Pengajuan biaya",
	"accounting.petty_cash": "Kas kecil", "accounting.withholding": "PPh potong", "accounting.final_tax": "PPh Final",
	"accounting.fixed_asset": "Aset tetap", "accounting.fiscal_period": "Periode akuntansi",
	"accounting.report": "Laporan keuangan", "dataexchange.transaction": "Impor/ekspor transaksi",
}

func resourceLabel(r string) string {
	if l, ok := resourceLabels[r]; ok {
		return l
	}
	return r
}
