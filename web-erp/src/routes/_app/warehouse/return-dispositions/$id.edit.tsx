import { createFileRoute } from "@tanstack/react-router"

import { ReturnDispositionFormPage } from "@/features/inventory/components/return-disposition-pages"

export const Route = createFileRoute(
  "/_app/warehouse/return-dispositions/$id/edit"
)({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <ReturnDispositionFormPage key={id} id={id} />
}
