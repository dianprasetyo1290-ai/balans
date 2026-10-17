import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReturnDispositionFormPage } from "@/features/inventory/components/return-disposition-pages"

export const Route = createFileRoute("/_app/warehouse/return-dispositions/new")(
  {
    validateSearch: z.object({ locationId: z.string().optional() }),
    component: RouteComponent,
  }
)

function RouteComponent() {
  const { locationId } = Route.useSearch()
  return <ReturnDispositionFormPage locationId={locationId} />
}
