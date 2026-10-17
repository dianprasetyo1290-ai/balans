import { createFileRoute } from "@tanstack/react-router"

import { SalesReturnFieldPage } from "@/features/sales/components/sales-return-pages"
import { returnFieldSearchSchema } from "@/features/sales/lib/return-search"

export const Route = createFileRoute("/_app/sales/returns/$id/field")({
  validateSearch: returnFieldSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { mode } = Route.useSearch()
  return <SalesReturnFieldPage key={`${id}-${mode}`} id={id} mode={mode} />
}
