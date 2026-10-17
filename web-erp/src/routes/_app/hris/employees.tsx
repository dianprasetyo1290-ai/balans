import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { EmployeesPage } from "@/features/hris/components/employees-page"

export const Route = createFileRoute("/_app/hris/employees")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    status: z.enum(["active", "resigned"]).optional(),
  }),
  component: EmployeesPage,
})
