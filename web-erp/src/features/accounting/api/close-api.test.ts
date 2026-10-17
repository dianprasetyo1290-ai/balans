import { describe, expect, it } from "vitest"

import {
  periodLabel,
  periodOptions,
  periodRange,
  previousPeriod,
} from "./close-api"

describe("utilitas periode tutup buku", () => {
  it("rentang periode mengikuti jumlah hari bulan", () => {
    expect(periodRange(2026, 10)).toEqual(["2026-10-01", "2026-10-31"])
    expect(periodRange(2028, 2)).toEqual(["2028-02-01", "2028-02-29"])
  })

  it("periode sebelumnya untuk penyusutan dan Neraca Saldo", () => {
    expect(previousPeriod("2026-10-17")).toEqual({ year: 2026, month: 9 })
    expect(previousPeriod("2027-01-01")).toEqual({ year: 2026, month: 12 })
  })

  it("pilihan periode terbaru dulu termasuk bulan depan", () => {
    const opts = periodOptions("2026-12-05", 3)
    expect(opts).toEqual([
      { year: 2027, month: 1 },
      { year: 2026, month: 12 },
      { year: 2026, month: 11 },
      { year: 2026, month: 10 },
    ])
    expect(periodLabel(2026, 10)).toBe("Okt 2026")
  })
})
