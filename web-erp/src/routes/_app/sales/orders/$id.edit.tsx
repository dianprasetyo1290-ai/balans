import { createFileRoute } from "@tanstack/react-router"

import { OrderEditPage } from "@/features/sales/components/order-form-page"

export const Route = createFileRoute("/_app/sales/orders/$id/edit")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <OrderEditPage key={id} id={id} />
}
