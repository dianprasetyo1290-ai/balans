import { createFileRoute } from "@tanstack/react-router"

import { productListSearchSchema } from "@/features/catalog/components/product-list-search"
import { ProductsPage } from "@/features/catalog/components/products-page"

export const Route = createFileRoute("/_app/master-data/products/")({
  validateSearch: productListSearchSchema,
  component: FinishedGoodsPage,
})

function FinishedGoodsPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ProductsPage
      kind="finished"
      search={search}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
