import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PriceListsPage } from "@/features/pricing/components/price-lists-page"

export const Route = createFileRoute("/_app/master/price-lists/")({
  validateSearch: z.object({
    q: z.string().optional(),
    priceListType: z
      .enum(["base", "channel", "partner", "reseller", "employee", "vending"])
      .optional()
      .catch(undefined),
    includeInactive: z.boolean().optional().catch(undefined),
    deleted: z.boolean().optional().catch(undefined),
  }),
  component: PriceListsRoute,
})

function PriceListsRoute() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PriceListsPage
      search={search}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
