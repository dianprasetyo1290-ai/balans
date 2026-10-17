import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { StockBalancesPage } from "@/features/inventory/components/stock-balances-page"

export const Route = createFileRoute("/_app/inventory/stock")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    productId: z.string().optional(),
    locationId: z.string().optional(),
    batchId: z.string().optional(),
    status: z
      .enum(["available", "quarantine", "in_transit", "reserved"])
      .optional()
      .catch(undefined),
  }),
  component: StockBalancesPage,
})
