import { describe, expect, it } from "vitest"

import {
  changePercent,
  monthLabel,
  monthStart,
  resolvePeriod,
  settlementSearchSchema,
  toNumber,
} from "./reports-lib"

describe("reports-lib", () => {
  it("periode default awal bulan s.d. hari ini", () => {
    expect(resolvePeriod({}, "2026-11-17")).toEqual({
      dateFrom: "2026-11-01",
      dateTo: "2026-11-17",
    })
    expect(resolvePeriod({ dateTo: "2026-10-05" }, "2026-11-17")).toEqual({
      dateFrom: "2026-10-01",
      dateTo: "2026-10-05",
    })
    expect(monthStart("2026-02-28")).toBe("2026-02-01")
  })

  it("format bulan dan perubahan persen", () => {
    expect(monthLabel("2026-08-01")).toBe("Agu 26")
    expect(changePercent("150", "100")).toBe(50)
    expect(changePercent("50", "0")).toBeNull()
    expect(toNumber(undefined)).toBe(0)
    expect(toNumber("abc")).toBe(0)
  })

  it("filter settlement di URL dengan nilai tidak valid diabaikan", () => {
    const s = settlementSearchSchema.parse({
      page: 0,
      status: "x",
      tab: "receivables",
    })
    expect(s.page).toBe(1)
    expect(s.status).toBeUndefined()
    expect(s.tab).toBe("receivables")
  })
})
