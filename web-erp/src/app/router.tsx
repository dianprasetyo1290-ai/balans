import type { QueryClient } from "@tanstack/react-query"
import { createRouter } from "@tanstack/react-router"

import { CircularProgress } from "@/components/common/circular-progress"
import { routeTree } from "@/routeTree.gen"

/** Indikator pindah halaman mengikuti aturan tunda 300 ms dan minimal 500 ms (Technical Guideline 10.7). */
export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree,
    context: { queryClient },
    defaultPreload: "intent",
    defaultPreloadStaleTime: 0,
    defaultPendingMs: 300,
    defaultPendingMinMs: 500,
    defaultPendingComponent: () => (
      <div className="flex min-h-40 items-center justify-center">
        <CircularProgress label="Memuat halaman" />
      </div>
    ),
  })
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>
  }
}
