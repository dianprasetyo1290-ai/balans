import { describe, expect, it } from "vitest"

import { orderKindOf } from "../api/procurement-api"
import {
  documentTotal,
  lineAmount,
  priceVariancePercent,
  terbilang,
} from "./procurement-lib"

describe("lineAmount & documentTotal", () => {
  it("membulatkan half-up ke 2 desimal seperti backend", () => {
    expect(lineAmount("1", "0.005")).toBe("0.01")
    expect(lineAmount("3.333", "1000.10")).toBe("3333.33")
    expect(lineAmount("", "1000")).toBe("0.00")
    expect(lineAmount("abc", "1000")).toBe("0.00")
  })

  it("menjumlah baris tanpa PPN", () => {
    expect(
      documentTotal([
        { qty: "10000", price: "1250" },
        { qty: "3", price: "0.33" },
      ])
    ).toBe("12500000.99")
    expect(documentTotal([])).toBe("0.00")
  })
})

describe("terbilang", () => {
  it.each([
    ["0", "nol rupiah"],
    ["1", "satu rupiah"],
    ["11", "sebelas rupiah"],
    ["15", "lima belas rupiah"],
    ["100", "seratus rupiah"],
    ["1000", "seribu rupiah"],
    ["1250000", "satu juta dua ratus lima puluh ribu rupiah"],
    ["56000000.00", "lima puluh enam juta rupiah"],
    ["2001001", "dua juta seribu satu rupiah"],
    ["1000000000", "satu miliar rupiah"],
    ["999999.6", "satu juta rupiah"],
  ])("%s → %s", (value, words) => {
    expect(terbilang(value)).toBe(words)
  })

  it("mengabaikan input tidak valid", () => {
    expect(terbilang("x")).toBe("")
  })
})

describe("orderKindOf", () => {
  it("memetakan jenis PO ke menu", () => {
    expect(orderKindOf("toll_manufacturing")).toBe("toll")
    expect(orderKindOf("reprocess")).toBe("reprocess")
    expect(orderKindOf("packaging")).toBe("regular")
  })
})

describe("priceVariancePercent", () => {
  it("menghitung selisih harga tagihan terhadap PO untuk toleransi 1%", () => {
    expect(priceVariancePercent("20500", "20000").toFixed(2)).toBe("2.50")
    expect(priceVariancePercent("1262.5", "1250").toFixed(2)).toBe("1.00")
    expect(priceVariancePercent("990", "1000").toFixed(2)).toBe("-1.00")
    expect(priceVariancePercent("1", "0").toFixed(0)).toBe("100")
    expect(priceVariancePercent("", "0").toFixed(0)).toBe("0")
  })
})
