import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { DeliveryOrderListPage } from "@/features/logistics/components/delivery-order-pages"

export const Route = createFileRoute("/_app/logistics/delivery-orders/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum(["draft", "in_progress", "in_transit", "received", "cancelled"])
      .optional()
      .catch(undefined),
  }),
  component: DeliveryOrderListPage,
})
