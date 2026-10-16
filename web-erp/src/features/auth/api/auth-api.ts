import { queryOptions, useQuery } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"
import { useSessionStore } from "@/stores/session-store"

export type CurrentUser = components["schemas"]["CurrentUser"]

export const meQuery = queryOptions({
  queryKey: ["identity", "me"],
  queryFn: () => unwrap(api.GET("/api/v1/me")),
  staleTime: 30_000,
})

/** Login; jika berhasil token disimpan di memori. Error dilempar sebagai ApiError. */
export async function login(
  email: string,
  password: string,
  totpCode?: string
) {
  const token = await unwrap(
    api.POST("/api/v1/auth/login", {
      body: { email, password, ...(totpCode ? { totpCode } : {}) },
    })
  )
  useSessionStore.getState().setToken(token.accessToken, token.expiresAt)
}

/** Logout mencabut sesi di server lalu menghapus token di memori. */
export async function logout() {
  try {
    await api.POST("/api/v1/auth/logout")
  } finally {
    useSessionStore.getState().clear()
  }
}

export async function reauthenticate(body: {
  password?: string
  totpCode?: string
}) {
  const token = await unwrap(api.POST("/api/v1/auth/reauth", { body }))
  useSessionStore.getState().setToken(token.accessToken, token.expiresAt)
}

/** Permission user login. Menyembunyikan tombol bukan pengamanan; API tetap menolak 403. */
export function usePermissions() {
  const { data } = useQuery(meQuery)
  const set = new Set(data?.permissions ?? [])
  return {
    user: data,
    can: (permission: string) => set.has(permission),
    canAny: (...permissions: string[]) => permissions.some((p) => set.has(p)),
  }
}
