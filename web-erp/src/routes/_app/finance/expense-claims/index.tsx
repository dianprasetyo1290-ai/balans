import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ExpenseClaimListPage } from "@/features/accounting/components/expense-claim-pages"

export const Route = createFileRoute("/_app/finance/expense-claims/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "pending_approval",
        "approved",
        "rejected",
        "awaiting_payment",
        "paid",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
  }),
  component: ExpenseClaimListPage,
})
