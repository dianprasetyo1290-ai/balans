import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Customer = components["schemas"]["Customer"]
export type CustomerInput = components["schemas"]["CustomerInput"]
export type CustomerType = components["schemas"]["CustomerType"]
export type CustomerAddressInput = components["schemas"]["CustomerAddressInput"]
export type Partner = components["schemas"]["Partner"]
export type PartnerInput = components["schemas"]["PartnerInput"]
export type PartnerType = components["schemas"]["PartnerType"]
export type PartnerAgreement = components["schemas"]["PartnerAgreement"]
export type PartnerAgreementInput =
  components["schemas"]["PartnerAgreementInput"]
export type AgreementType = components["schemas"]["AgreementType"]
export type BorneBy = components["schemas"]["BorneBy"]
export type PartnerProductCode = components["schemas"]["PartnerProductCode"]
export type PartnerProductCodeInput =
  components["schemas"]["PartnerProductCodeInput"]
export type Supplier = components["schemas"]["Supplier"]
export type SupplierInput = components["schemas"]["SupplierInput"]
export type SupplierType = components["schemas"]["SupplierType"]
export type WithholdingType = components["schemas"]["WithholdingType"]
export type PartyUserOption = components["schemas"]["PartyUserOption"]

/** Label UI tipe & jenis master (Naming Convention bagian 4). */
export const CUSTOMER_TYPE_LABELS: Record<CustomerType, string> = {
  retail: "Retail",
  b2b: "B2B",
  reseller: "Reseller",
  employee: "Karyawan",
}

export const PARTNER_TYPE_LABELS: Record<PartnerType, string> = {
  modern_trade: "Modern Trade",
  consignment: "Konsinyasi",
  b2b: "B2B",
}

export const AGREEMENT_TYPE_LABELS: Record<AgreementType, string> = {
  consignment: "Konsinyasi",
  modern_trade: "Modern Trade",
}

export const AGREEMENT_STATUS_LABELS: Record<
  PartnerAgreement["status"],
  string
> = {
  scheduled: "Terjadwal",
  active: "Berlaku",
  expired: "Kedaluwarsa",
}

export const BORNE_BY_LABELS: Record<BorneBy, string> = {
  partner: "Mitra",
  balans: "Balans",
}

export const SUPPLIER_TYPE_LABELS: Record<SupplierType, string> = {
  supplier: "Pemasok",
  toll_manufacturer: "Maklon",
}

export const WITHHOLDING_LABELS: Record<WithholdingType, string> = {
  none: "Tanpa PPh",
  pph23: "PPh 23",
  pph21: "PPh 21",
  pph4_2: "PPh 4(2)",
}

/** Ketentuan selisih rekonsiliasi konsinyasi (BRD 5.5, U-02) beserta default-nya. */
export const DISCREPANCY_RULES = [
  { field: "lossBorneBy", label: "Barang hilang", fallback: "partner" },
  {
    field: "partnerDamageBorneBy",
    label: "Rusak karena mitra",
    fallback: "partner",
  },
  {
    field: "productDefectBorneBy",
    label: "Cacat produk",
    fallback: "balans",
  },
  {
    field: "nearExpiryBorneBy",
    label: "Mendekati kedaluwarsa",
    fallback: "balans",
  },
] as const satisfies readonly {
  field: keyof PartnerAgreementInput
  label: string
  fallback: BorneBy
}[]

/** Batas ukuran PDF PKS; server menolak di atas ini dengan FILE_TOO_LARGE. */
export const MAX_AGREEMENT_FILE_BYTES = 10 * 1024 * 1024

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export type ListSearch = { page: number; sort?: string; q?: string }

// --- Pelanggan ---------------------------------------------------------------

export type CustomerSearch = ListSearch & { customerType?: CustomerType }

