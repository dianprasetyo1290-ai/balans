import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReturnListPage } from "@/features/procurement/components/return-pages"

export const Route = createFileRoute("/_app/procurement/purchase-returns/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "pending_approval",
        "approved",
        "rejected",
        "posted",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
    origin: z
      .enum(["manual", "qc_inbound", "qc_outbound"])
      .optional()
      .catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: ReturnListPage,
})
