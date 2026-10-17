import { describe, expect, it } from "vitest"

import { formatDate, formatDateTime, parseBusinessDate, todayBusinessDate, toBusinessDate } from "./date"
import { formatMoneyInput, formatRupiah, formatUnitCost, parseMoneyInput } from "./money"
import { formatPercent, formatQuantity, parseQuantityInput } from "./number"
import { statusLabel, STATUS_LABELS } from "./status-labels"

describe("formatRupiah", () => {
  it.each([
    ["1250000.00", "Rp1.250.000"],
    ["0", "Rp0"],
    ["999.50", "Rp1.000"],
    ["999.49", "Rp999"],
    ["-1500.00", "-Rp1.500"],
  ])("%s → %s", (input, expected) => {
    expect(formatRupiah(input)).toBe(expected)
  })

  it("menampilkan tanda strip untuk nilai kosong atau rusak", () => {
    expect(formatRupiah(null)).toBe("-")
    expect(formatRupiah("")).toBe("-")
    expect(formatRupiah("abc")).toBe("-")
  })
})

describe("formatUnitCost", () => {
  it("menampilkan HPP per unit sampai 4 desimal", () => {
    expect(formatUnitCost("12500.3333")).toBe("Rp12.500,3333")
    expect(formatUnitCost("12500.0000")).toBe("Rp12.500")
    expect(formatUnitCost("")).toBe("-")
    expect(formatUnitCost("x")).toBe("-")
  })
})

describe("input uang", () => {
  it("membaca format Indonesia menjadi string desimal API", () => {
    expect(parseMoneyInput("1.250.000")).toBe("1250000.00")
    expect(parseMoneyInput("Rp 15.000,5")).toBe("15000.50")
    expect(parseMoneyInput("")).toBeNull()
    expect(parseMoneyInput("-")).toBeNull()
  })

  it("memformat string desimal untuk ditampilkan di input", () => {
    expect(formatMoneyInput("1250000.00")).toBe("1.250.000")
    expect(formatMoneyInput("")).toBe("")
    expect(formatMoneyInput("x")).toBe("")
  })
})

describe("kuantitas & persen", () => {
  it("memformat dan membaca kuantitas", () => {
    expect(formatQuantity("1250.5")).toBe("1.250,5")
    expect(formatQuantity(undefined)).toBe("-")
    expect(parseQuantityInput("1.250,5")).toBe("1250.500")
    expect(parseQuantityInput("")).toBeNull()
  })

  it("memformat persentase", () => {
    expect(formatPercent("11.5")).toBe("11,5%")
    expect(formatPercent("")).toBe("-")
  })
})

describe("tanggal WIB", () => {
  it("menampilkan tanggal bisnis tanpa bergeser hari", () => {
    expect(formatDate("2026-10-14")).toBe("14 Okt 2026")
    expect(formatDate("2027-01-01")).toBe("1 Jan 2027")
    expect(formatDate(null)).toBe("-")
    expect(parseBusinessDate("14-10-2026")).toBeNull()
  })

  it("menampilkan timestamp UTC dalam WIB", () => {
    expect(formatDateTime("2026-10-14T02:30:00Z")).toBe("14 Okt 2026 09.30")
    expect(formatDateTime("2026-10-14T17:05:00Z")).toBe("15 Okt 2026 00.05")
    expect(formatDateTime("bukan tanggal")).toBe("-")
  })

  it("menghitung tanggal bisnis hari ini menurut WIB", () => {
    expect(todayBusinessDate(new Date("2026-10-14T16:59:00Z"))).toBe("2026-10-14")
    expect(todayBusinessDate(new Date("2026-10-14T17:00:00Z"))).toBe("2026-10-15")
    expect(toBusinessDate(new Date("2026-12-31T18:00:00Z"))).toBe("2027-01-01")
  })
})

describe("label status", () => {
  it("memakai label Naming Convention bagian 7", () => {
    expect(statusLabel("pending_approval").label).toBe("Menunggu Persetujuan")
    expect(statusLabel("qc_rejected").label).toBe("Ditolak QC")
    expect(statusLabel("posted").label).toBe("Diposting")
    expect(statusLabel("submitted").label).toBe("Diajukan")
    expect(Object.keys(STATUS_LABELS)).toHaveLength(20)
  })

  it("menampilkan kode yang belum terdaftar apa adanya", () => {
    expect(statusLabel("unknown_status")).toEqual({ label: "unknown_status", tone: "neutral" })
  })
})
