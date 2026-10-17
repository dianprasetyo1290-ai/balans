import { createFileRoute } from "@tanstack/react-router"

import { TaxReportPage } from "@/features/reports/components/finance-report-pages"
import { taxSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/tax")({
  validateSearch: taxSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <TaxReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
