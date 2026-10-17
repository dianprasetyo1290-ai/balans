import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { OrderDetailPage } from "@/features/procurement/components/order-detail-page"

export const Route = createFileRoute("/_app/procurement/purchase-orders/$id/")({
  validateSearch: z.object({
    tab: z.enum(["summary", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <OrderDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
