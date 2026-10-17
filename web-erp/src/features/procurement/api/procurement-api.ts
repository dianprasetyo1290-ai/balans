import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type PurchaseRequest = components["schemas"]["PurchaseRequest"]
export type PurchaseRequestSummary =
  components["schemas"]["PurchaseRequestSummary"]
export type PurchaseRequestStatus =
  components["schemas"]["PurchaseRequestStatus"]
export type PurchaseRequestFields =
  components["schemas"]["PurchaseRequestFields"]
export type PurchaseRequestLineInput =
  components["schemas"]["PurchaseRequestLineInput"]
export type PurchaseOrder = components["schemas"]["PurchaseOrder"]
export type PurchaseOrderSummary = components["schemas"]["PurchaseOrderSummary"]
export type PurchaseOrderStatus = components["schemas"]["PurchaseOrderStatus"]
export type PurchaseOrderType = components["schemas"]["PurchaseOrderType"]
export type PurchaseOrderInput = components["schemas"]["PurchaseOrderInput"]
export type PurchaseOrderFields = components["schemas"]["PurchaseOrderFields"]
export type PurchaseOrderLineInput =
  components["schemas"]["PurchaseOrderLineInput"]
export type PurchaseOrderMaterialInput =
  components["schemas"]["PurchaseOrderMaterialInput"]
export type PurchaseAdvance = components["schemas"]["PurchaseAdvance"]
export type PurchaseAdvanceStatus =
  components["schemas"]["PurchaseAdvanceStatus"]
export type PurchaseAdvanceInput = components["schemas"]["PurchaseAdvanceInput"]
export type PurchaseAdvanceFields =
  components["schemas"]["PurchaseAdvanceFields"]
