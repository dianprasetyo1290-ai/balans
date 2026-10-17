import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { TrialBalancePage } from "@/features/accounting/components/trial-balance-page"
import { previousPeriod } from "@/features/accounting/api/close-api"
import { todayBusinessDate } from "@/lib/format/date"

const prev = () => previousPeriod(todayBusinessDate())

export const Route = createFileRoute("/_app/finance/reports")({
  validateSearch: z.object({
    year: z
      .number()
      .int()
      .min(2000)
      .catch(() => prev().year),
    month: z
      .number()
      .int()
      .min(1)
      .max(12)
      .catch(() => prev().month),
    divisionId: z.string().optional(),
    channelId: z.string().optional(),
  }),
  component: TrialBalancePage,
})
