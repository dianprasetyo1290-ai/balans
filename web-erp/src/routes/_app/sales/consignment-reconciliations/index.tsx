import { createFileRoute } from "@tanstack/react-router"

import { ReconciliationListPage } from "@/features/sales/components/consignment-reconciliation-pages"
import { reconciliationSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute(
  "/_app/sales/consignment-reconciliations/"
)({
  validateSearch: reconciliationSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ReconciliationListPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