export const customersQuery = (s: CustomerSearch) =>
  queryOptions({
    queryKey: ["party", "customers", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/customers", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              customerType: s.customerType,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createCustomer(body: CustomerInput, key: string) {
  return unwrap(api.POST("/api/v1/customers", { params: idem(key), body }))
}

export function updateCustomer(
  id: string,
  body: CustomerInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/customers/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteCustomer(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/customers/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

// --- Mitra & PKS -------------------------------------------------------------

export type PartnerSearch = ListSearch & { partnerType?: PartnerType }

export const partnersQuery = (s: PartnerSearch) =>
  queryOptions({
    queryKey: ["party", "partners", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/partners", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              partnerType: s.partnerType,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const partnerOptionsQuery = (q: string) =>
  queryOptions({
    queryKey: ["party", "partner-options", q],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/partners", {
            params: { query: { q: q || undefined, pageSize: 20 } },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

export function createPartner(body: PartnerInput, key: string) {
  return unwrap(api.POST("/api/v1/partners", { params: idem(key), body }))
}

export function updatePartner(
  id: string,
  body: PartnerInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/partners/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deletePartner(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/partners/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

export const partnerAgreementsQuery = (partnerId: string) =>
  queryOptions({
    queryKey: ["party", "agreements", partnerId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/partners/{id}/agreements", {
          params: { path: { id: partnerId } },
        })
      ),
  })

export function createAgreement(
  partnerId: string,
  body: PartnerAgreementInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/partners/{id}/agreements", {
      params: { path: { id: partnerId }, ...idem(key) },
      body,
    })
  )
}

export function updateAgreement(
  id: string,
  body: PartnerAgreementInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/partner-agreements/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteAgreement(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/partner-agreements/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

export const partyUserOptionsQuery = (q: string, role?: string) =>
  queryOptions({
    queryKey: ["party", "user-options", q, role ?? ""],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/party/user-options", {
          params: { query: { q: q || undefined, role } },
        })
      ),
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

// --- Kode produk mitra -------------------------------------------------------

export type ProductCodeSearch = { page: number; q?: string; partnerId?: string }

export const productCodesQuery = (s: ProductCodeSearch) =>
  queryOptions({
    queryKey: ["party", "product-codes", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/partner-product-codes", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              partnerId: s.partnerId,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createProductCode(body: PartnerProductCodeInput, key: string) {
  return unwrap(
    api.POST("/api/v1/partner-product-codes", { params: idem(key), body })
  )
}

export function updateProductCode(
  id: string,
  body: {
    articleCode: string
    barcode?: string
    description?: string
    version: number
  },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/partner-product-codes/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteProductCode(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/partner-product-codes/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

/** Pilihan produk dari modul catalog (Data Master › Produk). */
export const productOptionsQuery = (q: string) =>
  queryOptions({
    queryKey: ["catalog", "product-options", q],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/products", {
            params: { query: { q: q || undefined, pageSize: 20 } },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

// --- Pemasok & maklon --------------------------------------------------------

export type SupplierSearch = ListSearch & { supplierType?: SupplierType }

export const suppliersQuery = (s: SupplierSearch) =>
  queryOptions({
    queryKey: ["party", "suppliers", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/suppliers", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              supplierType: s.supplierType,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createSupplier(body: SupplierInput, key: string) {
  return unwrap(api.POST("/api/v1/suppliers", { params: idem(key), body }))
}

export function updateSupplier(
  id: string,
  body: SupplierInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/suppliers/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteSupplier(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/suppliers/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

// --- File --------------------------------------------------------------------

/** Meminta URL upload PDF PKS (Technical Guideline 5.16). */
export async function requestAgreementUpload(file: File) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType: "partner_agreement",
        fileName: file.name,
        contentType: file.type || "application/pdf",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

/** Membuka file di tab baru lewat URL unduhan bertanda tangan berumur 5 menit. */
export async function openFile(fileId: string) {
  const target = await unwrap(
    api.GET("/api/v1/files/{id}/download-url", {
      params: { path: { id: fileId } },
    })
  )
  window.open(target.url, "_blank", "noopener")
}
