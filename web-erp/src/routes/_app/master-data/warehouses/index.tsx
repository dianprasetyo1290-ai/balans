import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { WarehousesPage } from "@/features/warehouse/components/warehouses-page"

export const Route = createFileRoute("/_app/master-data/warehouses/")({
  validateSearch: z.object({
    tab: z.enum(["warehouses", "external"]).catch("warehouses"),
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    deleted: z.boolean().optional().catch(undefined),
  }),
  component: WarehousesRoute,
})

function WarehousesRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <WarehousesPage
      search={search}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
