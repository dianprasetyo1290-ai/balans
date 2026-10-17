import { createFileRoute } from "@tanstack/react-router"

import { productListSearchSchema } from "@/features/catalog/components/product-list-search"
import { ProductsPage } from "@/features/catalog/components/products-page"

export const Route = createFileRoute("/_app/master-data/bundles")({
  validateSearch: productListSearchSchema,
  component: BundlesPage,
})

function BundlesPage() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ProductsPage
      kind="bundle"
      search={search}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
