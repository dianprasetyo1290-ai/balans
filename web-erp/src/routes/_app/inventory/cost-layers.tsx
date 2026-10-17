import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { CostLayersPage } from "@/features/inventory/components/cost-layers-page"

export const Route = createFileRoute("/_app/inventory/cost-layers")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    productId: z.string().optional(),
    batchId: z.string().optional(),
    openOnly: z.boolean().optional().catch(undefined),
  }),
  component: CostLayersPage,
})
