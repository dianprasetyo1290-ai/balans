import { createFileRoute } from "@tanstack/react-router"

import { CogsReportPage } from "@/features/reports/components/stock-report-pages"
import { periodSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/cogs")({
  validateSearch: periodSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <CogsReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
