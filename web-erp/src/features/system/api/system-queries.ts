import { queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"

export const systemKeys = {
  all: ["system"] as const,
  info: () => [...systemKeys.all, "info"] as const,
  readiness: () => [...systemKeys.all, "readiness"] as const,
}

export const systemInfoQuery = queryOptions({
  queryKey: systemKeys.info(),
  queryFn: () => unwrap(api.GET("/api/v1/system/info")),
  staleTime: 60_000,
})

/** /readyz mengembalikan 503 dengan body yang sama saat belum siap; keduanya ditampilkan apa adanya. */
export const readinessQuery = queryOptions({
  queryKey: systemKeys.readiness(),
  queryFn: async () => {
    const { data, error } = await api.GET("/readyz")
    return data ?? error
  },
  meta: { skipGlobalError: true },
  refetchInterval: 30_000,
})
