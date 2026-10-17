import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { sourceTypeLabel, STOCK_STATUS_LABELS } from "../api/inventory-api"
import { ExpiryBadge, ExpiryCell, StockStatusBadge } from "./stock-filters"

describe("label Persediaan", () => {
  it("memakai label status stok Bahasa Indonesia", () => {
    expect(STOCK_STATUS_LABELS).toEqual({
      available: "Tersedia",
      quarantine: "Karantina",
      in_transit: "Dalam Pengiriman",
      reserved: "Dialokasikan",
    })
    render(<StockStatusBadge status="quarantine" />)
    expect(screen.getByText("Karantina")).toHaveAttribute(
      "data-status",
      "quarantine"
    )
  })

  it("menampilkan kondisi kedaluwarsa kecuali aman", () => {
    render(<ExpiryBadge state="near_expiry" />)
    expect(screen.getByText("Segera Kedaluwarsa")).toBeInTheDocument()
    const { container } = render(
      <ExpiryCell expiryDate="2026-11-01" state="ok" />
    )
    expect(container.textContent).toBe("1 Nov 2026")
  })

  it("menerjemahkan jenis dokumen sumber dan membiarkan jenis baru apa adanya", () => {
    expect(sourceTypeLabel("goods_receipt")).toBe("Penerimaan Barang")
    expect(sourceTypeLabel("new_doc")).toBe("new_doc")
  })
})
