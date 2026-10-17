import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { InboundListPage } from "@/features/warehouse/components/inbound-pages"

export const Route = createFileRoute("/_app/warehouse/inbounds/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z.enum(["in_progress", "completed"]).optional().catch(undefined),
  }),
  component: InboundListPage,
})
