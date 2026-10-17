import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { CampaignsPage } from "@/features/accounting/components/campaigns-page"

export const Route = createFileRoute("/_app/master/campaigns")({
  validateSearch: z.object({
    q: z.string().optional(),
    includeInactive: z.boolean().catch(false),
  }),
  component: CampaignsPage,
})
