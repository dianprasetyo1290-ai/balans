import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { InTransitPage } from "@/features/warehouse/components/transfer-pages"

export const Route = createFileRoute("/_app/warehouse/in-transit")({
  validateSearch: z.object({ page: z.number().int().min(1).catch(1) }),
  component: RouteComponent,
})

function RouteComponent() {
  const { page } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <InTransitPage
      page={page}
      onPageChange={(p) => void navigate({ search: { page: p } })}
    />
  )
}
