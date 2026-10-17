import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ChartOfAccountsPage } from "@/features/accounting/components/chart-of-accounts-page"

export const Route = createFileRoute("/_app/finance/accounts")({
  validateSearch: z.object({
    q: z.string().optional(),
    includeInactive: z.boolean().catch(false),
  }),
  component: ChartOfAccountsPage,
})
