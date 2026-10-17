import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { Barcode } from "@/components/common/barcode"

import type { Product } from "../api/catalog-api"
import {
  expandLabels,
  MAX_LABELS_PER_ROW,
  MAX_LABELS_TOTAL,
  printStyles,
  ProductLabel,
} from "./product-labels-page"

const product: Product = {
  id: "p1",
  sku: "MS-COK-30",
  name: "Meal Shake",
  variant: "Coklat 30 g",
  productType: "finished_good",
  uom: "pcs",
  hasBatch: true,
  isBundle: false,
  isActive: true,
  barcodes: [{ barcode: "8991234567891", symbology: "ean13", isPrimary: true }],
  primaryBarcode: "8991234567891",
  bundleComponents: [],
  createdAt: "2026-10-19T02:00:00Z",
  updatedAt: "2026-10-19T02:00:00Z",
  version: 1,
  allowedActions: [],
}

const row = (qty: number) => ({
  key: crypto.randomUUID(),
  product,
  qty,
  batch: "",
  expiry: "",
})

describe("label produk", () => {
  it("menyusun label sesuai jumlah per baris dengan batas", () => {
    expect(expandLabels([row(3), row(2)])).toHaveLength(5)
    expect(expandLabels([row(0), row(-1), row(Number.NaN)])).toHaveLength(0)
    expect(expandLabels([row(10_000)])).toHaveLength(MAX_LABELS_PER_ROW)
    expect(
      expandLabels(Array.from({ length: 10 }, () => row(MAX_LABELS_PER_ROW)))
    ).toHaveLength(MAX_LABELS_TOTAL)
  })

  it("ukuran halaman cetak mengikuti format", () => {
    expect(printStyles("thermal")).toContain("size: 50mm 30mm")
    expect(printStyles("a4")).toContain("size: A4")
    expect(printStyles("a4")).toContain("#label-print-root")
  })

  it("label memuat nama, SKU, barcode utama, batch, dan kedaluwarsa", () => {
    render(
      <ProductLabel
        item={{ key: "k", product, batch: "B2312", expiry: "2027-12-12" }}
      />
    )
    expect(screen.getByText("Meal Shake Coklat 30 g")).toBeInTheDocument()
    expect(screen.getByText("MS-COK-30")).toBeInTheDocument()
    expect(screen.getByText("Batch B2312")).toBeInTheDocument()
    expect(screen.getByText("ED 12 Des 2027")).toBeInTheDocument()
    expect(
      screen.getByRole("img", { name: "Barcode 8991234567891" })
    ).toBeInTheDocument()
  })
})

describe("Barcode", () => {
  it("merender batang SVG dan teks", () => {
    const { container } = render(<Barcode value="MS-COK-30" />)
    expect(container.querySelectorAll("rect").length).toBeGreaterThan(10)
    expect(screen.getByText("MS-COK-30")).toBeInTheDocument()
  })

  it("menampilkan pesan jika barcode tidak bisa dirender", () => {
    render(<Barcode value="123" symbology="ean13" />)
    expect(screen.getByText("Barcode tidak valid")).toBeInTheDocument()
  })
})
