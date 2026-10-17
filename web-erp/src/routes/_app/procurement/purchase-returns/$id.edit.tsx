import { createFileRoute } from "@tanstack/react-router"

import { ReturnEditPage } from "@/features/procurement/components/return-pages"

export const Route = createFileRoute(
  "/_app/procurement/purchase-returns/$id/edit"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ReturnEditPage key={id} id={id} />
}
