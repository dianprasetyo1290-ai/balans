import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReturnCreatePage } from "@/features/procurement/components/return-pages"

export const Route = createFileRoute("/_app/procurement/purchase-returns/new")({
  validateSearch: z.object({
    purchaseOrderId: z.string().uuid().optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { purchaseOrderId } = Route.useSearch()
  return (
    <ReturnCreatePage
      key={purchaseOrderId ?? ""}
      purchaseOrderId={purchaseOrderId}
    />
  )
}
