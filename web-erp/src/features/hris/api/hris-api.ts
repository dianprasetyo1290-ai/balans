import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api, unwrap } from "@/lib/api/client"
import type { components } from "@/lib/api/schema.gen"

export type Employee = components["schemas"]["Employee"]
export type EmployeeInput = components["schemas"]["EmployeeInput"]
export type EmployeeOption = components["schemas"]["EmployeeOption"]
export type EmploymentStatus = components["schemas"]["EmploymentStatus"]

export type EmployeeSearch = {
  page: number
  sort?: string
  q?: string
  /** NIK dicari persis lewat hash di server (Technical Guideline 6.7). */
  nik?: string
  status?: EmploymentStatus
}

const idem = (key: string) => ({ header: { "Idempotency-Key": key } })

export const employeesQuery = (s: EmployeeSearch) =>
  queryOptions({
    queryKey: ["hris", "employees", s],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/employees", {
          params: {
            query: {
              page: s.page,
              sort: s.sort,
              q: s.q || undefined,
              nik: s.nik || undefined,
              status: s.status,
            },
          },
        })
      ),
    placeholderData: keepPreviousData,
  })

export const employeeOptionsQuery = (q: string) =>
  queryOptions({
    queryKey: ["hris", "employee-options", q],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/employee-options", {
          params: { query: { q: q || undefined } },
        })
      ),
    staleTime: 30_000,
    placeholderData: keepPreviousData,
  })

export function createEmployee(body: EmployeeInput, key: string) {
  return unwrap(api.POST("/api/v1/employees", { params: idem(key), body }))
}

export function updateEmployee(
  id: string,
  body: EmployeeInput & { version: number },
  key: string
) {
  return unwrap(
    api.PUT("/api/v1/employees/{id}", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}

export function resignEmployee(
  id: string,
  body: { resignDate: string; reason: string; version: number },
  key: string
) {
  return unwrap(
    api.POST("/api/v1/employees/{id}/resign", {
      params: { path: { id }, ...idem(key) },
      body,
    })
  )
}
