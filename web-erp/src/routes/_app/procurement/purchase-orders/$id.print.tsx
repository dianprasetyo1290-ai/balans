import { createFileRoute } from "@tanstack/react-router"

import { OrderPrintPage } from "@/features/procurement/components/order-print-page"

export const Route = createFileRoute(
  "/_app/procurement/purchase-orders/$id/print"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <OrderPrintPage key={id} id={id} />
}
