import { createFileRoute } from "@tanstack/react-router"

import { MarketplaceSettlementPage } from "@/features/reports/components/settlement-page"
import { settlementSearchSchema } from "@/features/reports/lib/reports-lib"

export const Route = createFileRoute("/_app/finance/marketplace-settlements")({
  validateSearch: settlementSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <MarketplaceSettlementPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s as typeof search) })
      }
    />
  )
}
