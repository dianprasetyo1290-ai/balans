import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { CashBankPage } from "@/features/accounting/components/cash-bank-page"

export const Route = createFileRoute("/_app/finance/cash-bank")({
  validateSearch: z.object({
    tab: z.enum(["accounts", "payments", "mutations"]).catch("accounts"),
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    direction: z.enum(["incoming", "outgoing"]).optional().catch(undefined),
    accountId: z.string().optional().catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: CashBankPage,
})
