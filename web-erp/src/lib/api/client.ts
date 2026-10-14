import createClient from "openapi-fetch"

import { ApiError, toProblem } from "./problem"
import type { paths } from "./schema.gen"

/** Client API bertipe dari schema.gen.ts. Tipe response DILARANG ditulis manual (Technical Guideline 10.4). */
export const api = createClient<paths>({ baseUrl: window.location.origin })

type FetchResult<T> = { data?: T; error?: unknown; response: Response }

/** Mengambil `data` atau melempar ApiError berisi problem+json. */
export async function unwrap<T>(request: Promise<FetchResult<T>>): Promise<T> {
  const { data, error, response } = await request
  if (!response.ok) {
    throw new ApiError(toProblem(error, response.status))
  }
  return data as T
}
