import { createFileRoute } from "@tanstack/react-router"

import { FinancialStatementsPage } from "@/features/reports/components/finance-report-pages"
import { statementSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/finance/financial-statements")({
  validateSearch: statementSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <FinancialStatementsPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
