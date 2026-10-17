import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { StockCountListPage } from "@/features/warehouse/components/stock-count-pages"

export const Route = createFileRoute("/_app/warehouse/stock-counts/")({
  validateSearch: z.object({
    tab: z.enum(["counts", "schedules"]).optional().catch(undefined),
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "in_progress",
        "pending_approval",
        "rejected",
        "posted",
        "completed",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
    countType: z
      .enum(["full", "cycle", "consignment", "vending"])
      .optional()
      .catch(undefined),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { tab, ...search } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <StockCountListPage
      tab={tab ?? "counts"}
      search={search}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
