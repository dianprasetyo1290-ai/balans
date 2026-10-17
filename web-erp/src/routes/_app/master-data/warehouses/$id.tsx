import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { WarehouseDetailPage } from "@/features/warehouse/components/warehouse-detail-page"

export const Route = createFileRoute("/_app/master-data/warehouses/$id")({
  validateSearch: z.object({
    tab: z.enum(["locations", "history"]).optional().catch(undefined),
    page: z.number().int().min(1).optional().catch(undefined),
  }),
  component: WarehouseDetailRoute,
})

function WarehouseDetailRoute() {
  const { id } = Route.useParams()
  const { tab, page } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <WarehouseDetailPage
      id={id}
      tab={tab ?? "locations"}
      page={page ?? 1}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
