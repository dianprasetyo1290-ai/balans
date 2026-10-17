import { createFileRoute } from "@tanstack/react-router"

import { RepackingFormPage } from "@/features/inventory/components/repacking-pages"

export const Route = createFileRoute("/_app/inventory/repackings/$id/edit")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <RepackingFormPage key={id} id={id} />
}
