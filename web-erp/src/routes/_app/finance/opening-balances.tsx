import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { OpeningBalancePage } from "@/features/accounting/components/opening-balance-page"

export const Route = createFileRoute("/_app/finance/opening-balances")({
  validateSearch: z.object({
    tab: z.enum(["balances", "items", "reconciliation"]).catch("balances"),
    page: z.number().int().min(1).catch(1),
    itemType: z.enum(["receivable", "payable"]).optional().catch(undefined),
  }),
  component: OpeningBalancePage,
})
