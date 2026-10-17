import { createFileRoute } from "@tanstack/react-router"

import { BillEditPage } from "@/features/procurement/components/bill-pages"

export const Route = createFileRoute(
  "/_app/procurement/supplier-bills/$id/edit"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <BillEditPage key={id} id={id} />
}
