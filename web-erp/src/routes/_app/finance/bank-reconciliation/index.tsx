import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { BankReconciliationListPage } from "@/features/accounting/components/bank-reconciliation-pages"

export const Route = createFileRoute("/_app/finance/bank-reconciliation/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    bankAccountId: z.string().optional().catch(undefined),
  }),
  component: BankReconciliationListPage,
})
