import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type User = components["schemas"]["User"]
export type UserInput = components["schemas"]["UserInput"]
export type UserUpdate = components["schemas"]["UserUpdate"]
export type Role = components["schemas"]["Role"]
export type Permission = components["schemas"]["Permission"]
export type AuditLog = components["schemas"]["AuditLog"]

export type UserListSearch = {
  page: number
  sort?: string
  q?: string
  isActive?: boolean
}

export const usersQuery = (s: UserListSearch) =>
  queryOptions({
    queryKey: ["identity", "users", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/users", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              isActive: s.isActive,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const rolesQuery = queryOptions({
  queryKey: ["identity", "roles"],
  queryFn: () => unwrap(api.GET("/api/v1/roles")),
})

export const permissionsQuery = queryOptions({
  queryKey: ["identity", "permissions"],
  queryFn: () => unwrap(api.GET("/api/v1/permissions")),
  staleTime: 5 * 60_000,
})

export type AuditSearch = {
  page: number
  entityType?: string
  entityId?: string
  action?: AuditLog["action"]
}

export const auditLogsQuery = (s: AuditSearch) =>
  queryOptions({
    queryKey: ["audit", "logs", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/audit-logs", {
          params: {
            query: {
              page: s.page,
              entityType: s.entityType || undefined,
              entityId: s.entityId || undefined,
              action: s.action,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export function createUser(body: UserInput, key: string) {
  return unwrap(api.POST("/api/v1/users", { params: idem(key), body }))
}

export function updateUser(id: string, body: UserUpdate, key: string) {
  return unwrap(
    api.PUT("/api/v1/users/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function setUserActive(
  id: string,
  active: boolean,
  version: number,
  key: string
) {
  const path = active
    ? "/api/v1/users/{id}/activate"
    : "/api/v1/users/{id}/deactivate"
  return unwrap(
    api.POST(path, {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function resendInvitation(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/users/{id}/resend-invitation", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function resetUserMfa(id: string, key: string) {
  return unwrap(
    api.POST("/api/v1/users/{id}/reset-mfa", {
      params: { path: { id }, ...idem(key) },
    })
  )
}

export function setRolePermissions(
  id: string,
  permissions: string[],
  version: number,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/roles/{id}/permissions", {
      params: { path: { id }, ...idem(key) },
      body: { permissions, version },
    })
  )
}
