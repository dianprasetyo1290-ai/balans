import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PriceListDetailPage } from "@/features/pricing/components/price-list-detail-page"

export const Route = createFileRoute("/_app/master/price-lists/$id")({
  validateSearch: z.object({
    tab: z.enum(["versions", "history"]).optional().catch(undefined),
  }),
  component: PriceListDetailRoute,
})

function PriceListDetailRoute() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PriceListDetailPage
      id={id}
      tab={tab ?? "versions"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
