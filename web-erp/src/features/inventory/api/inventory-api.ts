import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type StockStatus = components["schemas"]["StockStatus"]
export type ExpiryState = components["schemas"]["ExpiryState"]
export type StockBalance = components["schemas"]["StockBalance"]
export type StockCard = components["schemas"]["StockCard"]
export type StockCardLine = components["schemas"]["StockCardLine"]
export type BatchSummary = components["schemas"]["BatchSummary"]
export type BatchSuggestion = components["schemas"]["BatchSuggestion"]
export type SuggestedBatch = components["schemas"]["SuggestedBatch"]
export type CostLayer = components["schemas"]["CostLayer"]
export type CostLayerConsumption = components["schemas"]["CostLayerConsumption"]
export type BundleAvailability = components["schemas"]["BundleAvailability"]

/** Label status stok (Technical Guideline 5.7.2; Desain Status Dokumen). */
export const STOCK_STATUS_LABELS: Record<StockStatus, string> = {
  available: "Tersedia",
  quarantine: "Karantina",
  in_transit: "Dalam Pengiriman",
  reserved: "Dialokasikan",
}

export const STOCK_STATUSES = Object.keys(STOCK_STATUS_LABELS) as StockStatus[]

export const EXPIRY_STATE_LABELS: Record<ExpiryState, string> = {
  expired: "Kedaluwarsa",
  near_expiry: "Segera Kedaluwarsa",
  ok: "Aman",
  no_expiry: "Tanpa Kedaluwarsa",
}

/** Label jenis dokumen sumber mutasi; jenis yang belum terdaftar ditampilkan apa adanya. */
export const SOURCE_TYPE_LABELS: Record<string, string> = {
  opening_balance: "Saldo Awal",
  goods_receipt: "Penerimaan Barang",
  inbound: "Inbound",
  stock_transfer: "Transfer Stok",
  outbound: "Outbound",
  delivery_order: "Delivery Order",
  stock_count: "Stock Count",
  internal_usage: "Pemakaian Internal",
  repacking: "Repacking",
  disposal: "Pemusnahan",
  return_disposition: "Disposisi Retur",
  purchase_return: "Retur Pembelian",
  sales_return: "Retur Penjualan",
  sales_order: "Sales Order",
  dev_sample_issue: "Contoh (demo)",
}

export function sourceTypeLabel(t: string) {
  return SOURCE_TYPE_LABELS[t] ?? t
}

export type StockBalanceSearch = {
  page: number
  productId?: string
  locationId?: string
  batchId?: string
  status?: StockStatus
}

export const stockBalancesQuery = (s: StockBalanceSearch) =>
  queryOptions({
    queryKey: ["inventory", "balances", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-balances", {
          params: {
            query: {
              page: s.page,
              productId: s.productId,
              locationId: s.locationId ? [s.locationId] : undefined,
              batchId: s.batchId,
              status: s.status ? [s.status] : undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export type StockCardSearch = {
  page: number
  productId?: string
  locationId?: string
  batchId?: string
  status?: StockStatus
  dateFrom: string
  dateTo: string
}

export const stockCardQuery = (s: StockCardSearch & { productId: string }) =>
  queryOptions({
    queryKey: ["inventory", "stock-card", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/stock-card", {
          params: {
            query: {
              page: s.page,
              productId: s.productId,
              locationId: s.locationId,
              batchId: s.batchId,
              status: s.status,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export type BatchSearch = {
  page: number
  productId?: string
  q?: string
  expiryState?: ExpiryState
  withStock?: boolean
}

export const batchesQuery = (s: BatchSearch) =>
  queryOptions({
    queryKey: ["inventory", "batches", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/batches", {
          params: {
            query: {
              page: s.page,
              productId: s.productId,
              q: s.q || undefined,
              expiryState: s.expiryState,
              withStock: s.withStock,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

/** Batch satu produk untuk dropdown filter (maks. 200, termasuk yang stoknya habis). */
export const productBatchesQuery = (productId: string) =>
  queryOptions({
    queryKey: ["inventory", "product-batches", productId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/batches", {
          params: { query: { productId, withStock: false, pageSize: 200 } },
        })
      ),
  })

export const batchSuggestionQuery = (
  productId: string,
  locationId: string,
  qty: string
) =>
  queryOptions({
    queryKey: ["inventory", "batch-suggestion", productId, locationId, qty],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/batch-suggestions", {
          params: { query: { productId, locationId, qty } },
        })
      ),
  })

export type CostLayerSearch = {
  page: number
  productId?: string
  batchId?: string
  openOnly?: boolean
}

export const costLayersQuery = (s: CostLayerSearch) =>
  queryOptions({
    queryKey: ["inventory", "cost-layers", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/cost-layers", {
          params: {
            query: {
              page: s.page,
              productId: s.productId,
              batchId: s.batchId,
              openOnly: s.openOnly,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const costLayerConsumptionsQuery = (id: string) =>
  queryOptions({
    queryKey: ["inventory", "cost-layer-consumptions", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/cost-layers/{id}/consumptions", {
          params: { path: { id } },
        })
      ),
  })

export const bundleAvailabilityQuery = (bundleIds: string[]) =>
  queryOptions({
    queryKey: ["inventory", "bundle-availability", bundleIds],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/bundle-availability", {
          params: { query: { bundleId: bundleIds } },
        })
      ),
  })
