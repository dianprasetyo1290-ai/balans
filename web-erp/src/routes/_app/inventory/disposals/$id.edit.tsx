import { createFileRoute } from "@tanstack/react-router"

import { DisposalFormPage } from "@/features/inventory/components/disposal-pages"

export const Route = createFileRoute("/_app/inventory/disposals/$id/edit")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <DisposalFormPage key={id} id={id} />
}
