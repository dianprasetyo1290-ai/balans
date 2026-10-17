import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { PackageLabelsPage } from "@/features/logistics/components/print-pages"

export const Route = createFileRoute("/_app/logistics/package-labels")({
  validateSearch: z.object({
    deliveryOrderId: z.string().optional(),
  }),
  component: RouteComponent,
})

function RouteComponent() {
  const { deliveryOrderId } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <PackageLabelsPage
      key={deliveryOrderId ?? ""}
      deliveryOrderId={deliveryOrderId}
      onSelect={(id) => void navigate({ search: { deliveryOrderId: id } })}
    />
  )
}
