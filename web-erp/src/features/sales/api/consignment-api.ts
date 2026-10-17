import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type ConsignmentDelivery = components["schemas"]["ConsignmentDelivery"]
export type ConsignmentDeliverySummary =
  components["schemas"]["ConsignmentDeliverySummary"]
export type ConsignmentDeliveryType =
  components["schemas"]["ConsignmentDeliveryType"]
export type ConsignmentDeliveryStatus =
  components["schemas"]["ConsignmentDeliveryStatus"]
export type ConsignmentDeliveryLineInput =
  components["schemas"]["ConsignmentDeliveryLineInput"]
export type ConsignmentReconciliation =
  components["schemas"]["ConsignmentReconciliation"]
export type ConsignmentReconciliationSummary =
  components["schemas"]["ConsignmentReconciliationSummary"]
export type ConsignmentReconciliationStatus =
  components["schemas"]["ConsignmentReconciliationStatus"]
export type ConsignmentReconciliationLine =
  components["schemas"]["ConsignmentReconciliationLine"]
export type ConsignmentReconciliationUpdate =
  components["schemas"]["ConsignmentReconciliationUpdate"]
export type ConsignmentVarianceCause =
  components["schemas"]["ConsignmentVarianceCause"]
export type ConsignmentBorneBy = components["schemas"]["ConsignmentBorneBy"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const DELIVERY_TYPE_LABELS: Record<ConsignmentDeliveryType, string> = {
  consignment: "Konsinyasi",
  vending_buffer: "Buffer Vending",
}

export const DELIVERY_STATUSES: ConsignmentDeliveryStatus[] = [
  "draft",
  "in_progress",
  "in_transit",
  "completed",
  "cancelled",
]

export const RECONCILIATION_STATUSES: ConsignmentReconciliationStatus[] = [
  "draft",
  "in_progress",
  "pending_approval",
  "rejected",
  "posted",
  "cancelled",
]

/** Penyebab selisih (BRD U-02). */
export const VARIANCE_CAUSE_LABELS: Record<ConsignmentVarianceCause, string> = {
  lost: "Hilang",
  partner_damage: "Rusak karena mitra",
  product_defect: "Cacat produk",
  near_expiry: "Mendekati kedaluwarsa",
}

export const BORNE_BY_LABELS: Record<ConsignmentBorneBy, string> = {
  partner: "Ditagih ke mitra",
  balans: "Ditanggung Balans",
}

export type DeliveryListSearch = {
  page: number
  q?: string
  status?: ConsignmentDeliveryStatus
  deliveryType?: ConsignmentDeliveryType
}

export const deliveriesQuery = (s: DeliveryListSearch) =>
  queryOptions({
    queryKey: ["sales", "consignment-deliveries", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/consignment-deliveries", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              deliveryType: s.deliveryType,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const deliveryQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "consignment-delivery", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/consignment-deliveries/{id}", {
          params: { path: { id } },
        })
      ),
  })

