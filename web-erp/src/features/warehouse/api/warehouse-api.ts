import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Warehouse = components["schemas"]["Warehouse"]
export type WarehouseInput = components["schemas"]["WarehouseInput"]
export type WarehouseUpdate = components["schemas"]["WarehouseUpdate"]
export type Location = components["schemas"]["Location"]
export type LocationInput = components["schemas"]["LocationInput"]
export type LocationType = components["schemas"]["LocationType"]
export type ExternalPartyType = components["schemas"]["ExternalPartyType"]

/** Jenis lokasi eksternal beserta pihak yang boleh memilikinya (MD-09). */
export const EXTERNAL_LOCATION_TYPES: {
  type: LocationType
  label: string
  partyTypes: ExternalPartyType[]
}[] = [
  { type: "toll_manufacturer", label: "Maklon", partyTypes: ["supplier"] },
  { type: "consignment", label: "Konsinyasi", partyTypes: ["partner"] },
  {
    type: "vending_buffer",
    label: "Buffer Vending",
    partyTypes: ["partner", "vending_machine"],
  },
  {
    type: "vending_machine",
    label: "Mesin Vending",
    partyTypes: ["vending_machine"],
  },
]

/** Jenis lokasi tambahan di dalam gudang (lokasi standar dibuat otomatis). */
export const INTERNAL_LOCATION_TYPES: { type: LocationType; label: string }[] =
  [
    { type: "inbound", label: "Inbound" },
    { type: "finished_goods", label: "FG Non-Ecommerce" },
    { type: "ecommerce", label: "E-commerce Stock" },
    { type: "raw_material", label: "RM" },
    { type: "goods_material", label: "GM" },
    { type: "returns", label: "Retur" },
    { type: "event", label: "Event" },
  ]

export const PARTY_TYPE_LABELS: Record<ExternalPartyType, string> = {
  supplier: "Pemasok/maklon",
  partner: "Mitra",
  vending_machine: "Mesin vending",
}

export type WarehouseListSearch = {
  page: number
  sort?: string
  q?: string
  deleted?: boolean
}

export const warehousesQuery = (s: WarehouseListSearch) =>
  queryOptions({
    queryKey: ["warehouse", "warehouses", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/warehouses", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              deleted: s.deleted || undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const warehouseQuery = (id: string) =>
  queryOptions({
    queryKey: ["warehouse", "warehouse", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/warehouses/{id}", { params: { path: { id } } })),
  })

export type LocationListSearch = {
  page: number
  sort?: string
  q?: string
  warehouseId?: string
  external?: boolean
  deleted?: boolean
}

export const locationsQuery = (s: LocationListSearch) =>
  queryOptions({
    queryKey: ["warehouse", "locations", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/locations", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              warehouseId: s.warehouseId,
              external: s.external,
              deleted: s.deleted || undefined,
              pageSize: 100,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export function createWarehouse(body: WarehouseInput, key: string) {
  return unwrap(api.POST("/api/v1/warehouses", { params: idem(key), body }))
}

export function updateWarehouse(
  id: string,
  body: WarehouseUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/warehouses/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteWarehouse(id: string, key: string) {
  return unwrap(
    api.DELETE("/api/v1/warehouses/{id}", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function restoreWarehouse(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/warehouses/{id}/restore", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function createLocation(body: LocationInput, key: string) {
  return unwrap(api.POST("/api/v1/locations", { params: idem(key), body }))
}

export function updateLocation(
  id: string,
  body: { name: string; isActive: boolean; version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/locations/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteLocation(id: string, key: string) {
  return unwrap(
    api.DELETE("/api/v1/locations/{id}", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function restoreLocation(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/locations/{id}/restore", {
      params: { path: { id }, ...idem(key) },
    })
  )
}
