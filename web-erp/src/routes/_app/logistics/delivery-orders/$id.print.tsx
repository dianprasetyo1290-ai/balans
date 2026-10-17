import { createFileRoute } from "@tanstack/react-router"

import { DeliveryOrderPrintPage } from "@/features/logistics/components/print-pages"

export const Route = createFileRoute(
  "/_app/logistics/delivery-orders/$id/print"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <DeliveryOrderPrintPage key={id} id={id} />
}
