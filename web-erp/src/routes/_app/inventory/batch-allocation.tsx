import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { BatchAllocationPage } from "@/features/inventory/components/batch-allocation-page"

export const Route = createFileRoute("/_app/inventory/batch-allocation")({
  validateSearch: z.object({
    productId: z.string().optional(),
    locationId: z.string().optional(),
    qty: z.string().optional(),
  }),
  component: BatchAllocationPage,
})
