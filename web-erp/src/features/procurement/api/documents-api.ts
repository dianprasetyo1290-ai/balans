import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

import type { PurchaseOrderStatus } from "./procurement-api"

export type GoodsReceipt = components["schemas"]["GoodsReceipt"]
export type GoodsReceiptSummary = components["schemas"]["GoodsReceiptSummary"]
export type GoodsReceiptStatus = components["schemas"]["GoodsReceiptStatus"]
export type GoodsReceiptFields = components["schemas"]["GoodsReceiptFields"]
export type RejectDisposition = components["schemas"]["RejectDisposition"]
export type SupplierBill = components["schemas"]["SupplierBill"]
export type SupplierBillSummary = components["schemas"]["SupplierBillSummary"]
export type SupplierBillStatus = components["schemas"]["SupplierBillStatus"]
export type SupplierBillFields = components["schemas"]["SupplierBillFields"]
export type BillableReceipts = components["schemas"]["BillableReceipts"]
export type WithholdingType = components["schemas"]["WithholdingType"]
export type PurchaseReturn = components["schemas"]["PurchaseReturn"]
export type PurchaseReturnSummary =
  components["schemas"]["PurchaseReturnSummary"]
export type PurchaseReturnStatus = components["schemas"]["PurchaseReturnStatus"]
export type PurchaseReturnOrigin = components["schemas"]["PurchaseReturnOrigin"]
export type PurchaseReturnFields = components["schemas"]["PurchaseReturnFields"]
export type ReturnableReceipts = components["schemas"]["ReturnableReceipts"]
export type GoodsReceivedNotInvoicedReport =
  components["schemas"]["GoodsReceivedNotInvoicedReport"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const RECEIPT_STATUSES: GoodsReceiptStatus[] = [
  "draft",
  "posted",
  "cancelled",
]

export const BILL_STATUSES: SupplierBillStatus[] = [
  "draft",
  "pending_approval",
  "rejected",
  "awaiting_payment",
  "partially_paid",
  "paid",
  "cancelled",
]

export const RETURN_STATUSES: PurchaseReturnStatus[] = [
  "draft",
  "pending_approval",
  "approved",
  "rejected",
  "posted",
  "cancelled",
]

export const REJECT_DISPOSITION_LABELS: Record<RejectDisposition, string> = {
  absorbed: "Diserap ke HPP (ikut ditagih)",
  return_to_supplier: "Diganti pemasok (tidak diterima)",
}

export const WITHHOLDING_LABELS: Record<WithholdingType, string> = {
  none: "Tidak dipotong",
  pph23: "PPh 23",
  pph21: "PPh 21",
  pph4_2: "PPh 4(2)",
}

export const RETURN_ORIGIN_LABELS: Record<PurchaseReturnOrigin, string> = {
  manual: "Manual",
  qc_inbound: "Reject QC Inbound",
  qc_outbound: "Reject QC Outbound",
}

export const MATCH_STATUS_LABELS: Record<SupplierBill["matchStatus"], string> =
  {
    matched: "Cocok (dalam toleransi)",
    exceeds_tolerance: "Di luar toleransi",
    approved_exception: "Selisih disetujui",
  }

/** Pencarian PO berdasarkan status (form penerimaan, tagihan, retur). */
export const ordersByStatusQuery = (
  q: string,
  statuses: PurchaseOrderStatus[]
) =>
  queryOptions({
    queryKey: ["procurement", "orders-by-status", q, statuses],
    queryFn: async () => {
      const lists = await Promise.all(
        statuses.map((status) =>
          unwrap(
            api.GET("/api/v1/purchase-orders", {
              params: { query: { q: q || undefined, status, pageSize: 20 } },
            })
          )
        )
      )
      return lists.flatMap((l) => l.items)
    },
    placeholderData: keepPreviousData,
  })

export const RECEIVABLE_ORDER_STATUSES: PurchaseOrderStatus[] = [
  "approved",
  "partially_received",
]
export const BILLABLE_ORDER_STATUSES: PurchaseOrderStatus[] = [
  "partially_received",
  "received",
  "closed",
]
export const RETURNABLE_ORDER_STATUSES: PurchaseOrderStatus[] = [
  "partially_received",
  "received",
  "completed",
  "closed",
]

export const inboundLocationsQuery = queryOptions({
  queryKey: ["procurement", "inbound-locations"],
  queryFn: () => unwrap(api.GET("/api/v1/procurement/inbound-locations")),
  staleTime: 60_000,
})

/** Lokasi internal aktif (asal barang retur). */
export const stockLocationsQuery = queryOptions({
  queryKey: ["procurement", "stock-locations"],
  queryFn: async () =>
    (
      await unwrap(
        api.GET("/api/v1/locations", {
          params: { query: { isActive: true, pageSize: 200 } },
        })
      )
    ).items,
  staleTime: 60_000,
})

// --- Penerimaan Barang --------------------------------------------------------------------------

export type ReceiptListSearch = {
  page: number
  q?: string
  status?: GoodsReceiptStatus
  dateFrom?: string
  dateTo?: string
}

export const receiptsQuery = (s: ReceiptListSearch) =>
  queryOptions({
    queryKey: ["procurement", "receipts", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/goods-receipts", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const receiptQuery = (id: string) =>
  queryOptions({
    queryKey: ["procurement", "receipt", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/goods-receipts/{id}", { params: { path: { id } } })
      ),
  })

export function saveReceipt(
  existing: GoodsReceipt | undefined,
  purchaseOrderId: string,
  body: GoodsReceiptFields,
  key: string
) {
  if (existing) {
    return unwrap(
      api.PUT("/api/v1/goods-receipts/{id}", {
        params: { path: { id: existing.id }, ...idem(key) },
        body: { ...body, version: existing.version },
      })
    )
  }
  return unwrap(
    api.POST("/api/v1/goods-receipts", {
      params: idem(key),
      body: { ...body, purchaseOrderId },
    })
  )
}

export function receiptAction(
  action: "post" | "cancel",
  id: string,
  version: number,
  key: string,
  reason = ""
) {
  const params = { path: { id }, ...idem(key) }
  return action === "post"
    ? unwrap(
        api.POST("/api/v1/goods-receipts/{id}/post", {
          params,
          body: { version },
        })
      )
    : unwrap(
        api.POST("/api/v1/goods-receipts/{id}/cancel", {
          params,
          body: { version, reason: reason || undefined },
        })
      )
}

// --- Tagihan Pemasok ----------------------------------------------------------------------------

export type BillListSearch = {
  page: number
  q?: string
  status?: SupplierBillStatus
  dateFrom?: string
  dateTo?: string
}

export const billsQuery = (s: BillListSearch) =>
  queryOptions({
    queryKey: ["procurement", "bills", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/supplier-bills", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const billQuery = (id: string) =>
  queryOptions({
    queryKey: ["procurement", "bill", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/supplier-bills/{id}", { params: { path: { id } } })
      ),
  })

export const billableReceiptsQuery = (purchaseOrderId: string) =>
  queryOptions({
    queryKey: ["procurement", "billable-receipts", purchaseOrderId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/procurement/billable-receipts", {
          params: { query: { purchaseOrderId } },
        })
      ),
  })

export function saveBill(
  existing: SupplierBill | undefined,
  purchaseOrderId: string,
  body: SupplierBillFields,
  key: string
) {
  if (existing) {
    return unwrap(
      api.PUT("/api/v1/supplier-bills/{id}", {
        params: { path: { id: existing.id }, ...idem(key) },
        body: { ...body, version: existing.version },
      })
    )
  }
  return unwrap(
    api.POST("/api/v1/supplier-bills", {
      params: idem(key),
      body: { ...body, purchaseOrderId },
    })
  )
}

export function billAction(
  action: "post" | "revise" | "cancel",
  id: string,
  version: number,
  key: string,
  reason = ""
) {
  const params = { path: { id }, ...idem(key) }
  switch (action) {
    case "post":
      return unwrap(
        api.POST("/api/v1/supplier-bills/{id}/post", {
          params,
          body: { version },
        })
      )
    case "revise":
      return unwrap(
        api.POST("/api/v1/supplier-bills/{id}/revise", {
          params,
          body: { version },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/supplier-bills/{id}/cancel", {
          params,
          body: { version, reason: reason || undefined },
        })
      )
  }
}

// --- Retur Pembelian ----------------------------------------------------------------------------

export type ReturnListSearch = {
  page: number
  q?: string
  status?: PurchaseReturnStatus
  origin?: PurchaseReturnOrigin
  dateFrom?: string
  dateTo?: string
}

export const returnsQuery = (s: ReturnListSearch) =>
  queryOptions({
    queryKey: ["procurement", "returns", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-returns", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              origin: s.origin,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const returnQuery = (id: string) =>
  queryOptions({
    queryKey: ["procurement", "return", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-returns/{id}", { params: { path: { id } } })
      ),
  })

export const returnableReceiptsQuery = (purchaseOrderId: string) =>
  queryOptions({
    queryKey: ["procurement", "returnable-receipts", purchaseOrderId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/procurement/returnable-receipts", {
          params: { query: { purchaseOrderId } },
        })
      ),
  })

export function saveReturn(
  existing: PurchaseReturn | undefined,
  purchaseOrderId: string,
  body: PurchaseReturnFields,
  key: string
) {
  if (existing) {
    return unwrap(
      api.PUT("/api/v1/purchase-returns/{id}", {
        params: { path: { id: existing.id }, ...idem(key) },
        body: { ...body, version: existing.version },
      })
    )
  }
  return unwrap(
    api.POST("/api/v1/purchase-returns", {
      params: idem(key),
      body: { ...body, purchaseOrderId },
    })
  )
}

export function returnAction(
  action: "submit" | "post" | "revise" | "cancel",
  id: string,
  version: number,
  key: string,
  reason = ""
) {
  const params = { path: { id }, ...idem(key) }
  switch (action) {
    case "submit":
      return unwrap(
        api.POST("/api/v1/purchase-returns/{id}/submit", {
          params,
          body: { version },
        })
      )
    case "post":
      return unwrap(
        api.POST("/api/v1/purchase-returns/{id}/post", {
          params,
          body: { version },
        })
      )
    case "revise":
      return unwrap(
        api.POST("/api/v1/purchase-returns/{id}/revise", {
          params,
          body: { version },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/purchase-returns/{id}/cancel", {
          params,
          body: { version, reason: reason || undefined },
        })
      )
  }
}

// --- Laporan ------------------------------------------------------------------------------------

export const grniReportQuery = (q: string) =>
  queryOptions({
    queryKey: ["procurement", "grni-report", q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/procurement/reports/goods-received-not-invoiced", {
          params: { query: { q: q || undefined } },
        })
      ),
    placeholderData: keepPreviousData,
  })
