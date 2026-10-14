import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query"

import { isTransientProblem, parseProblem } from "@/lib/api/problem"
import { showErrorDialog } from "@/stores/error-dialog-store"

const MAX_QUERY_RETRIES = 2

declare module "@tanstack/react-query" {
  interface Register {
    queryMeta: { skipGlobalError?: boolean }
    mutationMeta: { skipGlobalError?: boolean }
  }
}

/**
 * Query di-retry maks. 2 kali hanya untuk error jaringan dan 5xx; mutation tidak di-retry otomatis.
 * Error yang tidak ditangani halaman ditampilkan lewat ErrorDialog (Technical Guideline 10.4, 10.6).
 */
export function createQueryClient() {
  const queryClient: QueryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: (failureCount, error) => failureCount < MAX_QUERY_RETRIES && isTransientProblem(parseProblem(error)),
      },
      mutations: { retry: false },
    },
    queryCache: new QueryCache({
      onError: (error, query) => {
        if (query.meta?.skipGlobalError || query.state.data !== undefined) {
          return
        }
        showErrorDialog(parseProblem(error), () => void queryClient.refetchQueries({ queryKey: query.queryKey }))
      },
    }),
    mutationCache: new MutationCache({
      onError: (error, _variables, _context, mutation) => {
        if (mutation.meta?.skipGlobalError || mutation.options.onError) {
          return
        }
        showErrorDialog(parseProblem(error))
      },
    }),
  })
  return queryClient
}
