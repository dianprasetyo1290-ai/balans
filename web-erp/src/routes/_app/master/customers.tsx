import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { CustomersPage } from "@/features/party/components/customers-page"

export const Route = createFileRoute("/_app/master/customers")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    customerType: z.enum(["retail", "b2b", "reseller", "employee"]).optional(),
  }),
  component: CustomersPage,
})
