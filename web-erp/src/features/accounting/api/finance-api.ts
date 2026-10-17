import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

type S = components["schemas"]

export type CashBankAccount = S["CashBankAccount"]
export type CashBankAccountInput = S["CashBankAccountInput"]
export type CashBankAccountUpdate = S["CashBankAccountUpdate"]
export type BankMutationList = S["BankMutationList"]
export type CashPayment = S["CashPayment"]
export type CashPaymentInput = S["CashPaymentInput"]
export type CashPaymentStatus = S["CashPaymentStatus"]
export type PaymentDirection = S["PaymentDirection"]
export type BankStatement = S["BankStatement"]
export type BankStatementDetail = S["BankStatementDetail"]
export type BankStatementLine = S["BankStatementLine"]
export type BankMatchCandidate = S["BankMatchCandidate"]
export type ExpenseClaimSummary = S["ExpenseClaimSummary"]
export type ExpenseClaimDetail = S["ExpenseClaimDetail"]
export type ExpenseClaimInput = S["ExpenseClaimInput"]
export type ExpenseClaimStatus = S["ExpenseClaimStatus"]
export type ExpenseClaimOptions = S["ExpenseClaimOptions"]
export type ExpenseClaimTaxChoice = S["ExpenseClaimTaxChoice"]
export type ExpenseClaimVerifyPreview = S["ExpenseClaimVerifyPreview"]
export type PettyCashFund = S["PettyCashFund"]
export type PettyCashFundList = S["PettyCashFundList"]
export type PettyCashFundInput = S["PettyCashFundInput"]
export type PettyCashTransaction = S["PettyCashTransaction"]
export type PettyCashType = S["PettyCashType"]
export type TaxRate = S["TaxRate"]
export type TaxRateInput = S["TaxRateInput"]
export type TaxType = S["TaxType"]
export type WithholdingType = S["WithholdingType"]
export type WithholdingSlip = S["WithholdingSlip"]
export type WithholdingSummaryRow = S["WithholdingSummaryRow"]
export type TaxDeposit = S["TaxDeposit"]
export type TaxDepositInput = S["TaxDepositInput"]
export type FinalTaxSummary = S["FinalTaxSummary"]
export type FinalTaxSettingInput = S["FinalTaxSettingInput"]
export type FinanceAction = S["FinanceAction"]

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const TAX_TYPE_LABELS: Record<TaxType, string> = {
  pph21: "PPh 21",
  pph23: "PPh 23",
  pph4_2: "PPh 4(2)",
  final_umkm: "PPh Final UMKM",
}

export const WITHHOLDING_LABELS: Record<WithholdingType, string> = {
  none: "Tidak dipotong",
  pph21: "PPh 21",
  pph23: "PPh 23",
  pph4_2: "PPh 4(2)",
}

export const DIRECTION_LABELS: Record<PaymentDirection, string> = {
  incoming: "Masuk",
  outgoing: "Keluar",
}

export const MONTH_LABELS = [
  "Januari",
  "Februari",
  "Maret",
  "April",
  "Mei",
  "Juni",
  "Juli",
  "Agustus",
  "September",
  "Oktober",
  "November",
  "Desember",
]

export function periodLabel(year: number, month: number) {
  return `${MONTH_LABELS[month - 1] ?? month} ${year}`
}

const invalidate = ["finance"] as const

/** Kunci query semua data keuangan R1.4 (di-invalidate setelah mutasi). */
export const financeKey = invalidate

// --- Rekening kas & bank ----------------------------------------------------------------------

export const cashBankAccountsQuery = (
  includeInactive = false,
  deleted = false
) =>
  queryOptions({
    queryKey: ["finance", "bank-accounts", includeInactive, deleted],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/cash-bank-accounts", {
          params: { query: { includeInactive, deleted } },
        })
      ),
    staleTime: 30_000,
  })

export const cashBankAccountQuery = (id: string) =>
  queryOptions({
    queryKey: ["finance", "bank-account", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/cash-bank-accounts/{id}", {
          params: { path: { id } },
        })
      ),
  })

export function createCashBankAccount(body: CashBankAccountInput, key: string) {
  return unwrap(
    api.POST("/api/v1/cash-bank-accounts", { params: idem(key), body })
  )
}

