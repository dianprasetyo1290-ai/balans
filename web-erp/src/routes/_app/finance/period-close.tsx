import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PeriodClosePage } from "@/features/accounting/components/period-close-page"

export const Route = createFileRoute("/_app/finance/period-close")({
  validateSearch: z.object({
    period: z.string().optional(),
  }),
  component: PeriodClosePage,
})
