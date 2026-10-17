import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { FixedAssetsPage } from "@/features/accounting/components/fixed-assets-page"

export const Route = createFileRoute("/_app/finance/fixed-assets/")({
  validateSearch: z.object({
    tab: z.enum(["assets", "classes"]).catch("assets"),
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z.enum(["draft", "active", "disposed"]).optional().catch(undefined),
    classId: z.string().optional(),
  }),
  component: FixedAssetsPage,
})