export type PurchaseOption = components["schemas"]["PurchaseOption"]
export type PurchaseProductRef = components["schemas"]["PurchaseProductRef"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const ORDER_TYPE_LABELS: Record<PurchaseOrderType, string> = {
  goods: "Barang",
  packaging: "Packaging",
  toll_manufacturing: "Jasa Maklon",
  reprocess: "Reprocess",
  other: "Lainnya",
}

/** Kelompok halaman PO: menu Purchase Order, Purchase Order Maklon, Purchase Order Reprocess. */
export type OrderKind = "regular" | "toll" | "reprocess"

export const ORDER_KINDS: Record<
  OrderKind,
  {
    title: string
    description: string
    types: PurchaseOrderType[]
    path:
      | "/procurement/purchase-orders"
      | "/procurement/toll-orders"
      | "/procurement/reprocess-orders"
  }
> = {
  regular: {
    title: "Purchase Order",
    description:
      "Pembelian packaging, barang, dan kebutuhan lain. Approval berjenjang sesuai total PO; tanpa PPN (NON-PKP).",
    types: ["packaging", "goods", "other"],
    path: "/procurement/purchase-orders",
  },
  toll: {
    title: "Purchase Order Maklon",
    description:
      "Jasa maklon per batch: FG hasil dengan harga jasa per unit dan packaging yang dikirim ke lokasi maklon.",
    types: ["toll_manufacturing"],
    path: "/procurement/toll-orders",
  },
  reprocess: {
    title: "Purchase Order Reprocess",
    description:
      "Reprocess barang retur di maklon: bahan GM dikirim dari Gudang GM ke lokasi maklon menjadi FG.",
    types: ["reprocess"],
    path: "/procurement/reprocess-orders",
  },
}

export function orderKindOf(t: PurchaseOrderType): OrderKind {
  if (t === "toll_manufacturing") return "toll"
  if (t === "reprocess") return "reprocess"
  return "regular"
}

export const REQUEST_STATUSES: PurchaseRequestStatus[] = [
  "draft",
  "pending_approval",
  "approved",
  "rejected",
  "completed",
  "closed",
  "cancelled",
]

export const ORDER_STATUSES: PurchaseOrderStatus[] = [
  "draft",
  "pending_approval",
  "approved",
  "rejected",
  "partially_received",
  "received",
  "completed",
  "closed",
  "cancelled",
]

export const ADVANCE_STATUSES: PurchaseAdvanceStatus[] = [
  "draft",
  "posted",
  "cancelled",
]

// --- Pilihan ------------------------------------------------------------------------------------

export const divisionOptionsQuery = queryOptions({
  queryKey: ["procurement", "division-options"],
  queryFn: () => unwrap(api.GET("/api/v1/procurement/division-options")),
  staleTime: 5 * 60_000,
})

export const cashBankAccountsQuery = queryOptions({
  queryKey: ["procurement", "cash-bank-accounts"],
  queryFn: () => unwrap(api.GET("/api/v1/procurement/cash-bank-accounts")),
  staleTime: 5 * 60_000,
})

/** Pencarian pemasok untuk pemilih (maklon saja untuk PO maklon/reprocess). */
export const supplierOptionsQuery = (q: string, tollOnly: boolean) =>
  queryOptions({
    queryKey: ["procurement", "supplier-options", q, tollOnly],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/suppliers", {
            params: {
              query: {
                q: q || undefined,
                supplierType: tollOnly ? "toll_manufacturer" : undefined,
                pageSize: 20,
              },
            },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

/** Lokasi asal bahan: Gudang GM – RM (maklon) atau Gudang GM – GM (reprocess). */
export const materialLocationsQuery = queryOptions({
  queryKey: ["procurement", "material-locations"],
  queryFn: async () =>
    (
      await unwrap(
        api.GET("/api/v1/locations", {
          params: {
            query: {
              locationType: ["raw_material", "goods_material"],
              isActive: true,
              pageSize: 200,
            },
          },
        })
      )
    ).items,
  staleTime: 60_000,
})

// --- Purchase Request ---------------------------------------------------------------------------

export type RequestListSearch = {
  page: number
  sort?: string
  q?: string
  status?: PurchaseRequestStatus
  divisionId?: string
  mine?: boolean
  dateFrom?: string
  dateTo?: string
}

export const requestsQuery = (s: RequestListSearch) =>
  queryOptions({
    queryKey: ["procurement", "requests", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-requests", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              status: s.status,
              divisionId: s.divisionId,
              mine: s.mine || undefined,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const requestQuery = (id: string) =>
  queryOptions({
    queryKey: ["procurement", "request", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-requests/{id}", { params: { path: { id } } })
      ),
  })

export function createRequest(body: PurchaseRequestFields, key: string) {
  return unwrap(
    api.POST("/api/v1/purchase-requests", { params: idem(key), body })
  )
}

export function updateRequest(
  id: string,
  body: PurchaseRequestFields & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/purchase-requests/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export type RequestAction = "submit" | "revise" | "cancel" | "close"

export function requestAction(
  action: RequestAction,
  id: string,
  version: number,
  key: string,
  reason = ""
) {
  const params = { path: { id }, ...idem(key) }
  switch (action) {
    case "submit":
      return unwrap(
        api.POST("/api/v1/purchase-requests/{id}/submit", {
          params,
          body: { version },
        })
      )
    case "revise":
      return unwrap(
        api.POST("/api/v1/purchase-requests/{id}/revise", {
          params,
          body: { version },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/purchase-requests/{id}/cancel", {
          params,
          body: { version },
        })
      )
    case "close":
      return unwrap(
        api.POST("/api/v1/purchase-requests/{id}/close", {
          params,
          body: { version, reason },
        })
      )
  }
}

// --- Purchase Order -----------------------------------------------------------------------------

export type OrderListSearch = {
  page: number
  sort?: string
  q?: string
  status?: PurchaseOrderStatus
  orderType?: PurchaseOrderType
  supplierId?: string
  dateFrom?: string
  dateTo?: string
}

export const ordersQuery = (kind: OrderKind, s: OrderListSearch) =>
  queryOptions({
    queryKey: ["procurement", "orders", kind, s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-orders", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              status: s.status,
              orderType: s.orderType ? [s.orderType] : ORDER_KINDS[kind].types,
              supplierId: s.supplierId,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const orderQuery = (id: string) =>
  queryOptions({
    queryKey: ["procurement", "order", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-orders/{id}", { params: { path: { id } } })
      ),
  })

/** PO yang bisa diberi uang muka (Disetujui, Diterima Sebagian, Diterima). */
export const advanceableOrdersQuery = (q: string) =>
  queryOptions({
    queryKey: ["procurement", "advanceable-orders", q],
    queryFn: async () => {
      const lists = await Promise.all(
        (["approved", "partially_received", "received"] as const).map(
          (status) =>
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

export function createOrder(body: PurchaseOrderInput, key: string) {
  return unwrap(
    api.POST("/api/v1/purchase-orders", { params: idem(key), body })
  )
}

export function updateOrder(
  id: string,
  body: PurchaseOrderFields & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/purchase-orders/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export type OrderAction = "submit" | "revise" | "cancel" | "close"

export function orderAction(
  action: OrderAction,
  id: string,
  version: number,
  key: string,
  reason = ""
) {
  const params = { path: { id }, ...idem(key) }
  switch (action) {
    case "submit":
      return unwrap(
        api.POST("/api/v1/purchase-orders/{id}/submit", {
          params,
          body: { version },
        })
      )
    case "revise":
      return unwrap(
        api.POST("/api/v1/purchase-orders/{id}/revise", {
          params,
          body: { version },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/purchase-orders/{id}/cancel", {
          params,
          body: { version, reason: reason || undefined },
        })
      )
    case "close":
      return unwrap(
        api.POST("/api/v1/purchase-orders/{id}/close", {
          params,
          body: { version, reason },
        })
      )
  }
}

// --- Uang muka ----------------------------------------------------------------------------------

export type AdvanceListSearch = {
  page: number
  q?: string
  status?: PurchaseAdvanceStatus
  purchaseOrderId?: string
  dateFrom?: string
  dateTo?: string
}

export const advancesQuery = (s: AdvanceListSearch) =>
  queryOptions({
    queryKey: ["procurement", "advances", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-advances", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              purchaseOrderId: s.purchaseOrderId,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const advanceQuery = (id: string) =>
  queryOptions({
    queryKey: ["procurement", "advance", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/purchase-advances/{id}", { params: { path: { id } } })
      ),
  })

export function createAdvance(body: PurchaseAdvanceInput, key: string) {
  return unwrap(
    api.POST("/api/v1/purchase-advances", { params: idem(key), body })
  )
}

export function updateAdvance(
  id: string,
  body: PurchaseAdvanceFields & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/purchase-advances/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function postAdvance(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/purchase-advances/{id}/post", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function cancelAdvance(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/purchase-advances/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason: reason || undefined },
    })
  )
}

/** Riwayat persetujuan satu dokumen untuk panel Persetujuan di detail. */
export const subjectApprovalsQuery = (subjectType: string, subjectId: string) =>
  queryOptions({
    queryKey: ["approval", "subject", subjectType, subjectId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-requests", {
          params: { query: { subjectType, subjectId, pageSize: 20 } },
        })
      ),
  })
