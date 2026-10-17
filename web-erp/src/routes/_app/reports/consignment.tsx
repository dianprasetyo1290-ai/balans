import { createFileRoute } from "@tanstack/react-router"

import { ConsignmentReportPage } from "@/features/reports/components/sales-report-pages"
import { periodSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/reports/consignment")({
  validateSearch: periodSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ConsignmentReportPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
