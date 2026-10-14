import {
  BarChart3Icon,
  BoxesIcon,
  BriefcaseIcon,
  CogIcon,
  DatabaseIcon,
  HandshakeIcon,
  LandmarkIcon,
  LayoutDashboardIcon,
  type LucideIcon,
  PackageIcon,
  PlugIcon,
  ShoppingBagIcon,
  ShoppingCartIcon,
  TargetIcon,
  TruckIcon,
  UsersIcon,
  CupSodaIcon,
  WarehouseIcon,
} from "lucide-react"

/**
 * Struktur menu ERP mengikuti Naming Convention bagian 5. Menu tampil jika minimal satu modulnya
 * aktif di ENABLED_MODULES (Technical Guideline 14.5). Item tampil jika halamannya sudah dibangun
 * (`to` terisi) dan, untuk fitur opsional, jika usulan terkait diambil Balans (BRD 12.2, D-26).
 */

export type MenuItem = {
  label: string
  /** Route halaman. Kosong = halaman belum dibangun, item tidak ditampilkan. */
  to?: string
  /** Kode usulan opsional (BRD 12.2 dan anggaran di U-13); item disembunyikan sampai usulan diambil. */
  optional?: string
}

export type MenuGroup = {
  label: string
  icon: LucideIcon
  /** Modul pemilik; menu tampil jika salah satu aktif. Kosong = selalu tampil. */
  modules: string[]
  items: MenuItem[]
}

const items = (...labels: string[]): MenuItem[] => labels.map((label) => ({ label }))

