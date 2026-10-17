import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type DocumentSequence = components["schemas"]["DocumentSequence"]

export const documentSequencesQuery = (year?: number) =>
  queryOptions({
    queryKey: ["document", "sequences", year ?? 0],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/document-sequences", { params: { query: { year } } })
      ),
    placeholderData: keepPreviousData,
  })
