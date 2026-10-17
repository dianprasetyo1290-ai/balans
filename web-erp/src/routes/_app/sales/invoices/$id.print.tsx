import { createFileRoute } from "@tanstack/react-router"

import { InvoicePrintPage } from "@/features/sales/components/billing-print-page"

export const Route = createFileRoute("/_app/sales/invoices/$id/print")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <InvoicePrintPage key={id} id={id} />
}
