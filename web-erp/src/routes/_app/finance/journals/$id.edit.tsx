import { createFileRoute } from "@tanstack/react-router"

import { JournalEditPage } from "@/features/accounting/components/journal-form-page"

export const Route = createFileRoute("/_app/finance/journals/$id/edit")({
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  return <JournalEditPage key={id} id={id} />
}
