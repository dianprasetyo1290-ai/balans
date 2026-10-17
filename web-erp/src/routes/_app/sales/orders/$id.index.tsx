import { createFileRoute } from "@tanstack/react-router"

import { OrderDetailPage } from "@/features/sales/components/order-detail-page"
import { detailSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/orders/$id/")({
  validateSearch: detailSearchSchema,
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
