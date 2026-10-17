import { createFileRoute } from "@tanstack/react-router"

import { OrderListPage } from "@/features/sales/components/order-list-page"
import { orderListSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/social-commerce")({
  validateSearch: orderListSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <OrderListPage
      kind="social_commerce"
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
