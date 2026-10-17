import { createFileRoute } from "@tanstack/react-router"

import { DisposalPrintPage } from "@/features/inventory/components/disposal-pages"

export const Route = createFileRoute("/_app/inventory/disposals/$id/print")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <DisposalPrintPage id={id} />
}
