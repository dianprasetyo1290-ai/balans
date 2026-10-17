import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { BillDetailPage } from "@/features/procurement/components/bill-pages"

export const Route = createFileRoute("/_app/procurement/supplier-bills/$id/")({
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
    <BillDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
