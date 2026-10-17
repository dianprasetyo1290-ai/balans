import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type SalesOrder = components["schemas"]["SalesOrder"]
export type SalesOrderSummary = components["schemas"]["SalesOrderSummary"]
export type SalesOrderType = components["schemas"]["SalesOrderType"]
export type SalesDocumentStatus = components["schemas"]["SalesDocumentStatus"]
export type SalesOrderFields = components["schemas"]["SalesOrderFields"]
export type SalesOrderLineInput = components["schemas"]["SalesOrderLineInput"]
export type SalesDiscount = components["schemas"]["SalesDiscount"]
export type SalesPaymentMode = components["schemas"]["SalesPaymentMode"]
export type SalesChannelOption = components["schemas"]["SalesChannelOption"]
export type SalesLocationRef = components["schemas"]["SalesLocationRef"]
export type SalesAccountOption = components["schemas"]["SalesAccountOption"]
export type ProformaInvoice = components["schemas"]["ProformaInvoice"]
export type ProformaInvoiceDetail =
  components["schemas"]["ProformaInvoiceDetail"]
export type SalesInvoice = components["schemas"]["SalesInvoice"]
export type SalesInvoiceSummary = components["schemas"]["SalesInvoiceSummary"]
export type SalesPartyType = components["schemas"]["SalesPartyType"]
export type CustomerPayment = components["schemas"]["CustomerPayment"]
export type CustomerPaymentInput = components["schemas"]["CustomerPaymentInput"]
export type CustomerPaymentMethod =
  components["schemas"]["CustomerPaymentMethod"]
export type ReceivableAgingReport =
  components["schemas"]["ReceivableAgingReport"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const ORDER_TYPE_LABELS: Record<SalesOrderType, string> = {
  modern_trade: "Modern Trade",
  b2b: "B2B / Corporate",
  social_commerce: "WhatsApp/Instagram",
  marketplace: "Marketplace",
  employee: "Penjualan Karyawan",
}

export const PRICE_RULE_LABELS: Record<string, string> = {
  special_price: "Harga khusus",
  promotion: "Promo",
  tiered_price: "Harga bertingkat",
  price_list: "Daftar harga",
  base_price: "Harga Dasar",
  marketplace_input: "Harga marketplace",
}

export const PAYMENT_METHOD_LABELS: Record<CustomerPaymentMethod, string> = {
  bank_transfer: "Transfer bank",
  cash: "Tunai",
}

/** Halaman daftar pesanan per menu Penjualan (Naming Convention 5). */
export type OrderKind =
  "all" | "modern_trade" | "marketplace" | "social_commerce" | "employee"

type OrderPath =
  | "/sales/orders"
  | "/sales/modern-trade"
  | "/sales/marketplace"
  | "/sales/social-commerce"
  | "/sales/employee-sales"

export const ORDER_KINDS: Record<
  OrderKind,
  {
    title: string
    description: string
    types: SalesOrderType[]
    path: OrderPath
    /** Jenis pesanan yang dibuat dari tombol Buat di halaman ini. */
    create: SalesOrderType[]
  }
> = {
  all: {
    title: "Sales Order",
    description:
      "Semua pesanan lintas channel. Harga otomatis dari daftar harga; konfirmasi mengalokasikan stok FEFO.",
    types: [],
    path: "/sales/orders",
    create: ["modern_trade", "b2b", "social_commerce", "employee"],
  },
  modern_trade: {
    title: "Modern Trade",
    description:
      "PO klien MT beli putus dan B2B/corporate: alokasi FG Non-Ecommerce, Proforma Invoice berlaku 14 hari atau invoice tempo.",
    types: ["modern_trade", "b2b"],
    path: "/sales/modern-trade",
    create: ["modern_trade", "b2b"],
  },
  marketplace: {
    title: "Penjualan Marketplace",
    description:
      "Order marketplace dicatat manual atau impor Excel sebelum integrasi: nomor order tidak bisa ganda, E-commerce Stock langsung dialokasikan, bundel dipecah ke komponen. Penjualan diakui saat serah ke kurir.",
    types: ["marketplace"],
    path: "/sales/marketplace",
    create: ["marketplace"],
  },
  social_commerce: {
    title: "Pesanan WhatsApp/Instagram",
    description:
      "Pesanan dari CS: cari pelanggan dari nomor HP, item & ongkir, unggah bukti bayar, F&A konfirmasi, kirim invoice ke WhatsApp.",
    types: ["social_commerce"],
    path: "/sales/social-commerce",
    create: ["social_commerce"],
  },
  employee: {
    title: "Penjualan Karyawan",
    description:
      "Pelanggan berstatus karyawan otomatis memakai daftar harga karyawan; karyawan yang resign kehilangan harga karyawan.",
    types: ["employee"],
    path: "/sales/employee-sales",
    create: ["employee"],
  },
}

export function orderKindOf(t: SalesOrderType): OrderKind {
  switch (t) {
    case "modern_trade":
    case "b2b":
      return "modern_trade"
    case "marketplace":
      return "marketplace"
    case "social_commerce":
      return "social_commerce"
    case "employee":
      return "employee"
  }
}

export const ORDER_STATUSES: SalesDocumentStatus[] = [
  "draft",
  "pending_approval",
  "awaiting_payment",
  "in_progress",
  "in_transit",
  "completed",
  "expired",
  "cancelled",
]

export const PROFORMA_STATUSES: SalesDocumentStatus[] = [
  "awaiting_payment",
  "paid",
  "expired",
  "cancelled",
]

export const INVOICE_STATUSES: SalesDocumentStatus[] = [
  "awaiting_payment",
  "partially_paid",
  "paid",
  "cancelled",
]

// --- Pilihan ------------------------------------------------------------------------------------

export const channelOptionsQuery = queryOptions({
  queryKey: ["sales", "channel-options"],
  queryFn: () => unwrap(api.GET("/api/v1/sales/channel-options")),
  staleTime: 5 * 60_000,
})

export const locationOptionsQuery = (orderType: SalesOrderType) =>
  queryOptions({
    queryKey: ["sales", "location-options", orderType],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/location-options", {
          params: { query: { orderType } },
        })
      ),
    staleTime: 5 * 60_000,
  })

export const cashBankAccountsQuery = queryOptions({
  queryKey: ["sales", "cash-bank-accounts"],
  queryFn: () => unwrap(api.GET("/api/v1/sales/cash-bank-accounts")),
  staleTime: 5 * 60_000,
})

/** Cari pelanggan berdasarkan nama/kode/nomor HP (format apa pun). */
export const customerOptionsQuery = (q: string, customerType?: "employee") =>
  queryOptions({
    queryKey: ["sales", "customer-options", q, customerType],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/customers", {
            params: {
              query: { q: q || undefined, customerType, pageSize: 20 },
            },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

export const partnerOptionsQuery = (
  q: string,
  partnerType?: "modern_trade" | "b2b"
) =>
  queryOptions({
    queryKey: ["sales", "partner-options", q, partnerType],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/partners", {
            params: { query: { q: q || undefined, partnerType, pageSize: 20 } },
          })
        )
      ).items,
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

// --- Sales Order --------------------------------------------------------------------------------

export type OrderListSearch = {
  page: number
  sort?: string
  q?: string
  status?: SalesDocumentStatus
  orderType?: SalesOrderType
  dateFrom?: string
  dateTo?: string
}

export const ordersQuery = (kind: OrderKind, s: OrderListSearch) =>
  queryOptions({
    queryKey: ["sales", "orders", kind, s],
    queryFn: () => {
      const types = s.orderType ? [s.orderType] : ORDER_KINDS[kind].types
      return unwrap(
        api.GET("/api/v1/sales/orders", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              status: s.status,
              orderType: types.length > 0 ? types : undefined,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      )
    },
    placeholderData: keepPreviousData,
  })

export const orderQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "order", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/orders/{id}", { params: { path: { id } } })
      ),
  })

export function createOrder(
  body: SalesOrderFields & { orderType: SalesOrderType },
  key: string
) {
  return unwrap(api.POST("/api/v1/sales/orders", { params: idem(key), body }))
}

export function updateOrder(
  id: string,
  body: SalesOrderFields & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/sales/orders/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function confirmOrder(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/sales/orders/{id}/confirm", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function cancelOrder(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/orders/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason: reason || undefined },
    })
  )
}

