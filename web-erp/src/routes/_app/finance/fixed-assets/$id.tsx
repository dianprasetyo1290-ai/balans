import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { FixedAssetDetailPage } from "@/features/accounting/components/fixed-asset-detail-page"

export const Route = createFileRoute("/_app/finance/fixed-assets/$id")({
  validateSearch: z.object({
    tab: z.enum(["detail", "history"]).catch("detail"),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <FixedAssetDetailPage key={id} id={id} />
}
