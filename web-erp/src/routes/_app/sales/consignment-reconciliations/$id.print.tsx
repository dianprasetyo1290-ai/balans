import { createFileRoute } from "@tanstack/react-router"

import { ReconciliationPrintPage } from "@/features/sales/components/consignment-reconciliation-pages"

export const Route = createFileRoute(
  "/_app/sales/consignment-reconciliations/$id/print"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ReconciliationPrintPage key={id} id={id} />
}
