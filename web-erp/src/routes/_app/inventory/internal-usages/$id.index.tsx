import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { InternalUsageDetailPage } from "@/features/inventory/components/internal-usage-pages"

export const Route = createFileRoute("/_app/inventory/internal-usages/$id/")({
  validateSearch: z.object({
    tab: z.enum(["detail", "approval", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <InternalUsageDetailPage key={id} id={id} />
}
