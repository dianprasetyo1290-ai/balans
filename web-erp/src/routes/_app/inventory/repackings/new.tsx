import { createFileRoute } from "@tanstack/react-router"

import { RepackingFormPage } from "@/features/inventory/components/repacking-pages"

export const Route = createFileRoute("/_app/inventory/repackings/new")({
  component: () => <RepackingFormPage />,
})
