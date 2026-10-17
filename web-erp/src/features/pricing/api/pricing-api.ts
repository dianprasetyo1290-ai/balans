import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

type Schemas = components["schemas"]

export type PriceList = Schemas["PriceList"]
export type PriceListDetail = Schemas["PriceListDetail"]
export type PriceListInput = Schemas["PriceListInput"]
export type PriceListUpdate = Schemas["PriceListUpdate"]
export type PriceListType = Schemas["PriceListType"]
export type PriceListBasis = Schemas["PriceListBasis"]
export type PriceListOption = Schemas["PriceListOption"]
export type PriceListVersion = Schemas["PriceListVersion"]
export type PriceListVersionSummary = Schemas["PriceListVersionSummary"]
export type PriceListVersionInput = Schemas["PriceListVersionInput"]
export type PriceListVersionUpdate = Schemas["PriceListVersionUpdate"]
export type PriceListItemInput = Schemas["PriceListItemInput"]
export type PricingPeriodStatus = Schemas["PricingPeriodStatus"]
export type SpecialPrice = Schemas["SpecialPrice"]
export type SpecialPriceInput = Schemas["SpecialPriceInput"]
export type SpecialPricePartyType = Schemas["SpecialPricePartyType"]
export type Promotion = Schemas["Promotion"]
export type PromotionInput = Schemas["PromotionInput"]
export type PromotionUpdate = Schemas["PromotionUpdate"]
export type TieredPrice = Schemas["TieredPrice"]
export type TieredPriceInput = Schemas["TieredPriceInput"]
export type DiscountLimit = Schemas["DiscountLimit"]
export type DiscountLimitInput = Schemas["DiscountLimitInput"]
export type DiscountLimitRole = Schemas["DiscountLimitRole"]
export type PriceQuote = Schemas["PriceQuote"]
export type PriceQuoteRequest = Schemas["PriceQuoteRequest"]
export type PriceRule = Schemas["PriceRule"]
export type DiscountKind = Schemas["DiscountKind"]
export type DiscountCheckRequest = Schemas["DiscountCheckRequest"]
export type DiscountCheckResult = Schemas["DiscountCheckResult"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

/** Jenis daftar harga yang bisa dibuat user. Reseller tersedia di Fase 2; Harga Dasar & HET sudah tersedia. */
export const PRICE_LIST_TYPE_LABELS: Record<PriceListType, string> = {
  base: "Harga Dasar & HET",
  channel: "Channel",
  partner: "Mitra",
  employee: "Karyawan",
  vending: "Vending",
  reseller: "Level Reseller (Fase 2)",
}

export const CREATABLE_PRICE_LIST_TYPES: Partial<
  Record<PriceListType, string>
> = {
  channel: "Channel / sub-channel",
  partner: "Mitra (MT / konsinyasi)",
  employee: "Karyawan",
  vending: "Vending",
}

export const BASIS_LABELS: Record<PriceListBasis, string> = {
  fixed: "Harga per produk",
  percent: "Persentase dari daftar harga acuan",
}

export const PERIOD_STATUS_LABELS: Record<PricingPeriodStatus, string> = {
  scheduled: "Terjadwal",
  active: "Berlaku",
  expired: "Berakhir",
}

export const PRICE_RULE_LABELS: Record<PriceRule, string> = {
  special_price: "Harga khusus",
  promotion: "Promo",
  tiered_price: "Harga bertingkat",
  price_list: "Daftar harga",
  base_price: "Harga dasar",
}

export const PARTY_TYPE_LABELS: Record<SpecialPricePartyType, string> = {
  customer: "Pelanggan",
  partner: "Mitra",
}

export const DISCOUNT_KIND_LABELS: Record<DiscountKind, string> = {
  percent: "Persen (%)",
  amount: "Nominal (Rp)",
}

/** Persen desimal API ("12.5000") → tampilan "12,5%". */
export function formatPercent(value: string | null | undefined): string {
  if (value == null || value === "") return "—"
  const n = Number(value)
  if (!Number.isFinite(n)) return value
  return `${n.toLocaleString("id-ID", { maximumFractionDigits: 4 })}%`
}

/** Input persen user ("12,5") → string desimal API ("12.5"), atau "" bila kosong. */
export function parsePercentInput(value: string): string {
  return value.trim().replace(/\./g, "").replace(",", ".")
}

// --- Daftar harga ------------------------------------------------------------

export type PriceListSearch = {
  q?: string
  priceListType?: PriceListType
  includeInactive?: boolean
  deleted?: boolean
}

export const priceListsQuery = (s: PriceListSearch) =>
  queryOptions({
    queryKey: ["pricing", "price-lists", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/price-lists", {
          params: {
            query: {
              q: s.q || undefined,
              priceListType: s.priceListType,
              includeInactive: s.includeInactive,
              deleted: s.deleted,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const priceListOptionsQuery = () =>
  queryOptions({
    queryKey: ["pricing", "price-list-options"],
    queryFn: () => unwrap(api.GET("/api/v1/price-lists/options")),
    staleTime: 60_000,
  })

export const priceListQuery = (id: string) =>
  queryOptions({
    queryKey: ["pricing", "price-list", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/price-lists/{id}", { params: { path: { id } } })),
  })

export const priceListVersionQuery = (id: string) =>
  queryOptions({
    queryKey: ["pricing", "price-list-version", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/price-list-versions/{id}", {
          params: { path: { id } },
        })
      ),
  })

export function createPriceList(body: PriceListInput, key: string) {
  return unwrap(api.POST("/api/v1/price-lists", { params: idem(key), body }))
}

export function updatePriceList(
  id: string,
  body: PriceListUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/price-lists/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deletePriceList(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/price-lists/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

export function restorePriceList(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/price-lists/{id}/restore", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function createPriceListVersion(
  priceListId: string,
  body: PriceListVersionInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/price-lists/{id}/versions", {
      params: { path: { id: priceListId }, ...idem(key) },
      body,
    })
  )
}

export function updatePriceListVersion(
  id: string,
  body: PriceListVersionUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/price-list-versions/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export type VersionAction = "submit" | "withdraw" | "cancel"

export function transitionPriceListVersion(
  id: string,
  action: VersionAction,
  version: number,
  key: string
) {
  const params = { path: { id }, ...idem(key) }
  const body = { version }
  switch (action) {
    case "submit":
      return unwrap(
        api.POST("/api/v1/price-list-versions/{id}/submit", { params, body })
      )
    case "withdraw":
      return unwrap(
        api.POST("/api/v1/price-list-versions/{id}/withdraw", { params, body })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/price-list-versions/{id}/cancel", { params, body })
      )
  }
}

// --- Aturan harga ------------------------------------------------------------

export const specialPricesQuery = (includeEnded: boolean) =>
  queryOptions({
    queryKey: ["pricing", "special-prices", includeEnded],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/special-prices", {
          params: { query: { includeEnded } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createSpecialPrice(body: SpecialPriceInput, key: string) {
  return unwrap(api.POST("/api/v1/special-prices", { params: idem(key), body }))
}

export const promotionsQuery = (includeEnded: boolean, q?: string) =>
  queryOptions({
    queryKey: ["pricing", "promotions", includeEnded, q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/promotions", {
          params: { query: { includeEnded, q: q || undefined } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const promotionQuery = (id: string) =>
  queryOptions({
    queryKey: ["pricing", "promotion", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/promotions/{id}", { params: { path: { id } } })),
  })

export function createPromotion(body: PromotionInput, key: string) {
  return unwrap(api.POST("/api/v1/promotions", { params: idem(key), body }))
}

export function updatePromotion(
  id: string,
  body: PromotionUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/promotions/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export const tieredPricesQuery = (includeEnded: boolean) =>
  queryOptions({
    queryKey: ["pricing", "tiered-prices", includeEnded],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/tiered-prices", {
          params: { query: { includeEnded } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createTieredPrice(body: TieredPriceInput, key: string) {
  return unwrap(api.POST("/api/v1/tiered-prices", { params: idem(key), body }))
}

// --- Batas diskon ------------------------------------------------------------

export const discountLimitsQuery = (includeEnded: boolean) =>
  queryOptions({
    queryKey: ["pricing", "discount-limits", includeEnded],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/discount-limits", {
          params: { query: { includeEnded } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const discountLimitRolesQuery = () =>
  queryOptions({
    queryKey: ["pricing", "discount-limit-roles"],
    queryFn: () => unwrap(api.GET("/api/v1/discount-limits/roles")),
    staleTime: 5 * 60_000,
  })

export function createDiscountLimit(body: DiscountLimitInput, key: string) {
  return unwrap(
    api.POST("/api/v1/discount-limits", { params: idem(key), body })
  )
}

// --- Data effective-dated: akhiri & hapus -------------------------------------

export type DatedResource =
  "special-prices" | "tiered-prices" | "discount-limits"

export function endDated(
  resource: DatedResource,
  id: string,
  validTo: string,
  version: number,
  key: string
): Promise<unknown> {
  const params = { path: { id }, ...idem(key) }
  const body = { validTo, version }
  switch (resource) {
    case "special-prices":
      return unwrap(
        api.POST("/api/v1/special-prices/{id}/end", { params, body })
      )
    case "tiered-prices":
      return unwrap(
        api.POST("/api/v1/tiered-prices/{id}/end", { params, body })
      )
    case "discount-limits":
      return unwrap(
        api.POST("/api/v1/discount-limits/{id}/end", { params, body })
      )
  }
}

export function deleteDated(
  resource: DatedResource | "promotions",
  id: string,
  version: number,
  key: string
): Promise<unknown> {
  const params = { path: { id }, query: { version }, ...idem(key) }
  switch (resource) {
    case "special-prices":
      return unwrap(api.DELETE("/api/v1/special-prices/{id}", { params }))
    case "tiered-prices":
      return unwrap(api.DELETE("/api/v1/tiered-prices/{id}", { params }))
    case "discount-limits":
      return unwrap(api.DELETE("/api/v1/discount-limits/{id}", { params }))
    case "promotions":
      return unwrap(api.DELETE("/api/v1/promotions/{id}", { params }))
  }
}

// --- Simulasi ----------------------------------------------------------------

export function quotePrice(body: PriceQuoteRequest) {
  return unwrap(api.POST("/api/v1/pricing/quote", { body }))
}

export function checkDiscount(body: DiscountCheckRequest) {
  return unwrap(api.POST("/api/v1/pricing/discount-check", { body }))
}

/** Pilihan pelanggan untuk harga khusus dan simulasi harga. */
export const customerOptionsQuery = (q: string) =>
  queryOptions({
    queryKey: ["party", "customer-options", q],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/customers", {
            params: { query: { q: q || undefined, pageSize: 20 } },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })
