import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type SalesReturn = components["schemas"]["SalesReturn"]
export type SalesReturnSummary = components["schemas"]["SalesReturnSummary"]
export type SalesReturnStatus = components["schemas"]["SalesReturnStatus"]
export type SalesReturnSettlement =
  components["schemas"]["SalesReturnSettlement"]
export type SalesReturnSource = components["schemas"]["SalesReturnSource"]
export type SalesReturnSourceDetail =
  components["schemas"]["SalesReturnSourceDetail"]
export type SalesReturnFields = components["schemas"]["SalesReturnFields"]
export type SalesReturnAction = SalesReturn["allowedActions"][number]
export type CreditNote = components["schemas"]["CreditNote"]
export type CreditNoteSummary = components["schemas"]["CreditNoteSummary"]
export type CreditNoteStatus = components["schemas"]["CreditNoteStatus"]
export type CreditNoteSettlement = components["schemas"]["CreditNoteSettlement"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const RETURN_STATUSES: SalesReturnStatus[] = [
  "draft",
  "submitted",
  "verified",
  "in_progress",
  "posted",
  "cancelled",
]

export const CREDIT_NOTE_STATUSES: CreditNoteStatus[] = [
  "posted",
  "completed",
  "pending_approval",
  "awaiting_payment",
  "paid",
]

export const SETTLEMENT_LABELS: Record<SalesReturnSettlement, string> = {
  credit_note: "Nota kredit",
  refund: "Refund",
  marketplace: "Potongan settlement marketplace",
}

/** Status Retur Penjualan yang belum terdaftar di label umum. */
export const RETURN_STATUS_LABELS: Record<SalesReturnStatus, string> = {
  draft: "Draft",
  submitted: "Diajukan",
  verified: "Terverifikasi",
  in_progress: "Diproses",
  posted: "Diposting",
  cancelled: "Dibatalkan",
}

export type ReturnListSearch = {
  page: number
  q?: string
  status?: SalesReturnStatus
  assignedToMe?: boolean
}

export const salesReturnsQuery = (s: ReturnListSearch) =>
  queryOptions({
    queryKey: ["sales", "returns", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/returns", {
          params: {
            query: {
              page: s.page,
              q: s.q,
              status: s.status ? [s.status] : undefined,
              assignedToMe: s.assignedToMe || undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const salesReturnQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "return", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/returns/{id}", { params: { path: { id } } })
      ),
  })

export const returnSourcesQuery = (q: string) =>
  queryOptions({
    queryKey: ["sales", "return-sources", q],
    queryFn: async () =>
      (
        await unwrap(
          api.GET("/api/v1/sales/return-sources", {
            params: { query: { q: q || undefined } },
          })
        )
      ).items,
  })

export const returnSourceQuery = (orderId: string) =>
  queryOptions({
    queryKey: ["sales", "return-source", orderId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/return-sources/{id}", {
          params: { path: { id: orderId } },
        })
      ),
  })

export const pickupUsersQuery = queryOptions({
  queryKey: ["sales", "return-pickup-users"],
  queryFn: async () =>
    (await unwrap(api.GET("/api/v1/sales/return-pickup-users"))).items,
  staleTime: 60_000,
})

export function saveSalesReturn(
  existing: SalesReturn | undefined,
  orderId: string,
  body: SalesReturnFields,
  key: string
) {
  if (existing) {
    return unwrap(
      api.PUT("/api/v1/sales/returns/{id}", {
        params: { path: { id: existing.id }, ...idem(key) },
        body: { ...body, version: existing.version },
      })
    )
  }
  return unwrap(
    api.POST("/api/v1/sales/returns", {
      params: idem(key),
      body: { ...body, salesOrderId: orderId },
    })
  )
}

export type ReturnActionInput =
  | { kind: "submit" }
  | { kind: "send_back"; note: string }
  | { kind: "verify"; pickupRequired: boolean; note?: string }
  | { kind: "assign_pickup"; pickupUserId: string }
  | { kind: "record_pickup"; note?: string; photoFileIds: string[] }
  | {
      kind: "receive"
      note?: string
      documentationFileIds: string[]
      lines?: { lineId: string; qty: string }[]
    }
  | { kind: "cancel"; reason?: string }

/** Menjalankan aksi status retur (allowedActions). */
export function salesReturnAction(
  id: string,
  version: number,
  input: ReturnActionInput,
  key: string
) {
  const path = { params: { path: { id }, ...idem(key) } }
  switch (input.kind) {
    case "submit":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/submit", {
          ...path,
          body: { version },
        })
      )
    case "send_back":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/send-back", {
          ...path,
          body: { version, note: input.note },
        })
      )
    case "verify":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/verify", {
          ...path,
          body: {
            version,
            pickupRequired: input.pickupRequired,
            note: input.note,
          },
        })
      )
    case "assign_pickup":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/assign-pickup", {
          ...path,
          body: { version, pickupUserId: input.pickupUserId },
        })
      )
    case "record_pickup":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/pickup", {
          ...path,
          body: { version, note: input.note, photoFileIds: input.photoFileIds },
        })
      )
    case "receive":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/receive", {
          ...path,
          body: {
            version,
            note: input.note,
            documentationFileIds: input.documentationFileIds,
            lines: input.lines,
          },
        })
      )
    case "cancel":
      return unwrap(
        api.POST("/api/v1/sales/returns/{id}/cancel", {
          ...path,
          body: { version, reason: input.reason },
        })
      )
  }
}

export async function requestReturnPhotoUpload(file: File) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType: "sales.sales_return_photo",
        fileName: file.name,
        contentType: file.type || "image/jpeg",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

// --- Nota Kredit -------------------------------------------------------------------------------

export type CreditNoteListSearch = {
  page: number
  q?: string
  status?: CreditNoteStatus
}

export const creditNotesQuery = (s: CreditNoteListSearch) =>
  queryOptions({
    queryKey: ["sales", "credit-notes", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/credit-notes", {
          params: {
            query: {
              page: s.page,
              q: s.q,
              status: s.status ? [s.status] : undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const creditNoteQuery = (id: string) =>
  queryOptions({
    queryKey: ["sales", "credit-note", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sales/credit-notes/{id}", { params: { path: { id } } })
      ),
  })

export function applyCreditNote(
  id: string,
  version: number,
  allocations: { invoiceId: string; amount: string }[],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/credit-notes/{id}/apply", {
      params: { path: { id }, ...idem(key) },
      body: { version, allocations },
    })
  )
}

export function payCreditNoteRefund(
  id: string,
  body: {
    version: number
    paymentDate: string
    accountCode: string
    reference?: string
  },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/sales/credit-notes/{id}/pay-refund", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}
