import { createFileRoute } from "@tanstack/react-router"

import { InternalUsageFormPage } from "@/features/inventory/components/internal-usage-pages"

export const Route = createFileRoute("/_app/inventory/internal-usages/new")({
  component: () => <InternalUsageFormPage />,
})