/** Mitra konsinyasi untuk form pengiriman dan rekonsiliasi. */
export const consignmentPartnersQuery = (q: string) =>
  queryOptions({
    queryKey: ["sales", "consignment-partners", q],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/partners", {
            params: {
              query: {
                q: q || undefined,
                partnerType: "consignment",
                pageSize: 20,
              },
            },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

export type DeliveryInput = {
  deliveryType: ConsignmentDeliveryType
  partnerId: string
  fromLocationId: string
  businessDate: string
  note?: string
  lines: ConsignmentDeliveryLineInput[]
}

export const createDelivery = (body: DeliveryInput, key: string) =>
  unwrap(
    api.POST("/api/v1/sales/consignment-deliveries", {
      params: idem(key),
      body,
    })
  )

export const updateDelivery = (
  id: string,
  version: number,
  body: Omit<DeliveryInput, "deliveryType" | "partnerId">,
  key: string
) =>
  unwrap(
    api.PUT("/api/v1/sales/consignment-deliveries/{id}", {
      params: { path: { id }, ...idem(key) },
      body: { ...body, version },
    })
  )

/** submit = ajukan ke Logistik (reservasi stok + DO); receive hanya data lama yang dikirim tanpa DO. */
export type DeliveryAction = "submit" | "receive" | "cancel"

export function deliveryAction(
  id: string,
  action: DeliveryAction,
  version: number,
  key: string,
  reason?: string
) {
  const params = { path: { id }, ...idem(key) }
  switch (action) {
    case "submit":
      return unwrap(
        api.POST("/api/v1/sales/consignment-deliveries/{id}/submit", {
          params,
          body: { version },
        })
      )
    case "receive":
      return unwrap(
        api.POST("/api/v1/sales/consignment-deliveries/{id}/receive", {
          params,
          body: { version },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/sales/consignment-deliveries/{id}/cancel", {
          params,
          body: { version, reason },
        })
      )
  }
}

// --- Rekonsiliasi -------------------------------------------------------------------------------

export type ReconciliationListSearch = {
  page: number
  q?: string
  status?: ConsignmentReconciliationStatus
}

export const reconciliationsQuery = (s: ReconciliationListSearch) =>
  queryOptions({
    queryKey: ["sales", "consignment-reconciliations", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/consignment-reconciliations", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const reconciliationQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "consignment-reconciliation", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/consignment-reconciliations/{id}", {
          params: { path: { id } },
        })
      ),
  })

/** Stock count konsinyasi yang sudah Selesai (dasar sisa fisik rekonsiliasi). */
export const completedConsignmentCountsQuery = (q: string) =>
  queryOptions({
    queryKey: ["sales", "consignment-counts", q],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/stock-counts", {
            params: {
              query: {
                q: q || undefined,
                countType: "consignment",
                status: ["completed"],
                pageSize: 20,
              },
            },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

export type ReconciliationInput = {
  partnerId: string
  cutOffDate: string
  stockCountId: string
  note?: string
}

export const createReconciliation = (body: ReconciliationInput, key: string) =>
  unwrap(
    api.POST("/api/v1/sales/consignment-reconciliations", {
      params: idem(key),
      body,
    })
  )

export const updateReconciliationEntries = (
  id: string,
  body: ConsignmentReconciliationUpdate,
  key: string
) =>
  unwrap(
    api.PUT("/api/v1/sales/consignment-reconciliations/{id}/entries", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )

export type ReconciliationAction = "start" | "submit" | "revise" | "cancel"

export function reconciliationAction(
  id: string,
  action: ReconciliationAction,
  version: number,
  key: string,
  reason?: string
) {
  const params = { path: { id }, ...idem(key) }
  switch (action) {
    case "start":
      return unwrap(
        api.POST("/api/v1/sales/consignment-reconciliations/{id}/start", {
          params,
          body: { version },
        })
      )
    case "submit":
      return unwrap(
        api.POST("/api/v1/sales/consignment-reconciliations/{id}/submit", {
          params,
          body: { version },
        })
      )
    case "revise":
      return unwrap(
        api.POST("/api/v1/sales/consignment-reconciliations/{id}/revise", {
          params,
          body: { version },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/sales/consignment-reconciliations/{id}/cancel", {
          params,
          body: { version, reason },
        })
      )
  }
}

/** Meminta URL upload BA bertanda tangan atau laporan penjualan mitra. */
export async function requestConsignmentUpload(
  file: File,
  ownerType: "sales.consignment_ba" | "sales.consignment_partner_report"
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

/** Selisih baris = terjual sistem − laporan mitra (untuk tampilan saat mengetik). */
export function varianceOf(soldQty: string, reportedQty: string): number {
  const sold = Number(soldQty)
  const reported = Number(reportedQty)
  if (!Number.isFinite(sold) || !Number.isFinite(reported)) {
    return 0
  }
  return Math.round((sold - reported) * 1000) / 1000
}
