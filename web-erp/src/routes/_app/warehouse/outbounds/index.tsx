import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { OutboundListPage } from "@/features/warehouse/components/outbound-pages"

export const Route = createFileRoute("/_app/warehouse/outbounds/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum(["in_progress", "completed", "cancelled"])
      .optional()
      .catch(undefined),
  }),
  component: OutboundListPage,
})
