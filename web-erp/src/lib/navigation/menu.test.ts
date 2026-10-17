import { describe, expect, it } from "vitest"

import { MENU, visibleMenu } from "./menu"

describe("menu", () => {
  it("mengikuti urutan menu utama Naming Convention bagian 5", () => {
    expect(MENU.map((g) => g.label)).toEqual([
      "Ringkasan",
      "Data Master",
      "Pengadaan",
      "Gudang",
      "Persediaan",
      "Logistik",
      "Penjualan",
      "Pipeline Penjualan",
      "Pelanggan (CRM)",
      "Reseller",
      "Mesin Vending",
      "E-commerce",
      "SDM (HRIS)",
      "Keuangan & Akuntansi",
      "Laporan",
      "Integrasi",
      "Pengaturan",
    ])
  })

  it("tanpa modul aktif hanya menampilkan halaman yang sudah dibangun", () => {
    const groups = visibleMenu([])
    expect(groups.map((g) => g.label)).toEqual(["Ringkasan", "Pengaturan"])
    expect(groups.flatMap((g) => g.items.map((i) => i.label))).toEqual([
      "Ringkasan Operasional",
      "Pengguna & Role",
      "Hak Akses",
      "Riwayat Perubahan",
      "Tentang Sistem",
    ])
  })

  it("menampilkan halaman persetujuan hanya jika modul approval aktif", () => {
    const labels = (modules: string[]) =>
      visibleMenu(modules, [], ["approval.policy.read"]).flatMap((g) =>
        g.items.map((i) => i.label)
      )
    expect(labels([])).not.toContain("Persetujuan Saya")
    expect(labels(["approval"])).toEqual(
      expect.arrayContaining(["Persetujuan Saya", "Matriks Persetujuan"])
    )
    expect(
      visibleMenu(["approval"], [], []).flatMap((g) =>
        g.items.map((i) => i.label)
      )
    ).not.toContain("Matriks Persetujuan")
  })

  it("menampilkan halaman akuntansi dan nomor dokumen jika modulnya aktif", () => {
    const perms = [
      "accounting.account.read",
      "accounting.journal.read",
      "accounting.dimension.read",
      "accounting.fiscal_period.read",
      "document.sequence.read",
    ]
    const labels = (modules: string[]) =>
      visibleMenu(modules, [], perms).flatMap((g) =>
        g.items.map((i) => `${g.label} › ${i.label}`)
      )
    expect(labels([])).not.toContain("Keuangan & Akuntansi › Jurnal Umum")
    expect(labels(["accounting", "document"])).toEqual(
      expect.arrayContaining([
        "Data Master › Kampanye",
        "Data Master › Channel & Divisi",
        "Keuangan & Akuntansi › Chart of Accounts",
        "Keuangan & Akuntansi › Jurnal Umum",
        "Keuangan & Akuntansi › Buku Besar",
        "Keuangan & Akuntansi › Periode Akuntansi",
        "Pengaturan › Nomor Dokumen",
      ])
    )
    expect(
      visibleMenu(["accounting"], [], ["accounting.account.read"]).flatMap(
        (g) => g.items.map((i) => i.label)
      )
    ).not.toContain("Jurnal Umum")
  })

  it("menampilkan Data Master dan Label Produk sesuai modul catalog/warehouse", () => {
    const perms = [
      "catalog.product.read",
      "warehouse.warehouse.read",
      "logistics.label.post",
    ]
    const group = (modules: string[], label: string, p = perms) =>
      visibleMenu(modules, [], p)
        .find((g) => g.label === label)
        ?.items.map((i) => i.label)
    expect(group(["catalog", "warehouse"], "Data Master")).toEqual([
      "Produk",
      "Packaging & Material",
      "Bundling",
      "Gudang & Lokasi",
    ])
    expect(group(["warehouse"], "Data Master")).toEqual(["Gudang & Lokasi"])
    expect(group(["catalog"], "Logistik")).toEqual(["Label Produk"])
    expect(
      group(["catalog"], "Logistik", ["logistics.label.read"]),
      "salah satu permission cukup"
    ).toEqual(["Label Produk"])
    expect(group(["catalog"], "Logistik", ["catalog.product.read"])).toBe(
      undefined
    )
  })

  it("menampilkan data master party dan karyawan sesuai modul dan permission", () => {
    const labels = (modules: string[], permissions?: string[]) =>
      visibleMenu(modules, [], permissions).flatMap((g) =>
        g.items.map((i) => `${g.label} › ${i.label}`)
      )
    expect(labels(["party", "hris"])).toEqual(
      expect.arrayContaining([
        "Data Master › Pelanggan",
        "Data Master › Mitra Konsinyasi",
        "Data Master › Kode Produk Mitra",
        "Data Master › Pemasok & Maklon",
        "Data Master › Karyawan",
        "SDM (HRIS) › Data Karyawan",
      ])
    )
    expect(labels(["party"])).not.toContain("Data Master › Karyawan")
    expect(labels(["hris"])).not.toContain("Data Master › Pelanggan")
    expect(labels(["party", "hris"], ["party.customer.read"])).toEqual(
      expect.arrayContaining(["Data Master › Pelanggan"])
    )
    expect(labels(["party", "hris"], ["party.customer.read"])).not.toContain(
      "Data Master › Pemasok & Maklon"
    )
  })

  it("menyembunyikan halaman yang permission-nya tidak dimiliki user", () => {
    const labels = visibleMenu([], [], ["identity.user.read"]).flatMap((g) =>
      g.items.map((i) => i.label)
    )
    expect(labels).toContain("Pengguna & Role")
    expect(labels).not.toContain("Hak Akses")
    expect(labels).not.toContain("Riwayat Perubahan")
    expect(labels).toContain("Tentang Sistem")
  })

  it("menyembunyikan item opsional sampai usulannya diambil", () => {
    const withOption = MENU.map((g) => ({
      ...g,
      items: g.items.map((i) => ({ ...i, to: i.to ?? "/x" })),
    }))
    const finance = withOption.find((g) => g.label === "Keuangan & Akuntansi")
    expect(
      finance?.items.some(
        (i) => i.label === "Anggaran Biaya" && i.optional === "U-13"
      )
    ).toBe(true)
  })

  it("setiap label unik di dalam grupnya", () => {
    for (const group of MENU) {
      const labels = group.items.map((i) => i.label)
      expect(new Set(labels).size, group.label).toBe(labels.length)
    }
  })

  it("menampilkan master MD-13 sesuai modul dan permission", () => {
    const labels = (modules: string[], perms: string[]) =>
      visibleMenu(modules, [], perms).flatMap((g) =>
        g.items.map((i) => `${g.label} › ${i.label}`)
      )
    const perms = ["sales.return_reason.read", "accounting.expense_type.read"]
    expect(labels(["sales", "accounting"], perms)).toEqual(
      expect.arrayContaining([
        "Data Master › Alasan Retur",
        "Data Master › Jenis Biaya",
      ])
    )
    expect(labels(["accounting"], perms)).not.toContain(
      "Data Master › Alasan Retur"
    )
    expect(labels(["sales", "accounting"], [])).not.toContain(
      "Data Master › Jenis Biaya"
    )
  })
})
