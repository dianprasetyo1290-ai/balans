import type { components } from "./schema.gen"
import { errorMessageFor, isErrorCode, type ErrorCode } from "./error-messages"

export type Problem = {
  code: ErrorCode
  title: string
  detail: string
  status: number
  fields?: Record<string, string>
  requestId?: string
}

type ProblemBody = components["schemas"]["Problem"]

/** Error dari API dengan body problem+json (Technical Guideline 8.3). */
export class ApiError extends Error {
  readonly problem: Problem

  constructor(problem: Problem) {
    super(`${problem.code}: ${problem.detail}`)
    this.name = "ApiError"
    this.problem = problem
  }
}

function isProblemBody(value: unknown): value is ProblemBody {
  return (
    typeof value === "object" &&
    value !== null &&
    "code" in value &&
    "status" in value
  )
}

/** Mengubah body error dan status HTTP menjadi Problem. FE hanya bergantung pada `code`. */
export function toProblem(body: unknown, status: number): Problem {
  if (isProblemBody(body) && isErrorCode(body.code)) {
    const fallback = errorMessageFor(body.code)
    return {
      code: body.code,
      title: body.title || fallback.title,
      detail: body.detail || fallback.message,
      status: body.status,
      fields: body.fields,
      requestId: body.requestId,
    }
  }
  const code: ErrorCode =
    status >= 500 || status === 0 ? "INTERNAL_ERROR" : "BAD_REQUEST"
  const fallback = errorMessageFor(code)
  return { code, title: fallback.title, detail: fallback.message, status }
}

/** Mengubah error apa pun (fetch gagal, ApiError, error lain) menjadi Problem. */
export function parseProblem(error: unknown): Problem {
  if (error instanceof ApiError) {
    return error.problem
  }
  if (error instanceof TypeError) {
    const fallback = errorMessageFor("NETWORK_ERROR")
    return {
      code: "NETWORK_ERROR",
      title: fallback.title,
      detail: fallback.message,
      status: 0,
    }
  }
  return toProblem(undefined, 500)
}

/** Error sementara yang boleh di-retry otomatis oleh query: jaringan dan 5xx (Technical Guideline 10.4). */
export function isTransientProblem(problem: Problem): boolean {
  return problem.code === "NETWORK_ERROR" || problem.status >= 500
}
