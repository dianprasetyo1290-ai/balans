import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { SuppliersPage } from "@/features/party/components/suppliers-page"

export const Route = createFileRoute("/_app/master/suppliers")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    supplierType: z.enum(["supplier", "toll_manufacturer"]).optional(),
  }),
  component: SuppliersPage,
})