export const MENU: MenuGroup[] = [
  {
    label: "Ringkasan",
    icon: LayoutDashboardIcon,
    modules: [],
    items: [{ label: "Ringkasan Operasional", to: "/" }, { label: "Dashboard Eksekutif" }],
  },
  {
    label: "Data Master",
    icon: DatabaseIcon,
    modules: ["catalog", "pricing", "party", "warehouse", "hris"],
    items: [
      ...items("Produk", "Packaging & Material", "Bundling", "Daftar Harga", "Aturan Harga", "Pelanggan", "Mitra Konsinyasi", "Reseller", "Pemasok & Maklon", "Gudang & Lokasi", "Karyawan", "Alasan Retur", "Jenis Biaya", "Kampanye"),
    ],
  },
  {
    label: "Pengadaan",
    icon: ShoppingCartIcon,
    modules: ["procurement"],
    items: items("Purchase Request", "Purchase Order", "Purchase Order Maklon", "Purchase Order Reprocess", "Persetujuan Pengadaan", "Penerimaan Barang", "Reject & Waste", "Tagihan Pemasok", "Uang Muka Pembelian"),
  },
  {
    label: "Gudang",
    icon: WarehouseIcon,
    modules: ["warehouse"],
    items: items("Inbound", "Put-away", "Transfer Stok", "Stok dalam Pengiriman", "Outbound", "Stock Count", "Gudang Retur", "Disposisi Retur", "Riwayat Mutasi Stok"),
  },
  {
    label: "Persediaan",
    icon: BoxesIcon,
    modules: ["inventory"],
    items: items("Stok per Lokasi", "Kartu Stok", "Batch & Kedaluwarsa", "Alokasi Batch (FEFO)", "HPP per Batch", "Pemakaian Internal", "Repacking", "Pemusnahan", "Peringatan Stok Minimum", "Peringatan Kedaluwarsa"),
  },
  {
    label: "Logistik",
    icon: TruckIcon,
    modules: ["logistics"],
    items: items("Delivery Order", "Pengiriman", "Kurir & Resi", "Label Pengiriman", "Pelacakan Pengiriman"),
  },
  {
    label: "Penjualan",
    icon: ShoppingBagIcon,
    modules: ["sales"],
    items: items("Sales Order", "Modern Trade", "Konsinyasi", "Penjualan Marketplace", "Pesanan WhatsApp/Instagram", "Penjualan Karyawan", "Proforma Invoice", "Invoice", "Retur Penjualan", "Nota Kredit", "Rekonsiliasi Konsinyasi", "Harga & Diskon"),
  },
  {
    label: "Pipeline Penjualan",
    icon: TargetIcon,
    modules: ["pipeline"],
    items: items("Lead", "Prospek", "Peluang Penjualan", "Aktivitas Sales", "Kunjungan", "Target Penjualan", "Pencapaian Sales"),
  },
  {
    label: "Pelanggan (CRM)",
    icon: HandshakeIcon,
    modules: ["crm"],
    items: items("Daftar Pelanggan", "Riwayat Pesanan", "Segmentasi Pelanggan", "Interaksi Customer Service"),
  },
  {
    label: "Reseller",
    icon: BriefcaseIcon,
    modules: ["reseller"],
    items: items("Daftar Reseller", "Verifikasi Reseller", "Level Reseller", "Harga Reseller", "Pesanan Reseller", "Deposit & Tagihan", "Evaluasi Level", "Portal Reseller"),
  },
  {
    label: "Mesin Vending",
    icon: CupSodaIcon,
    modules: ["vending"],
    items: items("Daftar Mesin", "Lokasi & Slot", "Stok Mesin", "Buffer Vending", "Refill", "Stock Count Mesin", "Transaksi Mesin", "Rekonsiliasi Vending", "Laporan per Mesin"),
  },
  {
    label: "E-commerce",
    icon: PackageIcon,
    modules: ["storefront"],
    items: [
      ...items("Katalog Produk", "Kategori", "Pencarian", "Keranjang", "Checkout", "Pesanan", "Pembayaran", "Voucher"),
      // Fitur lanjutan website (BRD U-16) dirilis setelah fitur launch.
      ...items("Subscription", "Member & Poin", "Ulasan & Rating", "Wishlist", "Referral", "Retur & Refund", "CMS", "SEO & Analytics"),
    ],
  },
  {
    label: "SDM (HRIS)",
    icon: UsersIcon,
    modules: ["hris", "payroll"],
    items: items("Data Karyawan", "Struktur Organisasi", "Jabatan & Kontrak", "Absensi", "Jadwal & Shift", "Cuti & Izin", "Lembur", "Reimbursement", "Payroll", "BPJS", "PPh 21", "Slip Gaji", "Self-Service"),
  },
  {
    label: "Keuangan & Akuntansi",
    icon: LandmarkIcon,
    modules: ["accounting", "tax"],
    items: [
      ...items("Ringkasan Keuangan", "Chart of Accounts", "Jurnal Umum", "Buku Besar", "Piutang Usaha", "Utang Usaha", "Kas & Bank", "Rekonsiliasi Bank", "Kas Kecil", "Biaya Operasional"),
      { label: "Anggaran Biaya", optional: "U-13" },
      ...items("Pajak & Bukti Potong", "Aset Tetap", "Penyusutan", "Tutup Buku", "Saldo Awal", "Laporan Keuangan"),
    ],
  },
  {
    label: "Laporan",
    icon: BarChart3Icon,
    modules: ["inventory", "sales", "accounting", "hris", "pipeline", "storefront", "logistics"],
    items: items("Laporan Stok", "Laporan Mutasi Stok", "Laporan Stock Count", "Laporan HPP", "Laporan Kedaluwarsa", "Laporan Penjualan", "Laporan Konsinyasi", "Laporan Retur", "Laporan Pemakaian Internal", "Laporan Pengiriman", "Laporan Piutang", "Laporan Utang", "Laporan Laba Rugi per Dimensi", "Laporan Pajak", "Laporan Pipeline", "Laporan HR", "Laporan E-commerce"),
  },
  {
    label: "Integrasi",
    icon: PlugIcon,
    modules: ["sales", "logistics", "vending", "notification", "dataexchange"],
    items: items("Midtrans", "Biteship", "Marketplace", "Mesin Vending", "WhatsApp Business", "Import/Export Excel"),
  },
  {
    label: "Pengaturan",
    icon: CogIcon,
    modules: [],
    items: [
      ...items("Pengguna & Role", "Hak Akses", "Matriks Persetujuan", "Notifikasi", "Nomor Dokumen", "Template Dokumen", "Preferensi Sistem"),
      { label: "Tentang Sistem", to: "/system/about" },
    ],
  },
]

/** Menu yang tampil untuk modul aktif dan usulan opsional yang diambil. Grup tanpa item tampil disembunyikan. */
export function visibleMenu(enabledModules: readonly string[], takenOptions: readonly string[] = []): MenuGroup[] {
  return MENU.map((group) => ({
    ...group,
    items: group.items.filter((item) => item.to && (!item.optional || takenOptions.includes(item.optional))),
  })).filter(
    (group) =>
      group.items.length > 0 && (group.modules.length === 0 || group.modules.some((m) => enabledModules.includes(m))),
  )
}
