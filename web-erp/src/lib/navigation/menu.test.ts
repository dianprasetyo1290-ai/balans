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
})
