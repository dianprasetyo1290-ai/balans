import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { AdvanceDetailPage } from "@/features/procurement/components/advance-pages"

export const Route = createFileRoute("/_app/procurement/purchase-advances/$id")(
  {
    validateSearch: z.object({
      tab: z.enum(["summary", "history"]).optional().catch(undefined),
    }),
    component: RouteComponent,
  }
)

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <AdvanceDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
