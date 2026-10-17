import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { BillListPage } from "@/features/procurement/components/bill-pages"

export const Route = createFileRoute("/_app/procurement/supplier-bills/")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    status: z
      .enum([
        "draft",
        "pending_approval",
        "rejected",
        "awaiting_payment",
        "partially_paid",
        "paid",
        "cancelled",
      ])
      .optional()
      .catch(undefined),
    dateFrom: z.string().optional(),
    dateTo: z.string().optional(),
  }),
  component: BillListPage,
})
