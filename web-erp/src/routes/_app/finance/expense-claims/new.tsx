import { createFileRoute } from "@tanstack/react-router"

import { ExpenseClaimFormPage } from "@/features/accounting/components/expense-claim-pages"

export const Route = createFileRoute("/_app/finance/expense-claims/new")({
  component: () => <ExpenseClaimFormPage />,
})
