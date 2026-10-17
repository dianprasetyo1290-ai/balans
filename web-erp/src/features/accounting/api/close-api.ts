import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

// API R1.4: Aset Tetap, Penyusutan, Tutup Buku, Neraca Saldo, Saldo Awal (S-408, S-410 – S-413).

type S = components["schemas"]
export type FixedAssetClass = S["FixedAssetClass"]
export type FixedAssetClassCreate = S["FixedAssetClassCreate"]
export type FixedAssetClassUpdate = S["FixedAssetClassUpdate"]
export type FixedAsset = S["FixedAsset"]
export type FixedAssetDetail = S["FixedAssetDetail"]
export type FixedAssetCreate = S["FixedAssetCreate"]
export type FixedAssetUpdate = S["FixedAssetUpdate"]
export type FixedAssetStatus = S["FixedAssetStatus"]
export type FixedAssetSource = S["FixedAssetSource"]
export type FixedAssetDisposeInput = S["FixedAssetDisposeInput"]
export type DepreciationRun = S["DepreciationRun"]
export type DepreciationRunDetail = S["DepreciationRunDetail"]
export type DepreciationLine = S["DepreciationLine"]
export type CloseChecklist = S["CloseChecklist"]
export type CloseChecklistStep = S["CloseChecklistStep"]
export type CloseStepCode = S["CloseStepCode"]
export type CloseStepStatus = S["CloseStepStatus"]
export type CloseChecklistStepInput = S["CloseChecklistStepInput"]
export type TrialBalance = S["TrialBalance"]
export type TrialBalanceRow = S["TrialBalanceRow"]
export type OpeningBalance = S["OpeningBalance"]
export type OpeningBalanceLine = S["OpeningBalanceLine"]
export type OpeningBalanceLineInput = S["OpeningBalanceLineInput"]
export type OpeningItem = S["OpeningItem"]
export type OpeningItemType = S["OpeningItemType"]
export type CutoverReconciliation = S["CutoverReconciliation"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const ASSET_SOURCE_LABELS: Record<FixedAssetSource, string> = {
  purchase: "Pembelian",
  opening: "Saldo awal migrasi",
}

export const ASSET_STATUS_LABELS: Record<FixedAssetStatus, string> = {
  draft: "Draft",
  active: "Aktif",
  disposed: "Dilepas",
}

export const CLOSE_STEP_STATUS_LABELS: Record<CloseStepStatus, string> = {
  pending: "Belum",
  done: "Selesai",
  skipped: "Tidak Berlaku",
}

export const OPENING_ITEM_TYPE_LABELS: Record<OpeningItemType, string> = {
  receivable: "Piutang",
  payable: "Utang",
}

// --- Kelas aset & aset tetap -------------------------------------------------------------------------

export const assetClassesQuery = (includeInactive = false) =>
  queryOptions({
    queryKey: ["accounting", "asset-classes", includeInactive],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/fixed-asset-classes", {
          params: { query: { includeInactive } },
        })
      ),
    staleTime: 60_000,
  })

export function createAssetClass(body: FixedAssetClassCreate, key: string) {
  return unwrap(
    api.POST("/api/v1/fixed-asset-classes", { params: idem(key), body })
  )
}

