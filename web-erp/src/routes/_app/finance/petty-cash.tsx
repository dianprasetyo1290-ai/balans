import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PettyCashPage } from "@/features/accounting/components/petty-cash-page"

export const Route = createFileRoute("/_app/finance/petty-cash")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    fundId: z.string().optional().catch(undefined),
  }),
  component: PettyCashPage,
})
