import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { OrderListPage } from "@/features/procurement/components/order-list-page"

export const Route = createFileRoute("/_app/procurement/toll-orders")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "pending_approval",
        "approved",
        "rejected",
        "partially_received",
        "received",
        "completed",
        "closed",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
    orderType: z
      .enum(["goods", "packaging", "other"])
      .optional()
      .catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <OrderListPage
      kind="toll"
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
