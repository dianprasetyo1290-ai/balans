import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen, within } from "@testing-library/react"
import type { ReactNode } from "react"
import { describe, expect, it } from "vitest"

import { visibleMenu } from "@/lib/navigation/menu"

import type {
  DataExchangeKind,
  ImportJob,
  ImportJobRow,
} from "../api/dataexchange-api"
import { ImportJobView } from "./import-job-view"
import { ImportSteps } from "./import-steps"

const kind: DataExchangeKind = {
  kind: "catalog.product",
  label: "Produk",
  canImport: true,
  canExport: true,
  uploadOwnerType: "import.catalog.product",
  maxFileBytes: 10 << 20,
  columns: [
    {
      key: "sku",
      header: "SKU",
      required: true,
      columnType: "text",
      options: [],
    },
    {
      key: "name",
      header: "Nama Produk",
      required: true,
      columnType: "text",
      options: [],
    },
    {
      key: "shelfLifeDays",
      header: "Umur Simpan (hari)",
      required: false,
      columnType: "integer",
      options: [],
    },
  ],
}

const baseJob: ImportJob = {
  id: "0199f000-0000-7000-8000-000000000001",
  kind: "catalog.product",
  kindLabel: "Produk",
  fileName: "produk.xlsx",
  status: "validated",
  totalRows: 5,
  processedRows: 5,
  validRows: 3,
  errorRows: 2,
  committedRows: 0,
  progress: 100,
  message: "",
  allowedActions: ["commit", "cancel", "download_errors"],
  createdBy: "0199f000-0000-7000-8000-000000000002",
  createdAt: "2026-10-17T02:00:00Z",
  updatedAt: "2026-10-17T02:01:00Z",
}

const errorRows: ImportJobRow[] = [
  {
    rowNumber: 4,
    isValid: false,
    values: { sku: "DX-A" },
    errors: [
      { column: "sku", message: "Data ganda dengan baris 2 di file yang sama" },
    ],
  },
  {
    rowNumber: 5,
    isValid: false,
    values: { sku: "DX-C" },
    errors: [
      {
        column: "shelfLifeDays",
        message: "Umur simpan wajib diisi untuk Produk Jadi",
      },
    ],
  },
]

function renderJob(job: ImportJob, node?: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  })
  client.setQueryData(["dataexchange", "import-jobs", job.id], job)
  client.setQueryData(
    ["dataexchange", "import-jobs", job.id, "rows", false, 1],
    {
      items: errorRows,
      pagination: { page: 1, pageSize: 20, totalItems: 2, totalPages: 1 },
    }
  )
  return render(
    <QueryClientProvider client={client}>
      {node ?? (
        <ImportJobView id={job.id} kind={kind} onReset={() => undefined} />
      )}
    </QueryClientProvider>
  )
}

describe("ImportJobView", () => {
  it("menampilkan ringkasan pemeriksaan, baris error, dan tombol simpan baris valid", () => {
    renderJob(baseJob)
    expect(screen.getByText("5 baris")).toBeInTheDocument()
    expect(screen.getByText("3 valid")).toBeInTheDocument()
    expect(screen.getByText("2 error")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: /Unduh baris error/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Simpan 3 baris valid" })
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Batal" })).toBeInTheDocument()

    const table = screen.getByRole("table")
    expect(within(table).getByText("Baris 4")).toBeInTheDocument()
    expect(within(table).getByText("Umur Simpan (hari)")).toBeInTheDocument()
    expect(
      within(table).getByText("Data ganda dengan baris 2 di file yang sama")
    ).toBeInTheDocument()
  })

  it("menampilkan progress saat file masih diperiksa", () => {
    renderJob({
      ...baseJob,
      status: "validating",
      totalRows: 200,
      processedRows: 50,
      progress: 25,
      allowedActions: [],
    })
    expect(screen.getByRole("dialog")).toHaveTextContent("Memeriksa file")
    expect(screen.getByText("Memeriksa baris 50 dari 200")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /Simpan/ })
    ).not.toBeInTheDocument()
  })

  it("menampilkan alasan gagal dan tombol impor ulang", () => {
    renderJob({
      ...baseJob,
      status: "failed",
      totalRows: 0,
      message: "Kolom wajib tidak ditemukan: SKU.",
      allowedActions: [],
    })
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Kolom wajib tidak ditemukan: SKU."
    )
    expect(
      screen.getByRole("button", { name: /Impor file lain/ })
    ).toBeInTheDocument()
  })
})

describe("ImportSteps", () => {
  it("menandai langkah aktif", () => {
    render(<ImportSteps current={2} />)
    const steps = screen.getAllByRole("listitem")
    expect(steps).toHaveLength(4)
    expect(steps[2]).toHaveAttribute("aria-current", "step")
  })
})

describe("menu Import/Export Excel", () => {
  const labels = (modules: string[], perms: string[]) =>
    visibleMenu(modules, [], perms).flatMap((g) => g.items.map((i) => i.label))

  it("tampil jika modul dataexchange aktif dan user punya izin impor/ekspor", () => {
    expect(labels(["dataexchange"], ["hris.employee.import"])).toContain(
      "Import/Export Excel"
    )
    expect(labels(["dataexchange"], ["hris.employee.read"])).not.toContain(
      "Import/Export Excel"
    )
    expect(labels([], ["hris.employee.import"])).not.toContain(
      "Import/Export Excel"
    )
  })
})
