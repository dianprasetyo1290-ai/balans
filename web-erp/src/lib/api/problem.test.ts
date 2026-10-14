import { describe, expect, it } from "vitest"

import { ERROR_MESSAGES, errorMessageFor, isErrorCode } from "./error-messages"
import { ApiError, isTransientProblem, parseProblem, toProblem } from "./problem"
import type { components } from "./schema.gen"

type ErrorCodeFromSpec = components["schemas"]["ErrorCode"]

describe("katalog error", () => {
  it("memuat semua kode dari kontrak OpenAPI", () => {
    const specCodes: Record<ErrorCodeFromSpec, true> = {
      VALIDATION_FAILED: true, AUTH_INVALID_CREDENTIALS: true, AUTH_ACCOUNT_LOCKED: true, AUTH_MFA_REQUIRED: true,
      AUTH_MFA_INVALID: true, AUTH_TOKEN_EXPIRED: true, AUTH_SESSION_EXPIRED: true, AUTH_REAUTH_REQUIRED: true,
      FORBIDDEN: true, NOT_FOUND: true, VERSION_CONFLICT: true, INVALID_STATE_TRANSITION: true,
      IDEMPOTENCY_KEY_REQUIRED: true, IDEMPOTENCY_IN_PROGRESS: true, IDEMPOTENCY_KEY_REUSED: true, DUPLICATE_CODE: true,
      PERIOD_LOCKED: true, STOCK_INSUFFICIENT: true, BATCH_SHELF_LIFE_TOO_SHORT: true, APPROVAL_REQUIRED: true,
      DISCOUNT_LIMIT_EXCEEDED: true, CREDIT_LIMIT_EXCEEDED: true, THREE_WAY_MATCH_FAILED: true, DEPOSIT_INSUFFICIENT: true,
      OUTSIDE_ATTENDANCE_RADIUS: true, FILE_TOO_LARGE: true, FILE_TYPE_NOT_ALLOWED: true, IMPORT_ALREADY_PROCESSED: true,
      RATE_LIMITED: true, PROVIDER_UNAVAILABLE: true, BAD_REQUEST: true, INTERNAL_ERROR: true,
    }
    for (const code of Object.keys(specCodes)) {
      expect(isErrorCode(code), code).toBe(true)
    }
    expect(isErrorCode("NETWORK_ERROR")).toBe(true)
  })

  it("login gagal tampil sebagai modal Gagal masuk", () => {
    expect(ERROR_MESSAGES.AUTH_INVALID_CREDENTIALS).toMatchObject({
      title: "Gagal masuk",
      message: "Email atau password salah.",
      action: "close",
      display: "modal",
    })
  })

  it("konflik versi menawarkan Muat ulang, error jaringan menawarkan Coba lagi", () => {
    expect(ERROR_MESSAGES.VERSION_CONFLICT.action).toBe("reload")
    expect(ERROR_MESSAGES.NETWORK_ERROR.action).toBe("retry")
    expect(ERROR_MESSAGES.VALIDATION_FAILED.display).toBe("inline")
  })

  it("kode tak dikenal diperlakukan sebagai INTERNAL_ERROR", () => {
    expect(errorMessageFor("SESUATU")).toBe(ERROR_MESSAGES.INTERNAL_ERROR)
  })
})

describe("parseProblem", () => {
  it("membaca body problem+json", () => {
    const problem = toProblem(
      { type: "x", title: "Data tidak valid", status: 422, code: "VALIDATION_FAILED", detail: "Periksa kembali.", fields: { supplierId: "Wajib diisi" }, requestId: "r1" },
      422,
    )
    expect(problem).toMatchObject({ code: "VALIDATION_FAILED", fields: { supplierId: "Wajib diisi" }, requestId: "r1" })
    expect(parseProblem(new ApiError(problem))).toBe(problem)
  })

  it("memakai teks default bila detail kosong", () => {
    const problem = toProblem({ type: "x", title: "", status: 409, code: "VERSION_CONFLICT", detail: "" }, 409)
    expect(problem.title).toBe("Data sudah berubah")
    expect(problem.detail).toContain("Muat ulang")
  })

  it("mengubah fetch gagal menjadi NETWORK_ERROR", () => {
    expect(parseProblem(new TypeError("Failed to fetch")).code).toBe("NETWORK_ERROR")
  })

  it("body yang tidak dikenal dipetakan berdasarkan status", () => {
    expect(toProblem("<html>", 502).code).toBe("INTERNAL_ERROR")
    expect(toProblem(undefined, 400).code).toBe("BAD_REQUEST")
    expect(parseProblem(new Error("x")).code).toBe("INTERNAL_ERROR")
  })

  it("hanya jaringan dan 5xx yang boleh di-retry otomatis", () => {
    expect(isTransientProblem(parseProblem(new TypeError("x")))).toBe(true)
    expect(isTransientProblem(toProblem(undefined, 503))).toBe(true)
    expect(isTransientProblem(toProblem({ code: "VERSION_CONFLICT", status: 409, title: "", detail: "", type: "" }, 409))).toBe(false)
  })
})
