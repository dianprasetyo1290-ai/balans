import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ReceiptListPage } from "@/features/procurement/components/receipt-pages"

export const Route = createFileRoute("/_app/procurement/goods-receipts/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum(["draft", "posted", "cancelled"])
      .optional()
      .catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: ReceiptListPage,
})
