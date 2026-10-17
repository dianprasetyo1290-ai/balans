import { createFileRoute } from "@tanstack/react-router"

import { ProformaDetailPage } from "@/features/sales/components/billing-pages"

export const Route = createFileRoute("/_app/sales/proforma-invoices/$id/")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ProformaDetailPage key={id} id={id} />
}
