import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { DisposalDetailPage } from "@/features/inventory/components/disposal-pages"

export const Route = createFileRoute("/_app/inventory/disposals/$id/")({
  validateSearch: z.object({
    tab: z.enum(["detail", "approval", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <DisposalDetailPage key={id} id={id} />
}
