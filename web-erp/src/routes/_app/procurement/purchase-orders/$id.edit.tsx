import { createFileRoute } from "@tanstack/react-router"

import { OrderEditPage } from "@/features/procurement/components/order-form-page"

export const Route = createFileRoute(
  "/_app/procurement/purchase-orders/$id/edit"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <OrderEditPage key={id} id={id} />
}
