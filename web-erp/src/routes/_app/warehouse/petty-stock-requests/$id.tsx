import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { TransferDetailPage } from "@/features/warehouse/components/transfer-pages"

export const Route = createFileRoute(
  "/_app/warehouse/petty-stock-requests/$id"
)({
  validateSearch: z.object({
    tab: z.enum(["lines", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <TransferDetailPage
      key={id}
      id={id}
      tab={tab ?? "lines"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
