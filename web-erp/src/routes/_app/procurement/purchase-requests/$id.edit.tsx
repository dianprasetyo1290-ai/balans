import { createFileRoute } from "@tanstack/react-router"

import { RequestEditPage } from "@/features/procurement/components/request-form-page"

export const Route = createFileRoute(
  "/_app/procurement/purchase-requests/$id/edit"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <RequestEditPage key={id} id={id} />
}
