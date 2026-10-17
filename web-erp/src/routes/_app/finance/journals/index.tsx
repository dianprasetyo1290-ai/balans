import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { JournalListPage } from "@/features/accounting/components/journal-list-page"

export const Route = createFileRoute("/_app/finance/journals/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z.enum(["draft", "posted"]).optional().catch(undefined),
    entryType: z
      .enum(["manual", "automatic", "reversal"])
      .optional()
      .catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: JournalListPage,
})
