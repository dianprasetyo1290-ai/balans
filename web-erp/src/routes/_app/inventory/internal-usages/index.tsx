import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { InternalUsageListPage } from "@/features/inventory/components/internal-usage-pages"

export const Route = createFileRoute("/_app/inventory/internal-usages/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "pending_approval",
        "approved",
        "rejected",
        "in_progress",
        "awaiting_settlement",
        "posted",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
  }),
  component: InternalUsageListPage,
})
