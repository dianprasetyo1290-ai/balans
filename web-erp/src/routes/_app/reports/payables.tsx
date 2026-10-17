import { createFileRoute } from "@tanstack/react-router"

import { PayablesReportPage } from "@/features/reports/components/sales-report-pages"
import { payablesSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/payables")({
  validateSearch: payablesSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PayablesReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
