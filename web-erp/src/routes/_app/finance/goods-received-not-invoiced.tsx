import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { GoodsReceivedNotInvoicedPage } from "@/features/procurement/components/grni-report-page"

export const Route = createFileRoute(
  "/_app/finance/goods-received-not-invoiced"
)({
  validateSearch: z.object({
    q: z.string().optional(),
  }),
  component: GoodsReceivedNotInvoicedPage,
})
