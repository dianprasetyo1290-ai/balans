import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import {
  DISPOSAL_REASON_LABELS,
  DISPOSITION_LABELS,
  PURPOSE_LABELS,
} from "../api/documents-api"
import { DocActionButtons, DocMovementsTable, stepText } from "./doc-shared"

describe("dokumen persediaan", () => {
  it("memakai label Naming Convention untuk tujuan, alasan, dan disposisi", () => {
    expect(PURPOSE_LABELS.employee_sample).toBe("Sampel Karyawan")
    expect(DISPOSAL_REASON_LABELS.return).toBe("Barang Retur")
    expect(Object.keys(DISPOSITION_LABELS)).toEqual([
      "restock",
      "repack",
      "reprocess",
      "marketing_sample",
      "dispose",
    ])
  })

  it("hanya menampilkan tombol aksi yang diizinkan backend", () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <DocActionButtons
          specs={[
            {
              action: "submit",
              label: "Ajukan",
              confirmTitle: "",
              confirmDescription: "",
            },
            {
              action: "cancel",
              label: "Batalkan",
              confirmTitle: "",
              confirmDescription: "",
            },
          ]}
          allowed={["submit"]}
          run={() => Promise.resolve()}
        />
      </QueryClientProvider>
    )
    expect(screen.getByRole("button", { name: "Ajukan" })).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Batalkan" })).toBeNull()
  })

  it("menampilkan pelaku langkah dan tabel mutasi kosong", () => {
    expect(stepText(null)).toBe("—")
    render(<DocMovementsTable movements={[]} />)
    expect(screen.getByText("Belum ada mutasi stok.")).toBeInTheDocument()
  })
})
