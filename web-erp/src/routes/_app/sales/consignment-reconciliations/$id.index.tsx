import { createFileRoute } from "@tanstack/react-router"

import { ReconciliationDetailPage } from "@/features/sales/components/consignment-reconciliation-pages"
import { detailSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute(
  "/_app/sales/consignment-reconciliations/$id/"
)({
  validateSearch: detailSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ReconciliationDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
