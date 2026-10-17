import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PriceRulesPage } from "@/features/pricing/components/price-rules-page"

export const Route = createFileRoute("/_app/master/price-rules")({
  validateSearch: z.object({
    tab: z
      .enum(["promotions", "tiered", "special"])
      .optional()
      .catch(undefined),
    includeEnded: z.boolean().optional().catch(undefined),
  }),
  component: PriceRulesRoute,
})

function PriceRulesRoute() {
  const { tab, includeEnded } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PriceRulesPage
      tab={tab ?? "promotions"}
      includeEnded={includeEnded ?? false}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
