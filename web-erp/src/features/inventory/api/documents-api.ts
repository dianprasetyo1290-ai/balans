import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

type S = components["schemas"]

export type InventoryDocStatus = S["InventoryDocStatus"]
export type InventoryDocAction = S["InventoryDocAction"]
export type InventoryDocMovement = S["InventoryDocMovement"]
export type InventoryDocStep = S["InventoryDocStep"]
export type InternalUsage = S["InternalUsage"]
export type InternalUsageInput = S["InternalUsageInput"]
export type InternalUsagePurpose = S["InternalUsagePurpose"]
export type InternalUsageAction = S["InternalUsageAction"]
export type Repacking = S["Repacking"]
export type RepackingInput = S["RepackingInput"]
export type RepackingAction = S["RepackingAction"]
export type Disposal = S["Disposal"]
export type DisposalInput = S["DisposalInput"]
export type DisposalReason = S["DisposalReason"]
export type DisposalAction = S["DisposalAction"]
export type DisposalReportFields = S["DisposalReportFields"]
export type ReturnDispositionDocument = S["ReturnDispositionDocument"]
export type ReturnDispositionDocumentInput = S["ReturnDispositionDocumentInput"]
export type ReturnDispositionAction = S["ReturnDispositionAction"]
export type ReturnDisposition = S["ReturnDisposition"]
export type InventoryDimensionOption = S["InventoryDimensionOption"]

/** Label tujuan Pemakaian Internal (INV-05, HR-02). */
export const PURPOSE_LABELS: Record<InternalUsagePurpose, string> = {
  marketing: "Marketing",
  event: "Event",
  employee_sample: "Sampel Karyawan",
  sales_sampling: "Sampling Sales",
}

export const DISPOSAL_REASON_LABELS: Record<DisposalReason, string> = {
  expired: "Kedaluwarsa",
  damaged: "Rusak",
  return: "Barang Retur",
}

/** Label disposisi barang retur (BRD 5.11, U-09). */
export const DISPOSITION_LABELS: Record<ReturnDisposition, string> = {
  restock: "Kembali ke stok",
  repack: "Repacking",
  reprocess: "Reprocess maklon",
  marketing_sample: "Sampel/marketing",
  dispose: "Pemusnahan",
}

export const DOC_STATUSES: InventoryDocStatus[] = [
  "draft",
  "pending_approval",
  "approved",
  "rejected",
  "in_progress",
  "awaiting_settlement",
  "posted",
  "cancelled",
]

export type DocListSearch = {
  page: number
  q?: string
  status?: InventoryDocStatus
}

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

// --- Pemakaian Internal -------------------------------------------------------------------------

export const internalUsagesQuery = (
  s: DocListSearch & { purpose?: InternalUsagePurpose }
) =>
  queryOptions({
    queryKey: ["inventory", "internal-usages", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/internal-usages", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              purpose: s.purpose,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const internalUsageQuery = (id: string) =>
  queryOptions({
    queryKey: ["inventory", "internal-usage", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/internal-usages/{id}", { params: { path: { id } } })
      ),
  })

export const dimensionOptionsQuery = (kind: "division" | "campaign") =>
  queryOptions({
    queryKey: ["inventory", "dimension-options", kind],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/internal-usage-options/{kind}", {
          params: { path: { kind } },
        })
      ),
    staleTime: 60_000,
  })

export function createInternalUsage(body: InternalUsageInput, key: string) {
  return unwrap(
    api.POST("/api/v1/internal-usages", { params: idem(key), body })
  )
}

export function updateInternalUsage(
  id: string,
  body: InternalUsageInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/internal-usages/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function runInternalUsageAction(
  id: string,
  action: InternalUsageAction,
  version: number,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/internal-usages/{id}/actions/{action}", {
      params: { path: { id, action }, ...idem(key) },
      body: { version },
    })
  )
}

export function settleInternalUsage(
  id: string,
  version: number,
  lines: { lineId: string; returnedQty: string }[],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/internal-usages/{id}/settle", {
      params: { path: { id }, ...idem(key) },
      body: { version, lines },
    })
  )
}

// --- Repacking ----------------------------------------------------------------------------------

