import { createFileRoute } from "@tanstack/react-router"

import { SalesReturnCreatePage } from "@/features/sales/components/sales-return-pages"

export const Route = createFileRoute("/_app/sales/returns/new")({
  component: SalesReturnCreatePage,
})
