import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ExpenseTypesPage } from "@/features/accounting/components/expense-types-page"

export const Route = createFileRoute("/_app/master/expense-types")({
  validateSearch: z.object({
    q: z.string().optional(),
    includeInactive: z.boolean().catch(false),
  }),
  component: ExpenseTypesPage,
})
