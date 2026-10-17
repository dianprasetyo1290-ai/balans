import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { RequestDetailPage } from "@/features/procurement/components/request-detail-page"

export const Route = createFileRoute(
  "/_app/procurement/purchase-requests/$id/"
)({
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
    <RequestDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
