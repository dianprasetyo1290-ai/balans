import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { RequestListPage } from "@/features/procurement/components/request-list-page"

export const Route = createFileRoute("/_app/procurement/purchase-requests/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "pending_approval",
        "approved",
        "rejected",
        "completed",
        "closed",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
    mine: z.boolean().optional().catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: RequestListPage,
})
