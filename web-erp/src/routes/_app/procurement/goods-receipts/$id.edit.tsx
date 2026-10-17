import { createFileRoute } from "@tanstack/react-router"

import { ReceiptEditPage } from "@/features/procurement/components/receipt-pages"

export const Route = createFileRoute(
  "/_app/procurement/goods-receipts/$id/edit"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ReceiptEditPage key={id} id={id} />
}
