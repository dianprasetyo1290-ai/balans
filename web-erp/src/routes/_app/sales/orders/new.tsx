import { createFileRoute } from "@tanstack/react-router"

import { OrderCreatePage } from "@/features/sales/components/order-form-page"
import { orderTypeSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/orders/new")({
  validateSearch: orderTypeSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const { orderType } = Route.useSearch()
  return <OrderCreatePage key={orderType} orderType={orderType} />
}
