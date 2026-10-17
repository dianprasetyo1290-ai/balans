import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { StockCountDetailPage } from "@/features/warehouse/components/stock-count-pages"

export const Route = createFileRoute("/_app/warehouse/stock-counts/$id")({
  validateSearch: z.object({
    tab: z.enum(["count", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StockCountDetailPage
      key={id}
      id={id}
      tab={tab ?? "count"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
