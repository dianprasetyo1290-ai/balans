import { createFileRoute } from "@tanstack/react-router"

import { JournalCreatePage } from "@/features/accounting/components/journal-form-page"

export const Route = createFileRoute("/_app/finance/journals/new")({
  component: JournalCreatePage,
})
