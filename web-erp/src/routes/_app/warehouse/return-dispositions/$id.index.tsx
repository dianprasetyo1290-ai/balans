import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReturnDispositionDetailPage } from "@/features/inventory/components/return-disposition-pages"

export const Route = createFileRoute(
  "/_app/warehouse/return-dispositions/$id/"
)({
  validateSearch: z.object({
    tab: z.enum(["detail", "approval", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ReturnDispositionDetailPage key={id} id={id} />
}
