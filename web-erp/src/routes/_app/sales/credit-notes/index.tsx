import { createFileRoute } from "@tanstack/react-router"

import { CreditNoteListPage } from "@/features/sales/components/credit-note-pages"
import { creditNoteListSearchSchema } from "@/features/sales/lib/return-search"

export const Route = createFileRoute("/_app/sales/credit-notes/")({
  validateSearch: creditNoteListSearchSchema,
  component: RouteComponent,
})

function RouteComponent() {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <CreditNoteListPage
      search={search}
      onSearchChange={(update) =>
        void navigate({ search: (s) => update(s) as typeof s })
      }
    />
  )
}
