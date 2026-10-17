import { createFileRoute } from "@tanstack/react-router"

import { DimensionProfitLossPage } from "@/features/reports/components/finance-report-pages"
import { dimensionSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/profit-loss-by-dimension")({
  validateSearch: dimensionSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <DimensionProfitLossPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
