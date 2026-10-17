import { createFileRoute } from "@tanstack/react-router"

import { ExpiryReportPage } from "@/features/reports/components/stock-report-pages"
import { stockSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/expiry")({
  validateSearch: stockSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ExpiryReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
