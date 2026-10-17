import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ChannelsDivisionsPage } from "@/features/accounting/components/channels-divisions-page"

export const Route = createFileRoute("/_app/master/channels")({
  validateSearch: z.object({
    tab: z.enum(["channels", "sub-channels", "divisions"]).catch("channels"),
    includeInactive: z.boolean().catch(false),
  }),
  component: ChannelsDivisionsPage,
})
