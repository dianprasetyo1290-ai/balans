import { createFileRoute } from "@tanstack/react-router"

import { BankStatementDetailPage } from "@/features/accounting/components/bank-reconciliation-pages"

export const Route = createFileRoute("/_app/finance/bank-reconciliation/$id")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <BankStatementDetailPage key={id} id={id} />
}
