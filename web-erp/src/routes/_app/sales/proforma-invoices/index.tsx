import { createFileRoute } from "@tanstack/react-router"

import { ProformaListPage } from "@/features/sales/components/billing-pages"
import { documentListSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/proforma-invoices/")({
  validateSearch: documentListSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ProformaListPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
