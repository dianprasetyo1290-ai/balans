import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { OrderCreatePage } from "@/features/procurement/components/order-form-page"

export const Route = createFileRoute("/_app/procurement/purchase-orders/new")({
  validateSearch: z.object({
    orderType: z
      .enum(["goods", "packaging", "toll_manufacturing", "reprocess", "other"])
      .catch("packaging"),
    fromRequest: z.string().uuid().optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { orderType, fromRequest } = Route.useSearch()
  return (
    <OrderCreatePage
      key={`${orderType}-${fromRequest ?? ""}`}
      orderType={orderType}
      fromRequestId={fromRequest}
    />
  )
}
