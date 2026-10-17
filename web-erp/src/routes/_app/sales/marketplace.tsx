import { createFileRoute } from "@tanstack/react-router"

import { MarketplacePage } from "@/features/sales/components/marketplace-page"
import { orderListSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/marketplace")({
  validateSearch: orderListSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <MarketplacePage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
