import { createFileRoute } from "@tanstack/react-router"

import { CouriersPage } from "@/features/logistics/components/shipment-pages"

export const Route = createFileRoute("/_app/logistics/couriers")({
  component: CouriersPage,
})
