import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Inbound = components["schemas"]["Inbound"]
export type InboundSummary = components["schemas"]["InboundSummary"]
export type InboundLine = components["schemas"]["InboundLine"]
export type InboundStatus = components["schemas"]["InboundStatus"]
export type InboundQcInput = components["schemas"]["InboundQcInput"]
export type StockTransfer = components["schemas"]["StockTransfer"]
export type StockTransferSummary = components["schemas"]["StockTransferSummary"]
export type StockTransferInput = components["schemas"]["StockTransferInput"]
export type StockTransferType = components["schemas"]["StockTransferType"]
export type StockTransferStatus = components["schemas"]["StockTransferStatus"]
export type StockTransferAction = components["schemas"]["StockTransferAction"]
export type StockTransferActionInput =
  components["schemas"]["StockTransferActionInput"]
export type StockInTransitLine = components["schemas"]["StockInTransitLine"]
export type StockCount = components["schemas"]["StockCount"]
export type StockCountSummary = components["schemas"]["StockCountSummary"]
export type StockCountLine = components["schemas"]["StockCountLine"]
export type StockCountType = components["schemas"]["StockCountType"]
export type StockCountStatus = components["schemas"]["StockCountStatus"]
export type StockCountAction = components["schemas"]["StockCountAction"]
export type StockCountEntriesInput =
  components["schemas"]["StockCountEntriesInput"]
export type StockCountSchedule = components["schemas"]["StockCountSchedule"]
export type StockCountScheduleInput =
  components["schemas"]["StockCountScheduleInput"]
export type StockCountFrequency = components["schemas"]["StockCountFrequency"]
export type WhLocationRef = components["schemas"]["WhLocationRef"]
export type WhBatchRef = components["schemas"]["WhBatchRef"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

/** Label jenis Stock Count (Naming Convention: Stock Count). */
export const COUNT_TYPE_LABELS: Record<StockCountType, string> = {
  full: "Penuh",
  cycle: "Cycle count",
  consignment: "Konsinyasi",
  vending: "Vending",
}

export const COUNT_TYPES: StockCountType[] = [
  "cycle",
  "full",
  "consignment",
  "vending",
]

export const FREQUENCY_LABELS: Record<StockCountFrequency, string> = {
  weekly: "Mingguan",
  monthly: "Bulanan",
  yearly: "Tahunan",
}

export const WEEKDAY_LABELS = [
  "Senin",
  "Selasa",
  "Rabu",
  "Kamis",
  "Jumat",
  "Sabtu",
  "Minggu",
]

export const MONTH_LABELS = [
  "Januari",
  "Februari",
  "Maret",
  "April",
  "Mei",
  "Juni",
  "Juli",
  "Agustus",
  "September",
  "Oktober",
  "November",
  "Desember",
]

/** Label tombol aksi dokumen gudang. */
export const TRANSFER_ACTION_LABELS: Partial<
  Record<StockTransferAction, string>
> = {
  submit: "Ajukan Permintaan",
  release: "Release",
  prepare: "Siapkan Barang",
  post: "Posting Transfer",
  ship: "Kirim (Dalam Pengiriman)",
  receive: "Konfirmasi Terima",
  complete: "Selesaikan",
  cancel: "Batalkan",
}

export function locationLabel(l: WhLocationRef) {
  return `${l.code} · ${l.name}`
}

export function batchLabel(b?: WhBatchRef | null) {
  return b ? b.number : "—"
}

// --- Inbound ---------------------------------------------------------------

export type InboundSearch = {
  page: number
  q?: string
  status?: InboundStatus
}

export const inboundsQuery = (s: InboundSearch) =>
  queryOptions({
    queryKey: ["warehouse", "inbounds", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/inbounds", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const inboundQuery = (id: string) =>
  queryOptions({
    queryKey: ["warehouse", "inbound", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/inbounds/{id}", { params: { path: { id } } })),
  })

export function recordQc(
  id: string,
  lineId: string,
  body: InboundQcInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/inbounds/{id}/lines/{lineId}/qc", {
      params: { path: { id, lineId }, ...idem(key) },
      body,
    })
  )
}

// --- Transfer Stok ---------------------------------------------------------

export type TransferSearch = {
  page: number
  q?: string
  status?: StockTransferStatus
}

export const transfersQuery = (type: StockTransferType, s: TransferSearch) =>
  queryOptions({
    queryKey: ["warehouse", "transfers", type, s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-transfers", {
          params: {
            query: {
              transferType: type,
              page: s.page,
              q: s.q || undefined,
              status: s.status ? [s.status] : undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const transferQuery = (id: string) =>
  queryOptions({
    queryKey: ["warehouse", "transfer", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-transfers/{id}", { params: { path: { id } } })
      ),
  })

export const inTransitQuery = (page: number, locationId?: string) =>
  queryOptions({
    queryKey: ["warehouse", "in-transit", page, locationId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-transfers/in-transit", {
          params: { query: { page, locationId } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createTransfer(body: StockTransferInput, key: string) {
  return unwrap(
    api.POST("/api/v1/stock-transfers", { params: idem(key), body })
  )
}

export function updateTransfer(
  id: string,
  body: StockTransferInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/stock-transfers/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function actOnTransfer(
  id: string,
  body: StockTransferActionInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/stock-transfers/{id}/actions", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

// --- Stock Count -----------------------------------------------------------

export type CountSearch = {
  page: number
  q?: string
  status?: StockCountStatus
  countType?: StockCountType
}

export const stockCountsQuery = (s: CountSearch) =>
  queryOptions({
    queryKey: ["warehouse", "stock-counts", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-counts", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status ? [s.status] : undefined,
              countType: s.countType,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const stockCountQuery = (id: string) =>
  queryOptions({
    queryKey: ["warehouse", "stock-count", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-counts/{id}", { params: { path: { id } } })
      ),
  })

export function createStockCounts(
  body: components["schemas"]["StockCountInput"],
  key: string
) {
  return unwrap(api.POST("/api/v1/stock-counts", { params: idem(key), body }))
}

export function saveCountEntries(
  id: string,
  body: StockCountEntriesInput,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/stock-counts/{id}/entries", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function actOnStockCount(
  id: string,
  action: StockCountAction,
  version: number,
  key: string,
  reason?: string
) {
  return unwrap(
    api.POST("/api/v1/stock-counts/{id}/actions", {
      params: { path: { id }, ...idem(key) },
      body: { action, version, reason },
    })
  )
}

export const schedulesQuery = queryOptions({
  queryKey: ["warehouse", "stock-count-schedules"],
  queryFn: () => unwrap(api.GET("/api/v1/stock-count-schedules")),
})

export function saveSchedule(
  body: StockCountScheduleInput,
  key: string,
  id?: string,
  version?: number
) {
  return id
    ? unwrap(
        api.PUT("/api/v1/stock-count-schedules/{id}", {
          params: { path: { id }, ...idem(key) },
          body: { ...body, version: version ?? 1 },
        })
      )
    : unwrap(
        api.POST("/api/v1/stock-count-schedules", { params: idem(key), body })
      )
}

// --- Foto ------------------------------------------------------------------

/** Meminta URL upload foto QC / stock count (Technical Guideline 5.16). */
export async function requestPhotoUpload(
  ownerType: "quality_check" | "stock_count",
  file: File
) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType,
        fileName: file.name,
        contentType: file.type || "image/jpeg",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

export const MAX_PHOTO_BYTES = 5 * 1024 * 1024
