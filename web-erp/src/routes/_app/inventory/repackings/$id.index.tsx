import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { RepackingDetailPage } from "@/features/inventory/components/repacking-pages"

export const Route = createFileRoute("/_app/inventory/repackings/$id/")({
  validateSearch: z.object({
    tab: z.enum(["detail", "approval", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <RepackingDetailPage key={id} id={id} />
}
