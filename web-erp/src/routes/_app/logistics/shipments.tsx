import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ShipmentListPage } from "@/features/logistics/components/shipment-pages"

export const Route = createFileRoute("/_app/logistics/shipments")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum(["picked_up", "in_transit", "delivered", "returned"])
      .optional()
      .catch(undefined),
    mine: z.boolean().optional().catch(undefined),
  }),
  component: ShipmentListPage,
})
