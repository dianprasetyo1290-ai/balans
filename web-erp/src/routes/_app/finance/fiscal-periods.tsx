import { createFileRoute } from "@tanstack/react-router"

import { FiscalPeriodsPage } from "@/features/accounting/components/fiscal-periods-page"

export const Route = createFileRoute("/_app/finance/fiscal-periods")({
  component: FiscalPeriodsPage,
})
