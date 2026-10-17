import { createFileRoute } from "@tanstack/react-router"

import { ProformaPrintPage } from "@/features/sales/components/billing-print-page"

export const Route = createFileRoute("/_app/sales/proforma-invoices/$id/print")(
  {
    component: RouteComponent,
  }
)

function RouteComponent() {
  const { id } = Route.useParams()
  return <ProformaPrintPage key={id} id={id} />
}
