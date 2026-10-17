import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type ApprovalRequest = components["schemas"]["ApprovalRequest"]
export type ApprovalStep = components["schemas"]["ApprovalStep"]
export type ApprovalTask = components["schemas"]["ApprovalTask"]
export type ApprovalPolicy = components["schemas"]["ApprovalPolicy"]
export type ApprovalPolicyInput = components["schemas"]["ApprovalPolicyInput"]
export type ApprovalPolicyUpdate = components["schemas"]["ApprovalPolicyUpdate"]
export type ApprovalSubjectType = components["schemas"]["ApprovalSubjectType"]
export type ApprovalDelegation = components["schemas"]["ApprovalDelegation"]
export type ApprovalDelegationInput =
  components["schemas"]["ApprovalDelegationInput"]
export type ApprovalUserOption = components["schemas"]["ApprovalUserOption"]
export type ApprovalStatus = components["schemas"]["ApprovalStatus"]

/** Interval polling inbox & badge: notifikasi in-app tanpa websocket (NFR-11). */
export const APPROVAL_POLL_MS = 60_000

export const approvalInboxQuery = (subjectType?: string) =>
  queryOptions({
    queryKey: ["approval", "inbox", subjectType ?? ""],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approvals/inbox", {
          params: { query: { subjectType: subjectType || undefined } },
        })
      ),
    refetchInterval: APPROVAL_POLL_MS,
  })

export const approvalInboxCountQuery = queryOptions({
  queryKey: ["approval", "inbox-count"],
  queryFn: () => unwrap(api.GET("/api/v1/approvals/inbox/count")),
  refetchInterval: APPROVAL_POLL_MS,
})

export type ApprovalHistorySearch = {
  page: number
  scope: "requested" | "decided" | "all"
  status?: ApprovalStatus
  subjectType?: string
}

export const approvalRequestsQuery = (s: ApprovalHistorySearch) =>
  queryOptions({
    queryKey: ["approval", "requests", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-requests", {
          params: {
            query: {
              page: s.page,
              scope: s.scope,
              status: s.status,
              subjectType: s.subjectType || undefined,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const approvalRequestQuery = (id: string) =>
  queryOptions({
    queryKey: ["approval", "request", id],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-requests/{id}", {
          params: { path: { id } },
        })
      ),
  })

export const subjectTypesQuery = queryOptions({
  queryKey: ["approval", "subject-types"],
  queryFn: () => unwrap(api.GET("/api/v1/approvals/subject-types")),
  staleTime: Infinity,
})

export const approvalPoliciesQuery = (includeEnded: boolean) =>
  queryOptions({
    queryKey: ["approval", "policies", includeEnded],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-policies", {
          params: { query: { includeEnded } },
        })
      ),
  })

export const delegationsQuery = (all: boolean, includePast: boolean) =>
  queryOptions({
    queryKey: ["approval", "delegations", all, includePast],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-delegations", {
          params: { query: { all, includePast } },
        })
      ),
  })

export const userOptionsQuery = (q: string) =>
  queryOptions({
    queryKey: ["approval", "user-options", q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approvals/user-options", {
          params: { query: { q: q || undefined } },
        })
      ),
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export function approveRequest(
  id: string,
  version: number,
  note: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/approval-requests/{id}/approve", {
      params: { path: { id }, ...idem(key) },
      body: { version, note: note || undefined },
    })
  )
}

export function rejectRequest(
  id: string,
  version: number,
  note: string,
  key: string
) {
  return unwrap(
    api.POST("/api/v1/approval-requests/{id}/reject", {
      params: { path: { id }, ...idem(key) },
      body: { version, note },
    })
  )
}

export function createPolicy(body: ApprovalPolicyInput, key: string) {
  return unwrap(
    api.POST("/api/v1/approval-policies", { params: idem(key), body })
  )
}

export function updatePolicy(
  id: string,
  body: ApprovalPolicyUpdate,
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/approval-policies/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function endPolicy(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/approval-policies/{id}/end", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}

export function createDelegation(body: ApprovalDelegationInput, key: string) {
  return unwrap(
    api.POST("/api/v1/approval-delegations", { params: idem(key), body })
  )
}

export function cancelDelegation(id: string, version: number, key: string) {
  return unwrap(
    api.POST("/api/v1/approval-delegations/{id}/cancel", {
      params: { path: { id }, ...idem(key) },
      body: { version },
    })
  )
}