export function attachPaymentProof(
  id: string,
  version: number,
  fileId: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/orders/{id}/payment-proof", {
      params: { path: { id }, ...idem(key) },
      body: { version, fileId },
    })
  )
}

/** Batas ukuran bukti bayar; server menolak di atas ini dengan FILE_TOO_LARGE. */
export const MAX_PROOF_FILE_BYTES = 10 * 1024 * 1024

/** Meminta URL upload bukti bayar (foto/scan transfer). */
export async function requestProofUpload(
  file: File,
  ownerType: "sales.payment_proof" | "sales.customer_payment_proof"
) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType,
        fileName: file.name,
        contentType: file.type || "image/jpeg",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

// --- Proforma Invoice ---------------------------------------------------------------------------

export type ProformaListSearch = {
  page: number
  q?: string
  status?: SalesDocumentStatus
}

export const proformasQuery = (s: ProformaListSearch) =>
  queryOptions({
    queryKey: ["sales", "proformas", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/proforma-invoices", {
          params: {
            query: { page: s.page, q: s.q || undefined, status: s.status },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const proformaQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "proforma", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/proforma-invoices/{id}", {
          params: { path: { id } },
        })
      ),
  })

export type ProformaPayment = {
  paymentDate: string
  method: CustomerPaymentMethod
  accountCode: string
  reference: string
  proofFileId: string | null
}

export function confirmProformaPayment(
  id: string,
  version: number,
  body: ProformaPayment,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/proforma-invoices/{id}/confirm-payment", {
      params: { path: { id }, ...idem(key) },
      body: { version, ...body, reference: body.reference || undefined },
    })
  )
}

// --- Invoice ------------------------------------------------------------------------------------

export type InvoiceListSearch = {
  page: number
  sort?: string
  q?: string
  status?: SalesDocumentStatus
  overdue?: boolean
  partyType?: SalesPartyType
  partyId?: string
  dateFrom?: string
  dateTo?: string
}

export const invoicesQuery = (s: InvoiceListSearch, open = false) =>
  queryOptions({
    queryKey: ["sales", "invoices", s, open],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/invoices", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              status: s.status
                ? [s.status]
                : open
                  ? ["awaiting_payment", "partially_paid"]
                  : undefined,
              overdue: s.overdue || undefined,
              partyType: s.partyType,
              partyId: s.partyId,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const invoiceQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "invoice", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/invoices/{id}", { params: { path: { id } } })
      ),
  })

export function invoiceShareText(id: string) {
  return unwrap(
    api.GET("/api/v1/sales/invoices/{id}/share-text", {
      params: { path: { id } },
    })
  )
}

/** Invoice terbuka satu pihak untuk form pembayaran (alokasi ke banyak invoice). */
export const openInvoicesQuery = (partyType: SalesPartyType, partyId: string) =>
  queryOptions({
    queryKey: ["sales", "open-invoices", partyType, partyId],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/sales/invoices", {
            params: {
              query: {
                partyType,
                partyId,
                status: ["awaiting_payment", "partially_paid"],
                sort: "dueDate",
                pageSize: 200,
              },
            },
          })
        )
      ).items,
  })

// --- Pembayaran pelanggan & piutang -------------------------------------------------------------

export type PaymentListSearch = {
  page: number
  q?: string
  status?: SalesDocumentStatus
  dateFrom?: string
  dateTo?: string
}

export const paymentsQuery = (s: PaymentListSearch) =>
  queryOptions({
    queryKey: ["sales", "payments", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/customer-payments", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createPayment(body: CustomerPaymentInput, key: string) {
  return unwrap(
    api.POST("/api/v1/sales/customer-payments", { params: idem(key), body })
  )
}

export function cancelPayment(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/customer-payments/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}

export const agingQuery = (asOf: string, partyType?: SalesPartyType) =>
  queryOptions({
    queryKey: ["sales", "aging", asOf, partyType],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/receivable-aging", {
          params: { query: { asOf: asOf || undefined, partyType } },
        })
      ),
    placeholderData: keepPreviousData,
  })
