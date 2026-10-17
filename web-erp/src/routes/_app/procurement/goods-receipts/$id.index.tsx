import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReceiptDetailPage } from "@/features/procurement/components/receipt-pages"

export const Route = createFileRoute("/_app/procurement/goods-receipts/$id/")({
  validateSearch: z.object({
    tab: z.enum(["summary", "history"]).optional().catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <ReceiptDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
