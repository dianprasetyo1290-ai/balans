import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { TransferListPage } from "@/features/warehouse/components/transfer-pages"

export const Route = createFileRoute("/_app/warehouse/stock-transfers/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "submitted",
        "in_progress",
        "in_transit",
        "partially_received",
        "received",
        "completed",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <TransferListPage
      type="direct"
      search={search}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
