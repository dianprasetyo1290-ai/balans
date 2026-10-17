import { createFileRoute } from "@tanstack/react-router"

import { StockCountReportPage } from "@/features/reports/components/stock-report-pages"
import { stockCountSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/stock-counts")({
  validateSearch: stockCountSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StockCountReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
