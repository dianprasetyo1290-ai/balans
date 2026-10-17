import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { BatchesPage } from "@/features/inventory/components/batches-page"

export const Route = createFileRoute("/_app/inventory/batches")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    productId: z.string().optional(),
    q: z.string().optional(),
    expiryState: z
      .enum(["expired", "near_expiry", "ok", "no_expiry"])
      .optional()
      .catch(undefined),
    withStock: z.boolean().optional().catch(undefined),
  }),
  component: BatchesPage,
})
