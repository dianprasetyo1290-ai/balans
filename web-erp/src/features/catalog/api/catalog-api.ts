import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Product = components["schemas"]["Product"]
export type ProductInput = components["schemas"]["ProductInput"]
export type ProductUpdate = components["schemas"]["ProductUpdate"]
export type ProductType = components["schemas"]["ProductType"]
export type PackagingType = components["schemas"]["PackagingType"]
export type ProductCategory = components["schemas"]["ProductCategory"]

/** Label jenis produk sesuai Naming Convention bagian 4. */
export const PRODUCT_TYPE_LABELS: Record<ProductType, string> = {
  finished_good: "Produk Jadi",
  packaging: "Packaging",
  reprocess_material: "Material",
}

export const PACKAGING_TYPE_LABELS: Record<PackagingType, string> = {
  tube: "Tube",
  pouch: "Pouch",
  roll_sachet: "Roll Sachet",
}

/**
 * Tiga halaman Data Master memakai daftar produk yang sama dengan filter berbeda:
 * Produk (Produk Jadi non-bundel), Packaging & Material, dan Bundling.
 */
export type ProductKind = "finished" | "material" | "bundle"

export const PRODUCT_KINDS: Record<
  ProductKind,
  {
    title: string
    description: string
    noun: string
    to: string
    types: ProductType[]
    isBundle: boolean
  }
> = {
  finished: {
    title: "Produk",
    description:
      "Produk Jadi (SKU) dengan barcode, umur simpan, dan isi per karton.",
    noun: "Produk",
    to: "/master-data/products",
    types: ["finished_good"],
    isBundle: false,
  },
  material: {
    title: "Packaging & Material",
    description:
      "Packaging (RM) untuk maklon dan Material (GM) barang jadi yang menunggu reprocess.",
    noun: "Packaging/Material",
    to: "/master-data/packaging-materials",
    types: ["packaging", "reprocess_material"],
    isBundle: false,
  },
  bundle: {
    title: "Bundling",
    description:
      "Bundel virtual tanpa stok sendiri; stok tersedia dihitung dari stok komponennya.",
    noun: "Bundel",
    to: "/master-data/bundles",
    types: ["finished_good"],
    isBundle: true,
  },
}

export function productKindOf(p: Pick<Product, "productType" | "isBundle">) {
  if (p.isBundle) return "bundle"
  return p.productType === "finished_good" ? "finished" : "material"
}

export function productDisplayName(p: Pick<Product, "name" | "variant">) {
  return p.variant ? `${p.name} ${p.variant}` : p.name
}

export type ProductListSearch = {
  page: number
  sort?: string
  q?: string
  productType?: ProductType
  categoryId?: string
  isActive?: boolean
  deleted?: boolean
}

export const productsQuery = (kind: ProductKind, s: ProductListSearch) =>
  queryOptions({
    queryKey: ["catalog", "products", kind, s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/products", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              productType: s.productType
                ? [s.productType]
                : PRODUCT_KINDS[kind].types,
              isBundle: PRODUCT_KINDS[kind].isBundle,
              categoryId: s.categoryId || undefined,
              isActive: s.isActive,
              deleted: s.deleted || undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

/** Pencarian produk untuk dropdown (komponen bundel, label produk): hanya yang aktif dan belum dihapus. */
export const productOptionsQuery = (
  q: string,
  filter: { types?: ProductType[]; isBundle?: boolean } = {}
) =>
  queryOptions({
    queryKey: ["catalog", "product-options", q, filter],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/products", {
          params: {
            query: {
              q: q || undefined,
              pageSize: 20,
              sort: "sku",
              productType: filter.types,
              isBundle: filter.isBundle,
              isActive: true,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const productQuery = (id: string) =>
  queryOptions({
    queryKey: ["catalog", "product", id],
    queryFn: () =>
      unwrap(api.GET("/api/v1/products/{id}", { params: { path: { id } } })),
  })

export const categoriesQuery = (deleted = false) =>
  queryOptions({
    queryKey: ["catalog", "categories", deleted],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/product-categories", {
          params: { query: { deleted: deleted || undefined } },
        })
      ),
  })

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export function createProduct(body: ProductInput, key: string) {
  return unwrap(api.POST("/api/v1/products", { params: idem(key), body }))
}

export function updateProduct(id: string, body: ProductUpdate, key: string) {
  return unwrap(
    api.PUT("/api/v1/products/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteProduct(id: string, key: string) {
  return unwrap(
    api.DELETE("/api/v1/products/{id}", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function restoreProduct(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/products/{id}/restore", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function createCategory(code: string, name: string, key: string) {
  return unwrap(
    api.POST("/api/v1/product-categories", {
      params: idem(key),
      body: { code, name },
    })
  )
}

export function updateCategory(
  id: string,
  name: string,
  version: number,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/product-categories/{id}", {
      params: { path: { id }, ...idem(key) },
      body: { name, version },
    })
  )
}

export function deleteCategory(id: string, key: string) {
  return unwrap(
    api.DELETE("/api/v1/product-categories/{id}", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function restoreCategory(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/product-categories/{id}/restore", {
      params: { path: { id }, ...idem(key) },
    })
  )
}
