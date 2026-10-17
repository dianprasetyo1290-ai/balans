import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ProductDetailPage } from "@/features/catalog/components/product-detail-page"

export const Route = createFileRoute("/_app/master-data/products/$id")({
  validateSearch: z.object({
    tab: z.enum(["summary", "history"]).optional().catch(undefined),
  }),
  component: ProductDetailRoute,
})

function ProductDetailRoute() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ProductDetailPage
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
