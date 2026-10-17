import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { ProductCodesPage } from "@/features/party/components/product-codes-page"

export const Route = createFileRoute("/_app/master/partner-product-codes")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    q: z.string().optional(),
    partnerId: z.uuid().optional().catch(undefined),
  }),
  component: ProductCodesPage,
})
