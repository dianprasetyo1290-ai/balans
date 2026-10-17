import { createFileRoute } from "@tanstack/react-router"

import { ConsignmentDeliveryListPage } from "@/features/sales/components/consignment-delivery-pages"
import { consignmentDeliverySearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/consignments/")({
  validateSearch: consignmentDeliverySearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ConsignmentDeliveryListPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
