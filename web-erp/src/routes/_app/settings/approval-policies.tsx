import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ApprovalPoliciesPage } from "@/features/approval/components/approval-policies-page"

export const Route = createFileRoute("/_app/settings/approval-policies")({
  validateSearch: z.object({
    includeEnded: z.boolean().catch(false),
  }),
  component: ApprovalPoliciesPage,
})
