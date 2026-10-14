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
    expect(groups.flatMap((g) => g.items.map((i) => i.label))).toEqual(["Ringkasan Operasional", "Tentang Sistem"])
  })

  it("menyembunyikan item opsional sampai usulannya diambil", () => {
    const withOption = MENU.map((g) => ({ ...g, items: g.items.map((i) => ({ ...i, to: i.to ?? "/x" })) }))
    const finance = withOption.find((g) => g.label === "Keuangan & Akuntansi")
    expect(finance?.items.some((i) => i.label === "Anggaran Biaya" && i.optional === "U-13")).toBe(true)
  })

  it("setiap label unik di dalam grupnya", () => {
    for (const group of MENU) {
      const labels = group.items.map((i) => i.label)
      expect(new Set(labels).size, group.label).toBe(labels.length)
    }
  })
})
