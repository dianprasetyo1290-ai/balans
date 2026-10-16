import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { AuditLogsPage } from "@/features/identity/components/audit-logs-page"

export const Route = createFileRoute("/_app/settings/audit-logs")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    entityType: z.string().optional(),
    entityId: z.string().optional(),
    action: z
      .enum([
        "create",
        "update",
        "delete",
        "restore",
        "status_change",
        "approve",
        "reject",
        "submit",
        "post",
        "cancel",
        "login",
        "login_failed",
        "logout",
        "import",
        "export",
      ])
      .optional(),
  }),
  component: AuditLogsPage,
})
