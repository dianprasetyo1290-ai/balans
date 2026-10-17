import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type DataExchangeKind = components["schemas"]["DataExchangeKind"]
export type DataExchangeColumn = components["schemas"]["DataExchangeColumn"]
export type ImportJob = components["schemas"]["ImportJob"]
export type ImportJobRow = components["schemas"]["ImportJobRow"]
export type ExportJob = components["schemas"]["ExportJob"]
export type ImportStatus = ImportJob["status"]
export type ExportStatus = ExportJob["status"]

/** Tab halaman Integrasi › Import/Export Excel. */
export type DataExchangeTab = "import" | "export" | "history"

export const XLSX_TYPE =
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

/** Permission impor/ekspor per jenis data (`<kind>.import` / `<kind>.export`). */
export const importPermission = (kind: string) => `${kind}.import`
export const exportPermission = (kind: string) => `${kind}.export`

export const IMPORT_STATUS_LABELS: Record<ImportStatus, string> = {
  uploaded: "Menunggu Pemeriksaan",
  validating: "Diperiksa",
  validated: "Siap Disimpan",
  committing: "Menyimpan",
  completed: "Selesai",
  failed: "Gagal",
  cancelled: "Dibatalkan",
}

export const EXPORT_STATUS_LABELS: Record<ExportStatus, string> = {
  queued: "Menunggu",
  running: "Diproses",
  completed: "Selesai",
  failed: "Gagal",
}

/** Status yang masih diproses worker; halaman memantau progress dengan polling. */
export const isImportRunning = (s: ImportStatus) =>
  s === "uploaded" || s === "validating" || s === "committing"
export const isExportRunning = (s: ExportStatus) =>
  s === "queued" || s === "running"

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

const POLL_MS = 1500

export const dataExchangeKindsQuery = queryOptions({
  queryKey: ["dataexchange", "kinds"],
  queryFn: () => unwrap(api.GET("/api/v1/data-exchange/kinds")),
  staleTime: 60_000,
})

export const importJobQuery = (id: string) =>
  queryOptions({
    queryKey: ["dataexchange", "import-jobs", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/import-jobs/{id}", { params: { path: { id } } })),
    refetchInterval: (q) =>
      q.state.data && isImportRunning(q.state.data.status) ? POLL_MS : false,
  })

export const importRowsQuery = (id: string, valid: boolean, page: number) =>
  queryOptions({
    queryKey: ["dataexchange", "import-jobs", id, "rows", valid, page],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/import-jobs/{id}/rows", {
          params: { path: { id }, query: { valid, page, pageSize: 20 } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const importJobsQuery = (page: number, kind?: string) =>
  queryOptions({
    queryKey: ["dataexchange", "import-jobs", "list", page, kind],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/import-jobs", {
          params: { query: { page, pageSize: 20, kind } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const exportJobsQuery = (page: number) =>
  queryOptions({
    queryKey: ["dataexchange", "export-jobs", "list", page],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/export-jobs", {
          params: { query: { page, pageSize: 20 } },
        })
      ),
    placeholderData: keepPreviousData,
    refetchInterval: (q) =>
      q.state.data?.items.some((j) => isExportRunning(j.status))
        ? POLL_MS
        : false,
  })

/** Menyimpan blob sebagai file unduhan di browser. */
export function saveBlob(blob: Blob, fileName: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = fileName
  document.body.append(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

/** Mengunduh template impor yang dihasilkan sistem dari definisi kolom. */
export async function downloadTemplate(kind: DataExchangeKind) {
  const { data, response } = await api.GET(
    "/api/v1/data-exchange/kinds/{kind}/template",
    { params: { path: { kind: kind.kind } }, parseAs: "blob" }
  )
  if (!response.ok || !data) {
    throw new Error(`template ${response.status}`)
  }
  saveBlob(data, `Template ${kind.label}.xlsx`)
}

/** Meminta URL upload file xlsx impor (Technical Guideline 5.16). */
export async function requestImportUpload(kind: DataExchangeKind, file: File) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType: kind.uploadOwnerType,
        fileName: file.name,
        contentType: XLSX_TYPE,
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

export function createImportJob(kind: string, fileId: string, key: string) {
  return unwrap(
    api.POST("/api/v1/import-jobs", {
      params: idem(key),
      body: { kind, fileId },
    })
  )
}

export function commitImportJob(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/import-jobs/{id}/commit", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function cancelImportJob(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/import-jobs/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function createExportJob(kind: string, key: string) {
  return unwrap(
    api.POST("/api/v1/export-jobs", { params: idem(key), body: { kind } })
  )
}

/** Membuka URL unduhan bertanda tangan (berumur 5 menit). */
function openUrl(url: string) {
  window.open(url, "_blank", "noopener")
}

export async function downloadImportErrors(id: string) {
  const link = await unwrap(
    api.GET("/api/v1/import-jobs/{id}/error-file", { params: { path: { id } } })
  )
  openUrl(link.url)
}

export async function downloadExport(id: string) {
  const link = await unwrap(
    api.GET("/api/v1/export-jobs/{id}/download-url", {
      params: { path: { id } },
    })
  )
  openUrl(link.url)
}
