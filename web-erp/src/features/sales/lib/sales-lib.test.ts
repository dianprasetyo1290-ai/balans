import { describe, expect, it } from "vitest"

import {
  autoAllocate,
  discountAmount,
  orderPreview,
  overdueLabel,
  sumAmounts,
} from "./sales-lib"

describe("sales-lib", () => {
  it("menghitung diskon persen dan nominal seperti backend", () => {
    expect(discountAmount("200000", "percent", "12.5")).toBe("25000.00")
    expect(discountAmount("100", "percent", "33.333")).toBe("33.33")
    expect(discountAmount("200000", "amount", "1500.5")).toBe("1500.50")
    expect(discountAmount("1000", "amount", "5000")).toBe("1000.00")
    expect(discountAmount("1000", "amount", "")).toBe("0.00")
    expect(discountAmount("1000", "percent", "abc")).toBe("0.00")
  })

  it("preview total: bruto − diskon baris − diskon dokumen + ongkir (tanpa PPN)", () => {
    const p = orderPreview(
      [
        {
          qty: "10",
          unitPrice: "100000",
          discountKind: "percent",
          discountValue: "10",
        },
        {
          qty: "2",
          unitPrice: "220000",
          discountKind: "amount",
          discountValue: "",
        },
      ],
      { kind: "amount", value: "40000" },
      "15000"
    )
    expect(p).toEqual({
      gross: "1440000.00",
      lineDiscount: "100000.00",
      subtotal: "1340000.00",
      documentDiscount: "40000.00",
      shipping: "15000.00",
      total: "1315000.00",
    })
  })

  it("mengalokasikan pembayaran ke invoice berurutan", () => {
    expect(
      autoAllocate("250000", [
        { id: "a", outstandingAmount: "200000" },
        { id: "b", outstandingAmount: "100000" },
        { id: "c", outstandingAmount: "50000" },
      ])
    ).toEqual({ a: "200000.00", b: "50000.00" })
    expect(sumAmounts(["1", "2.5", ""])).toBe("3.50")
  })

  it("label umur piutang", () => {
    expect(overdueLabel(0)).toBe("Belum jatuh tempo")
    expect(overdueLabel(12)).toBe("Lewat 12 hari")
  })
})
