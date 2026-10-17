import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ProductLabelsPage } from "@/features/catalog/components/product-labels-page"

export const Route = createFileRoute("/_app/logistics/product-labels")({
  validateSearch: z.object({
    productId: z.string().optional(),
  }),
  component: ProductLabelsRoute,
})

function ProductLabelsRoute() {
  const { productId } = Route.useSearch()
  return <ProductLabelsPage key={productId ?? ""} productId={productId} />
}
