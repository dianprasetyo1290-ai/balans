import { describe, expect, it } from "vitest"

import { varianceOf } from "./consignment-api"

describe("varianceOf", () => {
  it("menghitung selisih terjual sistem − laporan mitra", () => {
    expect(varianceOf("20.000", "17")).toBe(3)
    expect(varianceOf("4.000", "4.000")).toBe(0)
    expect(varianceOf("0.3", "0.1")).toBe(0.2)
  })

  it("isian tidak valid dianggap tanpa selisih", () => {
    expect(varianceOf("20", "abc")).toBe(0)
  })
})
