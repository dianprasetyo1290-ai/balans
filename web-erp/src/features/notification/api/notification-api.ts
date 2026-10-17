import { queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type AppNotification = components["schemas"]["Notification"]

/** Notifikasi in-app diambil berkala (NFR-11); cukup untuk 40 user back-office tanpa websocket. */
export const NOTIFICATION_POLL_MS = 60_000

export const unreadCountQuery = queryOptions({
  queryKey: ["notification", "unread-count"],
  queryFn: () => unwrap(api.GET("/api/v1/notifications/unread-count")),
  refetchInterval: NOTIFICATION_POLL_MS,
})

export const notificationsQuery = queryOptions({
  queryKey: ["notification", "list"],
  queryFn: () =>
    unwrap(
      api.GET("/api/v1/notifications", {
        params: { query: { pageSize: 20 } },
      })
    ),
})

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export function markNotificationRead(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/notifications/{id}/read", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function markAllNotificationsRead(key: string) {
  return unwrap(
    api.POST("/api/v1/notifications/read-all", { params: idem(key) })
  )
}
