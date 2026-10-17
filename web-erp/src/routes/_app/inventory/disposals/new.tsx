import { createFileRoute } from "@tanstack/react-router"

import { DisposalFormPage } from "@/features/inventory/components/disposal-pages"

export const Route = createFileRoute("/_app/inventory/disposals/new")({
  component: () => <DisposalFormPage />,
})
