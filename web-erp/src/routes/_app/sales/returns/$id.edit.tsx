import { createFileRoute } from "@tanstack/react-router"

import { SalesReturnEditPage } from "@/features/sales/components/sales-return-pages"

export const Route = createFileRoute("/_app/sales/returns/$id/edit")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <SalesReturnEditPage key={id} id={id} />
}
