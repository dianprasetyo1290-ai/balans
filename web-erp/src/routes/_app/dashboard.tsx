import { createFileRoute } from "@tanstack/react-router"

import { ExecutiveDashboardPage } from "@/features/reports/components/finance-report-pages"
import { periodSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/dashboard")({
  validateSearch: periodSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ExecutiveDashboardPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
