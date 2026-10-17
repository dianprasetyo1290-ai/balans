import { createFileRoute } from "@tanstack/react-router"

import { ReceivablesPage } from "@/features/sales/components/receivable-pages"
import { receivableSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/finance/receivables")({
  validateSearch: receivableSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ReceivablesPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
