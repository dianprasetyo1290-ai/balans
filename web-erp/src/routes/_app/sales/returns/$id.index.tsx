import { createFileRoute } from "@tanstack/react-router"

import { SalesReturnDetailPage } from "@/features/sales/components/sales-return-pages"
import { detailSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/returns/$id/")({
  validateSearch: detailSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SalesReturnDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
