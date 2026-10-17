import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PartnersPage } from "@/features/party/components/partners-page"

export const Route = createFileRoute("/_app/master/partners")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    partnerType: z.enum(["modern_trade", "consignment", "b2b"]).optional(),
  }),
  component: PartnersPage,
})
