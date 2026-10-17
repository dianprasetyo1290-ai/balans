import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { StockCardPage } from "@/features/inventory/components/stock-card-page"
import { todayBusinessDate } from "@/lib/format/date"

const firstOfMonth = () => `${todayBusinessDate().slice(0, 8)}01`

export const Route = createFileRoute("/_app/inventory/stock-card")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    productId: z.string().optional(),
    locationId: z.string().optional(),
    batchId: z.string().optional(),
    status: z
      .enum(["available", "quarantine", "in_transit", "reserved"])
      .optional()
      .catch(undefined),
    dateFrom: z.string().catch(firstOfMonth),
    dateTo: z.string().catch(() => todayBusinessDate()),
  }),
  component: StockCardPage,
})
