import { createFileRoute } from "@tanstack/react-router"

import { StockMovementReportPage } from "@/features/reports/components/stock-report-pages"
import { stockMovementSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/stock-movements")({
  validateSearch: stockMovementSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StockMovementReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
