import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

type S = components["schemas"]

export type FinancialStatement = S["FinancialStatement"]
export type StatementKind = S["StatementKind"]
export type StatementRow = S["StatementRow"]
export type ProfitLossSummary = S["ProfitLossSummary"]
export type DimensionProfitLoss = S["DimensionProfitLoss"]
export type DimensionKind = S["DimensionKind"]
export type TaxSummary = S["TaxSummary"]
export type ExecutiveDashboard = S["ExecutiveDashboard"]
export type ReadModelStatus = S["ReadModelStatus"]
export type SalesReport = S["SalesReport"]
export type ReportGroupBy = S["ReportGroupBy"]
export type StockPositionReport = S["StockPositionReport"]
export type StockMovementReport = S["StockMovementReport"]
export type StockExpiryReport = S["StockExpiryReport"]
export type InternalUsageReport = S["InternalUsageReport"]
export type ReturnReport = S["ReturnReport"]
export type StockCountVarianceReport = S["StockCountVarianceReport"]
export type OutstandingPurchaseOrderReport = S["OutstandingPurchaseOrderReport"]
export type PayableAgingReport = S["PayableAgingReport"]
export type BatchCostReport = S["BatchCostReport"]
export type ConsignmentReport = S["ConsignmentReport"]
export type MarketplaceSettlement = S["MarketplaceSettlement"]
export type MarketplaceSettlementSummary = S["MarketplaceSettlementSummary"]
export type SettlementStatus = S["SettlementStatus"]
export type SettlementFormat = S["SettlementFormat"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export type Period = { dateFrom: string; dateTo: string }

const base = (...key: unknown[]) => ["reports", ...key]

export const readModelsQuery = queryOptions({
  queryKey: base("read-models"),
  queryFn: () => unwrap(api.GET("/api/v1/reports/read-models")),
})

export function refreshReadModels(key: string) {
  return unwrap(
    api.POST("/api/v1/reports/read-models/refresh", { params: idem(key) })
  )
}

/** Membuat job ekspor laporan ke Excel (dataexchange.export) dengan parameter filter halaman. */
export function exportReport(
  kind: string,
  params: Record<string, string | undefined>,
  key: string
) {
  const clean: Record<string, string> = {}
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") {
      clean[k] = v
    }
  }
  return unwrap(
    api.POST("/api/v1/export-jobs", {
      params: idem(key),
      body: { kind: `report.${kind}`, params: clean },
    })
  )
}

export const statementQuery = (
  kind: StatementKind,
  p: Period,
  dims: { channelId?: string; divisionId?: string } = {}
) =>
  queryOptions({
    queryKey: base("statement", kind, p, dims),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/reports/financial-statements", {
          params: { query: { kind, ...p, ...dims } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const dimensionQuery = (dimension: DimensionKind, p: Period) =>
  queryOptions({
    queryKey: base("dimension", dimension, p),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/reports/profit-loss-by-dimension", {
          params: { query: { dimension, ...p } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const taxQuery = (year: number, month?: number) =>
  queryOptions({
    queryKey: base("tax", year, month),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/reports/tax-summary", {
          params: { query: { year, month } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const dashboardQuery = (p: Period) =>
  queryOptions({
    queryKey: base("dashboard", p),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/reports/executive-dashboard", {
          params: { query: p },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const salesReportQuery = (
  groupBy: ReportGroupBy,
  p: Period,
  filter: { channelId?: string; orderType?: string } = {}
) =>
  queryOptions({
    queryKey: base("sales", groupBy, p, filter),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/reports/sales", {
          params: { query: { groupBy, ...p, ...filter } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const consignmentReportQuery = (p: Period) =>
  queryOptions({
    queryKey: base("consignment", p),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/reports/consignment", { params: { query: p } })
      ),
    placeholderData: keepPreviousData,
  })

export type StockFilter = {
  locationType?: string
  productId?: string
  locationId?: string
}

export const stockPositionQuery = (f: StockFilter) =>
  queryOptions({
    queryKey: base("stock-position", f),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/inventory/reports/stock-position", {
          params: { query: f },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const stockMovementQuery = (p: Period, f: StockFilter) =>
  queryOptions({
    queryKey: base("stock-movements", p, f),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/inventory/reports/stock-movements", {
          params: { query: { ...p, ...f } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const stockExpiryQuery = (f: StockFilter, withinDays?: number) =>
  queryOptions({
    queryKey: base("stock-expiry", f, withinDays),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/inventory/reports/stock-expiry", {
          params: { query: { ...f, withinDays } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const internalUsageReportQuery = (p: Period) =>
  queryOptions({
    queryKey: base("internal-usage", p),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/inventory/reports/internal-usage", {
          params: { query: p },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const returnReportQuery = (p: Period) =>
  queryOptions({
    queryKey: base("returns", p),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/inventory/reports/returns", { params: { query: p } })
      ),
    placeholderData: keepPreviousData,
  })

export const stockCountReportQuery = (p: Period, varianceOnly: boolean) =>
  queryOptions({
    queryKey: base("stock-counts", p, varianceOnly),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/warehouse/reports/stock-count-variances", {
          params: { query: { ...p, varianceOnly } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const outstandingPOQuery = queryOptions({
  queryKey: base("outstanding-po"),
  queryFn: () =>
    unwrap(api.GET("/api/v1/procurement/reports/outstanding-purchase-orders")),
})

export const payableAgingQuery = (asOf: string) =>
  queryOptions({
    queryKey: base("payable-aging", asOf),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/procurement/reports/payable-aging", {
          params: { query: { asOf } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const batchCostQuery = (p: Period) =>
  queryOptions({
    queryKey: base("batch-costs", p),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/procurement/reports/batch-costs", {
          params: { query: p },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const settlementFormatsQuery = queryOptions({
  queryKey: base("settlement-formats"),
  queryFn: () => unwrap(api.GET("/api/v1/sales/settlement-formats")),
  staleTime: Infinity,
})

export const settlementsQuery = (page: number, status?: SettlementStatus) =>
  queryOptions({
    queryKey: base("settlements", page, status),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/marketplace-settlements", {
          params: { query: { page, status } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const settlementQuery = (id: string) =>
  queryOptions({
    queryKey: base("settlement", id),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/marketplace-settlements/{id}", {
          params: { path: { id } },
        })
      ),
  })

export const marketplaceReceivablesQuery = (
  page: number,
  openOnly: boolean,
  q?: string
) =>
  queryOptions({
    queryKey: base("marketplace-receivables", page, openOnly, q),
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/marketplace-receivables", {
          params: { query: { page, openOnly, q: q || undefined } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function postSettlement(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/sales/marketplace-settlements/{id}/post", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function cancelSettlement(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/marketplace-settlements/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}
