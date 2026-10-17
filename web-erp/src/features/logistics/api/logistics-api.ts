import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type DeliveryOrder = components["schemas"]["DeliveryOrder"]
export type DeliveryOrderSummary = components["schemas"]["DeliveryOrderSummary"]
export type DeliveryOrderStatus = components["schemas"]["DeliveryOrderStatus"]
export type DeliveryOrderType = components["schemas"]["DeliveryOrderType"]
export type DeliveryOrderShipInput =
  components["schemas"]["DeliveryOrderShipInput"]
export type DeliveryOrderDeliverInput =
  components["schemas"]["DeliveryOrderDeliverInput"]
export type DeliveryPackage = components["schemas"]["DeliveryPackage"]
export type DeliveryProductLabel = components["schemas"]["DeliveryProductLabel"]
export type DeliveryBatchRef = components["schemas"]["DeliveryBatchRef"]
export type Courier = components["schemas"]["Courier"]
export type CourierInput = components["schemas"]["CourierInput"]
export type Shipment = components["schemas"]["Shipment"]
export type ShipmentSummary = components["schemas"]["ShipmentSummary"]
export type ShipmentStatus = components["schemas"]["ShipmentStatus"]
export type ShipmentEvent = components["schemas"]["ShipmentEvent"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

/** Label jenis DO. */
export const DELIVERY_TYPE_LABELS: Record<DeliveryOrderType, string> = {
  sales: "Penjualan",
  marketplace: "Marketplace",
  consignment: "Konsinyasi",
  vending_buffer: "Buffer Vending",
  transfer: "Lainnya",
}

/** Label status pengiriman (pelacakan). */
export const SHIPMENT_STATUS_LABELS: Record<ShipmentStatus, string> = {
  picked_up: "Diserahkan ke Kurir",
  in_transit: "Dalam Perjalanan",
  delivered: "Diterima",
  returned: "Kembali ke Gudang",
}

export function batchText(b?: DeliveryBatchRef | null) {
  return b ? b.number : "—"
}

// --- Delivery Order --------------------------------------------------------

export type DeliveryOrderSearch = {
  page: number
  q?: string
  status?: DeliveryOrderStatus
}

export const deliveryOrdersQuery = (s: DeliveryOrderSearch) =>
  queryOptions({
    queryKey: ["logistics", "delivery-orders", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/delivery-orders", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const deliveryOrderQuery = (id: string) =>
  queryOptions({
    queryKey: ["logistics", "delivery-order", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/delivery-orders/{id}", { params: { path: { id } } })
      ),
  })

export function createDeliveryOrder(
  body: { salesOrderId: string; note?: string },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/delivery-orders", { params: idem(key), body })
  )
}

export function releaseDeliveryOrder(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/delivery-orders/{id}/release", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function shipDeliveryOrder(
  id: string,
  body: DeliveryOrderShipInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/delivery-orders/{id}/ship", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deliverDeliveryOrder(
  id: string,
  body: DeliveryOrderDeliverInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/delivery-orders/{id}/deliver", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function cancelDeliveryOrder(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/delivery-orders/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}

/** Sales Order Diproses yang bisa dibuatkan DO. */
export const shippableOrdersQuery = (q: string) =>
  queryOptions({
    queryKey: ["logistics", "shippable-orders", q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/orders", {
          params: { query: { status: "in_progress", q: q || undefined } },
        })
      ),
  })

// --- Kurir & pengiriman ------------------------------------------------------

export const couriersQuery = (activeOnly: boolean) =>
  queryOptions({
    queryKey: ["logistics", "couriers", activeOnly],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/couriers", {
          params: { query: { active: activeOnly || undefined } },
        })
      ),
  })

export function createCourier(body: CourierInput, key: string) {
  return unwrap(api.POST("/api/v1/couriers", { params: idem(key), body }))
}

export function updateCourier(
  id: string,
  body: Omit<CourierInput, "code"> & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/couriers/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export type ShipmentSearch = {
  page: number
  q?: string
  status?: ShipmentStatus
  mine?: boolean
}

export const shipmentsQuery = (s: ShipmentSearch) =>
  queryOptions({
    queryKey: ["logistics", "shipments", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/shipments", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              mine: s.mine || undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function recordShipmentEvent(
  id: string,
  body: { status: ShipmentStatus; note?: string },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/shipments/{id}/events", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

/** Meminta URL upload foto/tanda terima barang diterima (kurir internal dari HP). */
export async function requestProofUpload(file: File) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType: "delivery_proof",
        fileName: file.name,
        contentType: file.type || "image/jpeg",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

/** Satu label koli 100 × 150 mm per koli (S-316). */
export type ColliLabel = {
  key: string
  doNumber: string
  n: number
  total: number
  recipient: string
  phone: string
  address: string
  weightGrams?: number
  contents: string[]
}

/** Menyusun label koli dari koli Outbound DO. */
export function colliLabels(d: DeliveryOrder): ColliLabel[] {
  return d.packages.map((p) => ({
    key: `${d.id}-${p.packageNumber}`,
    doNumber: d.number ?? "",
    n: p.packageNumber,
    total: p.packageCount,
    recipient: d.recipientName,
    phone: d.recipientPhone,
    address: d.shipToAddress,
    weightGrams: p.weightGrams ?? undefined,
    contents: p.items.map(
      (i) =>
        `${i.product.sku} ${i.product.name} × ${Number(i.qty)}${i.batch ? ` (Batch ${i.batch.number})` : ""}`
    ),
  }))
}
