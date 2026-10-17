import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { OutboundDetailPage } from "@/features/warehouse/components/outbound-pages"

export const Route = createFileRoute("/_app/warehouse/outbounds/$id")({
  validateSearch: z.object({
    tab: z.enum(["lines", "packages", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <OutboundDetailPage
      key={id}
      id={id}
      tab={tab ?? "lines"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