export const repackingsQuery = (s: DocListSearch) =>
  queryOptions({
    queryKey: ["inventory", "repackings", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/repackings", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const repackingQuery = (id: string) =>
  queryOptions({
    queryKey: ["inventory", "repacking", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/repackings/{id}", { params: { path: { id } } })),
  })

export function createRepacking(body: RepackingInput, key: string) {
  return unwrap(api.POST("/api/v1/repackings", { params: idem(key), body }))
}

export function updateRepacking(
  id: string,
  body: RepackingInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/repackings/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function runRepackingAction(
  id: string,
  action: RepackingAction,
  version: number,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/repackings/{id}/actions/{action}", {
      params: { path: { id, action }, ...idem(key) },
      body: { version },
    })
  )
}

// --- Pemusnahan ---------------------------------------------------------------------------------

export const disposalsQuery = (
  s: DocListSearch & { reason?: DisposalReason }
) =>
  queryOptions({
    queryKey: ["inventory", "disposals", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/disposals", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              reason: s.reason,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const disposalQuery = (id: string) =>
  queryOptions({
    queryKey: ["inventory", "disposal", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/disposals/{id}", { params: { path: { id } } })),
  })

export function createDisposal(body: DisposalInput, key: string) {
  return unwrap(api.POST("/api/v1/disposals", { params: idem(key), body }))
}

export function updateDisposal(
  id: string,
  body: DisposalInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/disposals/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function updateDisposalReport(
  id: string,
  body: DisposalReportFields & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/disposals/{id}/report", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function runDisposalAction(
  id: string,
  action: DisposalAction,
  version: number,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/disposals/{id}/actions/{action}", {
      params: { path: { id, action }, ...idem(key) },
      body: { version },
    })
  )
}

/** Meminta URL upload foto berita acara pemusnahan (Technical Guideline 5.16). */
export async function requestDisposalPhotoUpload(file: File) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType: "disposal",
        fileName: file.name,
        contentType: file.type || "image/jpeg",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

export const fileUrlQuery = (id: string) =>
  queryOptions({
    queryKey: ["files", "download-url", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/files/{id}/download-url", {
          params: { path: { id } },
        })
      ),
    staleTime: 4 * 60_000,
  })

// --- Disposisi Retur ----------------------------------------------------------------------------

export const returnDispositionsQuery = (s: DocListSearch) =>
  queryOptions({
    queryKey: ["inventory", "return-dispositions", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/return-dispositions", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const returnDispositionQuery = (id: string) =>
  queryOptions({
    queryKey: ["inventory", "return-disposition", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/return-dispositions/{id}", {
          params: { path: { id } },
        })
      ),
  })

export function createReturnDisposition(
  body: ReturnDispositionDocumentInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/return-dispositions", { params: idem(key), body })
  )
}

export function updateReturnDisposition(
  id: string,
  body: ReturnDispositionDocumentInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/return-dispositions/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function runReturnDispositionAction(
  id: string,
  action: ReturnDispositionAction,
  version: number,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/return-dispositions/{id}/actions/{action}", {
      params: { path: { id, action }, ...idem(key) },
      body: { version },
    })
  )
}

export const returnReasonOptionsQuery = queryOptions({
  queryKey: ["sales", "return-reasons", "options"],
  queryFn: () =>
    unwrap(api.GET("/api/v1/return-reasons", { params: { query: {} } })),
  staleTime: 60_000,
})

/** Lokasi aktif untuk form dokumen (maks. 100), difilter jenis lokasinya di klien. */
export const docLocationsQuery = queryOptions({
  queryKey: ["warehouse", "locations", "doc-options"],
  queryFn: () =>
    unwrap(
      api.GET("/api/v1/locations", { params: { query: { pageSize: 100 } } })
    ),
  staleTime: 60_000,
})

/** Saldo stok satu lokasi untuk memilih baris dokumen (produk × batch × status). */
export const locationStockQuery = (locationId: string) =>
  queryOptions({
    queryKey: ["inventory", "balances", "location", locationId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-balances", {
          params: {
            query: {
              locationId: [locationId],
              status: ["available", "quarantine"],
              pageSize: 200,
            },
          },
        })
      ),
  })

/** Riwayat persetujuan satu dokumen (panel Persetujuan). */
export const subjectApprovalsQuery = (subjectType: string, subjectId: string) =>
  queryOptions({
    queryKey: ["approval", "subject", subjectType, subjectId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-requests", {
          params: { query: { subjectType, subjectId, page: 1 } },
        })
      ),
  })
