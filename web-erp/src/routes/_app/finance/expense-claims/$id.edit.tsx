import { createFileRoute } from "@tanstack/react-router"

import { ExpenseClaimFormPage } from "@/features/accounting/components/expense-claim-pages"

export const Route = createFileRoute("/_app/finance/expense-claims/$id/edit")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ExpenseClaimFormPage key={id} id={id} />
}
