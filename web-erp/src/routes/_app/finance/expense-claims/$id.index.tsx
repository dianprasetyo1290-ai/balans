import { createFileRoute } from "@tanstack/react-router"

import { ExpenseClaimDetailPage } from "@/features/accounting/components/expense-claim-pages"

export const Route = createFileRoute("/_app/finance/expense-claims/$id/")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ExpenseClaimDetailPage key={id} id={id} />
}
