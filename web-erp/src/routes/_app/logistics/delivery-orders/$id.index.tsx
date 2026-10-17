import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { DeliveryOrderDetailPage } from "@/features/logistics/components/delivery-order-pages"

export const Route = createFileRoute("/_app/logistics/delivery-orders/$id/")({
  validateSearch: z.object({
    tab: z.enum(["lines", "shipment", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <DeliveryOrderDetailPage
      key={id}
      id={id}
      tab={tab ?? "lines"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