export function updateCashBankAccount(
  id: string,
  body: CashBankAccountUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/cash-bank-accounts/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function deleteCashBankAccount(
  id: string,
  version: number,
  key: string
) {
  return unwrap(
    api.DELETE("/api/v1/cash-bank-accounts/{id}", {
      params: { path: { id }, query: { version }, ...idem(key) },
    })
  )
}

export function restoreCashBankAccount(
  id: string,
  version: number,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/cash-bank-accounts/{id}/restore", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export const bankMutationsQuery = (p: {
  id: string
  dateFrom: string
  dateTo: string
  page: number
}) =>
  queryOptions({
    queryKey: ["finance", "mutations", p],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/cash-bank-accounts/{id}/mutations", {
          params: {
            path: { id: p.id },
            query: {
              dateFrom: p.dateFrom,
              dateTo: p.dateTo,
              page: p.page,
              pageSize: 50,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

// --- Pembayaran -------------------------------------------------------------------------------

export const cashPaymentsQuery = (p: {
  page: number
  q?: string
  bankAccountId?: string
  direction?: PaymentDirection
  status?: CashPaymentStatus
}) =>
  queryOptions({
    queryKey: ["finance", "payments", p],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/cash-payments", {
          params: { query: { ...p, q: p.q || undefined, pageSize: 50 } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createCashPayment(body: CashPaymentInput, key: string) {
  return unwrap(api.POST("/api/v1/cash-payments", { params: idem(key), body }))
}

export function updateCashPayment(
  id: string,
  body: CashPaymentInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/cash-payments/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function postCashPayment(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/cash-payments/{id}/post", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function cancelCashPayment(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/cash-payments/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}

// --- Rekonsiliasi bank ------------------------------------------------------------------------

export const bankStatementsQuery = (p: {
  page: number
  bankAccountId?: string
}) =>
  queryOptions({
    queryKey: ["finance", "statements", p],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/bank-statements", {
          params: { query: { ...p, pageSize: 50 } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const bankStatementQuery = (id: string) =>
  queryOptions({
    queryKey: ["finance", "statement", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/bank-statements/{id}", { params: { path: { id } } })
      ),
  })

export async function requestStatementUpload(file: File) {
  const contentType = file.name.toLowerCase().endsWith(".csv")
    ? "text/csv"
    : "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType: "bank_statement",
        fileName: file.name,
        contentType,
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

export function importBankStatement(
  body: { bankAccountId: string; fileId: string },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/bank-statements", { params: idem(key), body })
  )
}

export function autoMatchStatement(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/bank-statements/{id}/auto-match", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function confirmStatementSuggestions(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/bank-statements/{id}/confirm-suggestions", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export const lineCandidatesQuery = (id: string) =>
  queryOptions({
    queryKey: ["finance", "line-candidates", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/bank-statement-lines/{id}/candidates", {
          params: { path: { id } },
        })
      ),
  })

export function confirmStatementLine(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/bank-statement-lines/{id}/confirm", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function matchStatementLine(
  id: string,
  body: S["BankLineMatchInput"],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/bank-statement-lines/{id}/match", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function createStatementLineEntry(
  id: string,
  body: S["BankLineEntryInput"],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/bank-statement-lines/{id}/create-entry", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function unmatchStatementLine(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/bank-statement-lines/{id}/unmatch", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

// --- Pengajuan biaya --------------------------------------------------------------------------

export const expenseClaimOptionsQuery = queryOptions({
  queryKey: ["finance", "expense-claim-options"],
  queryFn: () => unwrap(api.GET("/api/v1/expense-claims/options")),
  staleTime: 5 * 60_000,
})

export const expenseClaimsQuery = (p: {
  page: number
  q?: string
  status?: ExpenseClaimStatus
}) =>
  queryOptions({
    queryKey: ["finance", "expense-claims", p],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/expense-claims", {
          params: { query: { ...p, q: p.q || undefined, pageSize: 50 } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const expenseClaimQuery = (id: string) =>
  queryOptions({
    queryKey: ["finance", "expense-claim", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/expense-claims/{id}", { params: { path: { id } } })
      ),
  })

export async function requestReceiptUpload(
  ownerType: "expense_claim" | "petty_cash_transaction" | "payment",
  file: File
) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: idem(crypto.randomUUID()),
      body: {
        ownerType,
        fileName: file.name,
        contentType: file.type || "application/pdf",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}

export const MAX_RECEIPT_BYTES = 10 * 1024 * 1024
export const RECEIPT_ACCEPT = "application/pdf,image/jpeg,image/png,image/webp"

export function createExpenseClaim(body: ExpenseClaimInput, key: string) {
  return unwrap(api.POST("/api/v1/expense-claims", { params: idem(key), body }))
}

export function updateExpenseClaim(
  id: string,
  body: ExpenseClaimInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/expense-claims/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function claimAction(
  id: string,
  action: "submit" | "revise",
  version: number,
  key: string
) {
  const path =
    action === "submit"
      ? "/api/v1/expense-claims/{id}/submit"
      : "/api/v1/expense-claims/{id}/revise"
  return unwrap(
    api.POST(path, {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function cancelExpenseClaim(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/expense-claims/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}

export function previewClaimVerification(
  id: string,
  body: S["ExpenseClaimVerifyInput"],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/expense-claims/{id}/verify-preview", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function verifyExpenseClaim(
  id: string,
  body: S["ExpenseClaimVerifyInput"],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/expense-claims/{id}/verify", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function payExpenseClaim(
  id: string,
  body: S["ExpenseClaimPayInput"],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/expense-claims/{id}/pay", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

// --- Kas kecil --------------------------------------------------------------------------------

export const pettyCashFundsQuery = (includeInactive = false) =>
  queryOptions({
    queryKey: ["finance", "petty-cash-funds", includeInactive],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/petty-cash-funds", {
          params: { query: { includeInactive } },
        })
      ),
  })

export function createPettyCashFund(body: PettyCashFundInput, key: string) {
  return unwrap(
    api.POST("/api/v1/petty-cash-funds", { params: idem(key), body })
  )
}

export function updatePettyCashFund(
  id: string,
  body: S["PettyCashFundUpdate"],
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/petty-cash-funds/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export const pettyCashTransactionsQuery = (p: {
  page: number
  fundId?: string
  type?: PettyCashType
}) =>
  queryOptions({
    queryKey: ["finance", "petty-cash", p],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/petty-cash-transactions", {
          params: { query: { ...p, pageSize: 50 } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export function createPettyCash(
  body: S["PettyCashTransactionInput"],
  key: string
) {
  return unwrap(
    api.POST("/api/v1/petty-cash-transactions", { params: idem(key), body })
  )
}

export function updatePettyCash(
  id: string,
  body: S["PettyCashTransactionUpdate"],
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/petty-cash-transactions/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function pettyCashAction(
  id: string,
  action: "submit" | "revise",
  version: number,
  key: string
) {
  const path =
    action === "submit"
      ? "/api/v1/petty-cash-transactions/{id}/submit"
      : "/api/v1/petty-cash-transactions/{id}/revise"
  return unwrap(
    api.POST(path, {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function cancelPettyCash(
  id: string,
  version: number,
  reason: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/petty-cash-transactions/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version, reason },
    })
  )
}

export function postPettyCash(
  id: string,
  version: number,
  bankAccountId: string | undefined,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/petty-cash-transactions/{id}/post", {
      params: { path: { id }, ...idem(key) },
      body: { version, bankAccountId },
    })
  )
}

// --- Pajak & Bukti Potong ---------------------------------------------------------------------

export const taxRatesQuery = (taxType?: TaxType) =>
  queryOptions({
    queryKey: ["finance", "tax-rates", taxType],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/withholding-tax-rates", {
          params: { query: { taxType } },
        })
      ),
  })

export function createTaxRate(body: TaxRateInput, key: string) {
  return unwrap(
    api.POST("/api/v1/withholding-tax-rates", { params: idem(key), body })
  )
}

export const withholdingSlipsQuery = (p: {
  year: number
  month: number
  taxType?: TaxType
  page: number
}) =>
  queryOptions({
    queryKey: ["finance", "withholdings", p],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/withholding-slips", {
          params: { query: { ...p, pageSize: 100 } },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const withholdingSummaryQuery = (year: number) =>
  queryOptions({
    queryKey: ["finance", "withholding-summary", year],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/withholding-slips/summary", {
          params: { query: { year } },
        })
      ),
  })

export function setSlipNumber(
  id: string,
  version: number,
  slipNumber: string,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/withholding-slips/{id}/slip-number", {
      params: { path: { id }, ...idem(key) },
      body: { version, slipNumber },
    })
  )
}

export function exportWithholdingRecap(
  body: { year: number; month: number; taxType?: TaxType },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/withholding-slips/export", { params: idem(key), body })
  )
}

export const taxDepositsQuery = (year: number) =>
  queryOptions({
    queryKey: ["finance", "tax-deposits", year],
    queryFn: () =>
      unwrap(api.GET("/api/v1/tax-deposits", { params: { query: { year } } })),
  })

export function createTaxDeposit(body: TaxDepositInput, key: string) {
  return unwrap(api.POST("/api/v1/tax-deposits", { params: idem(key), body }))
}

export const finalTaxQuery = (year: number) =>
  queryOptions({
    queryKey: ["finance", "final-tax", year],
    queryFn: () =>
      unwrap(api.GET("/api/v1/final-tax", { params: { query: { year } } })),
  })

export function createFinalTaxSetting(body: FinalTaxSettingInput, key: string) {
  return unwrap(
    api.POST("/api/v1/final-tax/settings", { params: idem(key), body })
  )
}

export function runFinalTax(year: number, month: number, key: string) {
  return unwrap(
    api.POST("/api/v1/final-tax/runs", {
      params: idem(key),
      body: { year, month },
    })
  )
}
