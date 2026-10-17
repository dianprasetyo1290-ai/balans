import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { distributePackages } from "@/features/warehouse/api/outbound-api"

import { colliLabels, type DeliveryOrder } from "../api/logistics-api"
import { ColliLabelView, partnerLabelContent } from "./print-pages"

const product = {
  id: "p1",
  sku: "COK-01",
  name: "Coklat 250 g",
  uom: "pcs",
  barcode: "8991234567890",
}

function deliveryOrder(): DeliveryOrder {
  return {
    id: "do1",
    number: "DO/2026/11/00001",
    deliveryType: "sales",
    sourceType: "sales_order",
    sourceId: "so1",
    sourceLabel: "SO/2026/11/00001",
    recipientName: "Toko Sehat",
    recipientPhone: "0812",
    shipToAddress: "Jl. Melati 5",
    businessDate: "2026-11-02",
    status: "in_progress",
    lineCount: 1,
    createdAt: "2026-11-02T02:00:00Z",
    location: {
      id: "l1",
      code: "UTAMA-FG",
      name: "FG",
      locationType: "finished_goods",
      locationTypeLabel: "FG",
      isExternal: false,
    },
    note: "",
    releasedByName: "",
    cancelReason: "",
    lines: [],
    printLines: [],
    productLabels: [],
    events: [],
    allowedActions: [],
    version: 1,
    packages: [
      {
        packageNumber: 1,
        packageCount: 2,
        weightGrams: 1500,
        items: [{ product, batch: { id: "b1", number: "B1" }, qty: "7.000" }],
      },
      { packageNumber: 2, packageCount: 2, items: [{ product, qty: "5.000" }] },
    ],
  }
}

describe("label koli & label produk mitra (S-316)", () => {
  it("menyusun satu label per koli dengan n/total, penerima, dan isi", () => {
    const labels = colliLabels(deliveryOrder())
    expect(labels).toHaveLength(2)
    expect(labels[0]).toMatchObject({
      doNumber: "DO/2026/11/00001",
      n: 1,
      total: 2,
      recipient: "Toko Sehat",
      weightGrams: 1500,
    })
    expect(labels[0].contents).toEqual(["COK-01 Coklat 250 g × 7 (Batch B1)"])
    render(<ColliLabelView label={labels[1]} />)
    expect(screen.getByText("2/2")).toBeInTheDocument()
    expect(screen.getByText("Toko Sehat")).toBeInTheDocument()
  })

  it("memakai kode artikel & barcode mitra bila ada", () => {
    const withCode = partnerLabelContent({
      product,
      qty: "1.000",
      partnerArticleCode: "ART-77",
      partnerBarcode: "2000000000017",
      partnerCodeMissing: false,
    })
    expect(withCode).toMatchObject({
      articleCode: "ART-77",
      barcode: "2000000000017",
    })
    const without = partnerLabelContent({
      product,
      qty: "1.000",
      partnerCodeMissing: true,
    })
    expect(without).toMatchObject({
      articleCode: undefined,
      barcode: "8991234567890",
      sku: "COK-01",
    })
  })

  it("membagi baris ke koli secara bergiliran tanpa koli kosong", () => {
    const lines = [
      { lineId: "a", qty: "1" },
      { lineId: "b", qty: "2" },
      { lineId: "c", qty: "3" },
    ]
    expect(
      distributePackages(lines, 2).map((p) => p.items.map((i) => i.lineId))
    ).toEqual([["a", "c"], ["b"]])
    expect(distributePackages(lines, 9)).toHaveLength(3)
    expect(distributePackages(lines, 0)).toHaveLength(1)
  })
})
