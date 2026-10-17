import { createFileRoute } from "@tanstack/react-router"

import { JournalDetailPage } from "@/features/accounting/components/journal-detail-page"

export const Route = createFileRoute("/_app/finance/journals/$id/")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <JournalDetailPage key={id} id={id} />
}
