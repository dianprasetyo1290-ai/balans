import { createFileRoute } from "@tanstack/react-router"

import { ConsignmentDeliveryDetailPage } from "@/features/sales/components/consignment-delivery-pages"
import { detailSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/consignments/$id")({
  validateSearch: detailSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ConsignmentDeliveryDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
