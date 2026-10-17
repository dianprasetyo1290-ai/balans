import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Outbound = components["schemas"]["Outbound"]
export type OutboundSummary = components["schemas"]["OutboundSummary"]
export type OutboundLine = components["schemas"]["OutboundLine"]
export type OutboundStatus = components["schemas"]["OutboundStatus"]
export type OutboundLineStatus = components["schemas"]["OutboundLineStatus"]
export type OutboundQcInput = components["schemas"]["OutboundQcInput"]
export type OutboundPackageInput = components["schemas"]["OutboundPackageInput"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

/** Label & warna status baris Outbound. */
export const OUTBOUND_LINE_STATUS: Record<
  OutboundLineStatus,
  { label: string; tone: "neutral" | "info" | "warning" | "success" | "danger" }
> = {
  pending: { label: "Menunggu Picking", tone: "neutral" },
  picked: { label: "Menunggu QC", tone: "info" },
  passed: { label: "Lulus QC", tone: "success" },
  qc_rejected: { label: "Ditolak QC", tone: "danger" },
  held: { label: "Ditahan (stok kurang)", tone: "warning" },
  dropped: { label: "Tidak Dikirim", tone: "neutral" },
}

export type OutboundSearch = {
  page: number
  q?: string
  status?: OutboundStatus
}

export const outboundsQuery = (s: OutboundSearch) =>
  queryOptions({
    queryKey: ["warehouse", "outbounds", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/outbounds", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const outboundQuery = (id: string) =>
  queryOptions({
    queryKey: ["warehouse", "outbound", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/outbounds/{id}", { params: { path: { id } } })),
  })

export function pickOutbound(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/outbounds/{id}/pick", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function packOutbound(
  id: string,
  version: number,
  packages: OutboundPackageInput[],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/outbounds/{id}/pack", {
      params: { path: { id }, ...idem(key) },
      body: { version, packages },
    })
  )
}

export function recordOutboundQc(
  id: string,
  lineId: string,
  body: OutboundQcInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/outbounds/{id}/lines/{lineId}/qc", {
      params: { path: { id, lineId }, ...idem(key) },
      body,
    })
  )
}

export function reallocateOutboundLine(
  id: string,
  lineId: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/outbounds/{id}/lines/{lineId}/reallocate", {
      params: { path: { id, lineId }, ...idem(key) },
    })
  )
}

export function dropOutboundLine(
  id: string,
  lineId: string,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/outbounds/{id}/lines/{lineId}/drop", {
      params: { path: { id, lineId }, ...idem(key) },
      body: { reason },
    })
  )
}

/** Baris yang masuk koli saat packing: qty diambil (belum QC) atau qty lulus QC. */
export function packableLines(o: Outbound) {
  return o.lines
    .filter((l) => l.status === "picked" || l.status === "passed")
    .map((l) => ({
      line: l,
      qty: l.status === "picked" ? l.pickedQty : l.passedQty,
    }))
}

/**
 * Membagi barang ke n koli: setiap baris masuk utuh ke koli secara bergiliran (round-robin) agar koli tidak
 * kosong; user bisa mengubah isi sebelum disimpan.
 */
export function distributePackages(
  lines: { lineId: string; qty: string }[],
  count: number
): OutboundPackageInput[] {
  const n = Math.max(1, Math.min(count, Math.max(lines.length, 1)))
  const pkgs: OutboundPackageInput[] = Array.from({ length: n }, () => ({
    items: [],
  }))
  lines.forEach((l, i) => {
    pkgs[i % n].items.push({ lineId: l.lineId, qty: l.qty })
  })
  return pkgs
}
