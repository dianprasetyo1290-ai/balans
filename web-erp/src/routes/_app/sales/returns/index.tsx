import { createFileRoute } from "@tanstack/react-router"

import { SalesReturnListPage } from "@/features/sales/components/sales-return-pages"
import { returnListSearchSchema } from "@/features/sales/lib/return-search"

export const Route = createFileRoute("/_app/sales/returns/")({
  validateSearch: returnListSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <SalesReturnListPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
