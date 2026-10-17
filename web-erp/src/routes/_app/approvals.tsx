import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ApprovalInboxPage } from "@/features/approval/components/approval-inbox-page"

export const Route = createFileRoute("/_app/approvals")({
  validateSearch: z.object({
    tab: z
      .enum(["inbox", "requested", "decided", "delegations"])
      .catch("inbox"),
    page: z.number().int().min(1).catch(1),
    subjectType: z.string().optional(),
  }),
  component: ApprovalInboxPage,
})
