import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Account = components["schemas"]["Account"]
export type AccountInput = components["schemas"]["AccountInput"]
export type AccountUpdate = components["schemas"]["AccountUpdate"]
export type Dimension = components["schemas"]["Dimension"]
export type DimensionKind = components["schemas"]["DimensionKind"]
export type DimensionRef = components["schemas"]["DimensionRef"]
export type CampaignInput = components["schemas"]["CampaignInput"]
export type FiscalPeriod = components["schemas"]["FiscalPeriod"]
export type JournalEntry = components["schemas"]["JournalEntry"]
export type JournalEntrySummary = components["schemas"]["JournalEntrySummary"]
export type JournalStatus = components["schemas"]["JournalStatus"]
export type JournalEntryType = components["schemas"]["JournalEntryType"]
export type ManualJournalInput = components["schemas"]["ManualJournalFields"]
export type ManualJournalLineInput =
  components["schemas"]["ManualJournalLineInput"]
export type GeneralLedger = components["schemas"]["GeneralLedger"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const ENTRY_TYPE_LABELS: Record<JournalEntryType, string> = {
  manual: "Jurnal Umum",
  automatic: "Otomatis",
  reversal: "Balik Jurnal",
}

// --- Akun -------------------------------------------------------------------------------------

export const accountsQuery = (includeInactive: boolean) =>
  queryOptions({
    queryKey: ["accounting", "accounts", includeInactive],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/accounts", {
          params: { query: { includeInactive } },
        })
      ),
    staleTime: 60_000,
  })

export function createAccount(body: AccountInput, key: string) {
  return unwrap(api.POST("/api/v1/accounts", { params: idem(key), body }))
}

export function updateAccount(id: string, body: AccountUpdate, key: string) {
  return unwrap(
    api.PUT("/api/v1/accounts/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteAccount(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/accounts/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

// --- Dimensi ----------------------------------------------------------------------------------

export const DIMENSION_LABELS: Record<DimensionKind, string> = {
  channel: "Channel",
  sub_channel: "Sub-channel",
  division: "Divisi",
  campaign: "Kampanye",
}

export const channelsQuery = (includeInactive = false) =>
  queryOptions({
    queryKey: ["accounting", "channels", includeInactive],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/channels", { params: { query: { includeInactive } } })
      ),
    staleTime: 60_000,
  })

export const subChannelsQuery = (includeInactive = false) =>
  queryOptions({
    queryKey: ["accounting", "sub-channels", includeInactive],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/sub-channels", {
          params: { query: { includeInactive } },
        })
      ),
    staleTime: 60_000,
  })

export const divisionsQuery = (includeInactive = false) =>
  queryOptions({
    queryKey: ["accounting", "divisions", includeInactive],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/divisions", { params: { query: { includeInactive } } })
      ),
    staleTime: 60_000,
  })

export const campaignsQuery = (includeInactive = false, q = "") =>
  queryOptions({
    queryKey: ["accounting", "campaigns", includeInactive, q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/campaigns", {
          params: { query: { includeInactive, q: q || undefined } },
        })
      ),
    staleTime: 60_000,
    placeholderData: keepPreviousData,
  })

export type SimpleDimensionKind = "channel" | "division"

export function createSimpleDimension(
  kind: SimpleDimensionKind,
  body: { code: string; name: string; isActive: boolean },
  key: string
) {
  return kind === "channel"
    ? unwrap(api.POST("/api/v1/channels", { params: idem(key), body }))
    : unwrap(api.POST("/api/v1/divisions", { params: idem(key), body }))
}

export function updateSimpleDimension(
  kind: SimpleDimensionKind,
  id: string,
  body: { name: string; isActive: boolean; version: number },
  key: string
) {
  const params = { path: { id }, ...idem(key) }
  return kind === "channel"
    ? unwrap(api.PUT("/api/v1/channels/{id}", { params, body }))
    : unwrap(api.PUT("/api/v1/divisions/{id}", { params, body }))
}

export function createSubChannel(
  body: { channelId: string; code: string; name: string; isActive: boolean },
  key: string
) {
  return unwrap(api.POST("/api/v1/sub-channels", { params: idem(key), body }))
}

export function updateSubChannel(
  id: string,
  body: { name: string; isActive: boolean; version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/sub-channels/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function createCampaign(body: CampaignInput, key: string) {
  return unwrap(api.POST("/api/v1/campaigns", { params: idem(key), body }))
}

export function updateCampaign(
  id: string,
  body: Omit<CampaignInput, "code"> & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/campaigns/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteDimension(d: Dimension, key: string) {
  const params = {
    path: { id: d.id },
    query: { version: d.version },
    ...idem(key),
  }
  switch (d.kind) {
    case "channel":
      return unwrap(api.DELETE("/api/v1/channels/{id}", { params }))
    case "sub_channel":
      return unwrap(api.DELETE("/api/v1/sub-channels/{id}", { params }))
    case "division":
      return unwrap(api.DELETE("/api/v1/divisions/{id}", { params }))
    case "campaign":
      return unwrap(api.DELETE("/api/v1/campaigns/{id}", { params }))
  }
}

// --- Periode ----------------------------------------------------------------------------------

export const fiscalPeriodsQuery = (year?: number) =>
  queryOptions({
    queryKey: ["accounting", "fiscal-periods", year ?? 0],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/fiscal-periods", { params: { query: { year } } })
      ),
  })

export function ensureFiscalPeriods(key: string) {
  return unwrap(
    api.POST("/api/v1/fiscal-periods/ensure", { params: idem(key) })
  )
}

// --- Jurnal -----------------------------------------------------------------------------------

export type JournalSearch = {
  page: number
  q?: string
  status?: JournalStatus
  entryType?: JournalEntryType
  dateFrom?: string
  dateTo?: string
}

export const journalsQuery = (s: JournalSearch) =>
  queryOptions({
    queryKey: ["accounting", "journals", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/journal-entries", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              entryType: s.entryType,
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const journalQuery = (id: string) =>
  queryOptions({
    queryKey: ["accounting", "journal", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/journal-entries/{id}", { params: { path: { id } } })
      ),
  })

export function createJournal(body: ManualJournalInput, key: string) {
  return unwrap(
    api.POST("/api/v1/journal-entries", { params: idem(key), body })
  )
}

export function updateJournal(
  id: string,
  body: ManualJournalInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/journal-entries/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteJournal(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/journal-entries/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

export function postJournal(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/journal-entries/{id}/post", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function reverseJournal(
  id: string,
  body: { version: number; businessDate: string; reason: string },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/journal-entries/{id}/reverse", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

// --- Buku Besar -------------------------------------------------------------------------------

export type LedgerSearch = {
  accountId?: string
  dateFrom: string
  dateTo: string
  divisionId?: string
  channelId?: string
  page: number
}

export const generalLedgerQuery = (s: LedgerSearch) =>
  queryOptions({
    queryKey: ["accounting", "general-ledger", s],
    enabled: !!s.accountId,
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/general-ledger", {
          params: {
            query: {
              accountId: s.accountId ?? "",
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
              divisionId: s.divisionId,
              channelId: s.channelId,
              page: s.page,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })
