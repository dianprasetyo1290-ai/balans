import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PricingSalesPage } from "@/features/pricing/components/pricing-sales-page"

export const Route = createFileRoute("/_app/sales/pricing")({
  validateSearch: z.object({
    tab: z.enum(["simulator", "limits"]).optional().catch(undefined),
    includeEnded: z.boolean().optional().catch(undefined),
  }),
  component: PricingSalesRoute,
})

function PricingSalesRoute() {
  const { tab, includeEnded } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PricingSalesPage
      tab={tab ?? "simulator"}
      includeEnded={includeEnded ?? false}
      onSearchChange={(patch) =>
        void navigate({ search: (s) => ({ ...s, ...patch }) })
      }
    />
  )
}
