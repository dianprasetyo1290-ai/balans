import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { TaxesPage } from "@/features/accounting/components/taxes-page"

export const Route = createFileRoute("/_app/finance/taxes")({
  validateSearch: z.object({
    tab: z.enum(["slips", "deposits", "final", "rates"]).catch("slips"),
    year: z.number().int().min(2000).max(9999).optional().catch(undefined),
    month: z.number().int().min(1).max(12).optional().catch(undefined),
  }),
  component: TaxesPage,
})
