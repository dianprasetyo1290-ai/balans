import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type ReturnReason = components["schemas"]["ReturnReason"]
export type ReturnReasonInput = components["schemas"]["ReturnReasonInput"]
export type ReturnReasonUpdate = components["schemas"]["ReturnReasonUpdate"]
export type ReturnReasonAppliesTo =
  components["schemas"]["ReturnReasonAppliesTo"]
export type ReturnDisposition = components["schemas"]["ReturnDisposition"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const APPLIES_TO_LABELS: Record<ReturnReasonAppliesTo, string> = {
  sales_return: "Retur penjualan",
  purchase_return: "Retur pembelian",
  both: "Penjualan & pembelian",
}

export const DISPOSITION_LABELS: Record<ReturnDisposition, string> = {
  restock: "Kembali ke stok",
  repack: "Repacking",
  reprocess: "Reprocess maklon",
  marketing_sample: "Sampel/marketing",
  dispose: "Pemusnahan",
}

export const returnReasonsQuery = (includeInactive = false, q = "") =>
  queryOptions({
    queryKey: ["sales", "return-reasons", includeInactive, q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/return-reasons", {
          params: { query: { includeInactive, q: q || undefined } },
        })
      ),
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })

export function createReturnReason(body: ReturnReasonInput, key: string) {
  return unwrap(api.POST("/api/v1/return-reasons", { params: idem(key), body }))
}

export function updateReturnReason(
  id: string,
  body: ReturnReasonUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/return-reasons/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteReturnReason(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/return-reasons/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}
