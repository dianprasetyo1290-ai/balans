import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { TrackingPage } from "@/features/logistics/components/shipment-pages"

export const Route = createFileRoute("/_app/logistics/tracking")({
  validateSearch: z.object({ q: z.string().optional() }),
  component: RouteComponent,
})

function RouteComponent() {
  const { q } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <TrackingPage
      key={q ?? ""}
      q={q}
      onSearch={(v) => void navigate({ search: { q: v } })}
    />
  )
}
