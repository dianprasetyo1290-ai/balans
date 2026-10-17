import { createFileRoute } from "@tanstack/react-router"

import { CreditNoteDetailPage } from "@/features/sales/components/credit-note-pages"
import { detailSearchSchema } from "@/features/sales/lib/search"

export const Route = createFileRoute("/_app/sales/credit-notes/$id/")({
  validateSearch: detailSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const { id } = Route.useParams()
  const { tab } = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <CreditNoteDetailPage
      key={id}
      id={id}
      tab={tab ?? "summary"}
      onTabChange={(t) => void navigate({ search: { tab: t } })}
    />
  )
}
