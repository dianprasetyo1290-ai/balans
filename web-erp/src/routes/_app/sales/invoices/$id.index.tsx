import { createFileRoute } from "@tanstack/react-router"

import { InvoiceDetailPage } from "@/features/sales/components/billing-pages"

export const Route = createFileRoute("/_app/sales/invoices/$id/")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <InvoiceDetailPage key={id} id={id} />
}
