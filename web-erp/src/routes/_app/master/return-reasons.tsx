import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReturnReasonsPage } from "@/features/sales/components/return-reasons-page"

export const Route = createFileRoute("/_app/master/return-reasons")({
  validateSearch: z.object({
    q: z.string().optional(),
    includeInactive: z.boolean().catch(false),
  }),
  component: ReturnReasonsPage,
})
