import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReceiptCreatePage } from "@/features/procurement/components/receipt-pages"

export const Route = createFileRoute("/_app/procurement/goods-receipts/new")({
  validateSearch: z.object({
    purchaseOrderId: z.string().uuid().optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { purchaseOrderId } = Route.useSearch()
  return (
    <ReceiptCreatePage
      key={purchaseOrderId ?? ""}
      purchaseOrderId={purchaseOrderId}
    />
  )
}