export function updateAssetClass(
  id: string,
  body: FixedAssetClassUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/fixed-asset-classes/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export type FixedAssetSearch = {
  page: number
  q?: string
  status?: FixedAssetStatus
  classId?: string
}

export const fixedAssetsQuery = (s: FixedAssetSearch) =>
  queryOptions({
    queryKey: ["accounting", "fixed-assets", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/fixed-assets", {
          params: {
            query: {
              page: s.page,
              q: s.q || undefined,
              status: s.status,
              classId: s.classId,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const fixedAssetQuery = (id: string) =>
  queryOptions({
    queryKey: ["accounting", "fixed-asset", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/fixed-assets/{id}", { params: { path: { id } } })
      ),
  })

export function createFixedAsset(body: FixedAssetCreate, key: string) {
  return unwrap(api.POST("/api/v1/fixed-assets", { params: idem(key), body }))
}

export function updateFixedAsset(
  id: string,
  body: FixedAssetUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/fixed-assets/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteFixedAsset(id: string, version: number, key: string) {
  return unwrap(
    api.DELETE("/api/v1/fixed-assets/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

export function activateFixedAsset(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/fixed-assets/{id}/activate", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function disposeFixedAsset(
  id: string,
  body: FixedAssetDisposeInput,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/fixed-assets/{id}/dispose", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

// --- Penyusutan --------------------------------------------------------------------------------------

export const depreciationRunsQuery = (page: number) =>
  queryOptions({
    queryKey: ["accounting", "depreciation-runs", page],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/depreciation-runs", { params: { query: { page } } })
      ),
    placeholderData: keepPreviousData,
  })

export const depreciationRunQuery = (id: string) =>
  queryOptions({
    queryKey: ["accounting", "depreciation-run", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/depreciation-runs/{id}", { params: { path: { id } } })
      ),
  })

export function runDepreciation(year: number, month: number, key: string) {
  return unwrap(
    api.POST("/api/v1/depreciation-runs", {
      params: idem(key),
      body: { year, month },
    })
  )
}

// --- Tutup buku ---------------------------------------------------------------------------------------

export const closeChecklistQuery = (periodId: string) =>
  queryOptions({
    queryKey: ["accounting", "close-checklist", periodId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/fiscal-periods/{id}/close-checklist", {
          params: { path: { id: periodId } },
        })
      ),
  })

export function updateChecklistStep(
  periodId: string,
  stepCode: CloseStepCode,
  body: CloseChecklistStepInput,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/fiscal-periods/{id}/close-checklist/{stepCode}", {
      params: { path: { id: periodId, stepCode }, ...idem(key) },
      body,
    })
  )
}

export function closeFiscalPeriod(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/fiscal-periods/{id}/close", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function requestReopen(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/fiscal-periods/{id}/reopen", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}

// --- Neraca Saldo -------------------------------------------------------------------------------------

export type TrialBalanceSearch = {
  dateFrom: string
  dateTo: string
  divisionId?: string
  channelId?: string
}

export const trialBalanceQuery = (s: TrialBalanceSearch) =>
  queryOptions({
    queryKey: ["accounting", "trial-balance", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/trial-balance", {
          params: {
            query: {
              dateFrom: s.dateFrom,
              dateTo: s.dateTo,
              divisionId: s.divisionId,
              channelId: s.channelId,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

// --- Saldo awal ---------------------------------------------------------------------------------------

export const openingBalanceQuery = queryOptions({
  queryKey: ["accounting", "opening-balance"],
  queryFn: () => unwrap(api.GET("/api/v1/opening-balance")),
})

export function saveCutoverDate(
  cutoverDate: string,
  version: number,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/opening-balance", {
      params: idem(key),
      body: { cutoverDate, version },
    })
  )
}

export function saveOpeningLine(body: OpeningBalanceLineInput, key: string) {
  return unwrap(
    api.POST("/api/v1/opening-balance/lines", { params: idem(key), body })
  )
}

export function deleteOpeningLine(id: string, key: string) {
  return unwrap(
    api.DELETE("/api/v1/opening-balance/lines/{id}", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function postOpeningBalance(version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/opening-balance/post", {
      params: idem(key),
      body: { version },
    })
  )
}

export const openingItemsQuery = (
  page: number,
  itemType?: OpeningItemType,
  q?: string
) =>
  queryOptions({
    queryKey: ["accounting", "opening-items", page, itemType, q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/opening-items", {
          params: { query: { page, itemType, q: q || undefined } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const cutoverReconciliationQuery = queryOptions({
  queryKey: ["accounting", "cutover-reconciliation"],
  queryFn: () => unwrap(api.GET("/api/v1/cutover-reconciliation")),
})

// --- Utilitas -------------------------------------------------------------------------------------------

const pad = (n: number) => String(n).padStart(2, "0")

/** Rentang tanggal satu periode bulanan: [tanggal 1, tanggal terakhir]. */
export function periodRange(year: number, month: number): [string, string] {
  const last = new Date(Date.UTC(year, month, 0)).getUTCDate()
  return [`${year}-${pad(month)}-01`, `${year}-${pad(month)}-${pad(last)}`]
}

/** Periode bulan sebelumnya dari tanggal bisnis YYYY-MM-DD (default penyusutan). */
export function previousPeriod(today: string): { year: number; month: number } {
  const year = Number(today.slice(0, 4))
  const month = Number(today.slice(5, 7))
  return month === 1
    ? { year: year - 1, month: 12 }
    : { year, month: month - 1 }
}

/** Pilihan periode untuk dropdown: 24 bulan terakhir s.d. bulan depan, terbaru dulu. */
export function periodOptions(
  today: string,
  back = 24
): { year: number; month: number }[] {
  let year = Number(today.slice(0, 4))
  let month = Number(today.slice(5, 7)) + 1
  if (month === 13) {
    month = 1
    year++
  }
  const out: { year: number; month: number }[] = []
  for (let i = 0; i <= back; i++) {
    out.push({ year, month })
    month--
    if (month === 0) {
      month = 12
      year--
    }
  }
  return out
}

const MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "Mei",
  "Jun",
  "Jul",
  "Agu",
  "Sep",
  "Okt",
  "Nov",
  "Des",
]

/** Label periode, mis. `Okt 2026` (sama dengan label periode backend). */
export const periodLabel = (year: number, month: number) =>
  `${MONTHS[month - 1]} ${year}`
