import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ProcurementApprovalsPage } from "@/features/procurement/components/procurement-approvals-page"

export const Route = createFileRoute("/_app/procurement/approvals")({
  validateSearch: z.object({
    tab: z.enum(["inbox", "requested", "decided"]).catch("inbox"),
    page: z.number().int().min(1).catch(1),
    subjectType: z
      .enum(["purchase_request", "purchase_order"])
      .optional()
      .catch(undefined),
  }),
  component: ProcurementApprovalsPage,
})
