import { createFileRoute } from "@tanstack/react-router"

import { ReceivableAgingPage } from "@/features/sales/components/receivable-pages"
import { agingSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/reports/receivable-aging")({
  validateSearch: agingSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ReceivableAgingPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
