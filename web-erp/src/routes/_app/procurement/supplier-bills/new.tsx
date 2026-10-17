import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { BillCreatePage } from "@/features/procurement/components/bill-pages"

export const Route = createFileRoute("/_app/procurement/supplier-bills/new")({
  validateSearch: z.object({
    purchaseOrderId: z.string().uuid().optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { purchaseOrderId } = Route.useSearch()
  return (
    <BillCreatePage
      key={purchaseOrderId ?? ""}
      purchaseOrderId={purchaseOrderId}
    />
  )
}
