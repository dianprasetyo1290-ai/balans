import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReturnsWarehousePage } from "@/features/inventory/components/return-disposition-pages"

export const Route = createFileRoute("/_app/warehouse/returns")({
  validateSearch: z.object({ locationId: z.string().optional() }),
  component: ReturnsWarehousePage,
})
