import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { DepreciationPage } from "@/features/accounting/components/depreciation-page"

export const Route = createFileRoute("/_app/finance/depreciation")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    run: z.string().optional(),
  }),
  component: DepreciationPage,
})
