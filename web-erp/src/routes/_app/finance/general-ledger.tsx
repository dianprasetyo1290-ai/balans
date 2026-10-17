import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { GeneralLedgerPage } from "@/features/accounting/components/general-ledger-page"
import { todayBusinessDate } from "@/lib/format/date"

const firstOfMonth = () => `${todayBusinessDate().slice(0, 8)}01`

export const Route = createFileRoute("/_app/finance/general-ledger")({
  validateSearch: z.object({
    accountId: z.string().optional(),
    dateFrom: z.string().catch(firstOfMonth),
    dateTo: z.string().catch(() => todayBusinessDate()),
    divisionId: z.string().optional(),
    channelId: z.string().optional(),
    page: z.number().int().min(1).catch(1),
  }),
  component: GeneralLedgerPage,
})
