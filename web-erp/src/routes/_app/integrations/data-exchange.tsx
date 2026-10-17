import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { DataExchangePage } from "@/features/dataexchange/components/data-exchange-page"

export const Route = createFileRoute("/_app/integrations/data-exchange")({
  validateSearch: z.object({
    tab: z.enum(["import", "export", "history"]).optional().catch(undefined),
    kind: z.string().optional().catch(undefined),
    job: z.uuid().optional().catch(undefined),
    page: z.number().int().min(1).optional().catch(undefined),
  }),
  component: DataExchangePage,
})
