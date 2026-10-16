import { createFileRoute } from "@tanstack/react-router"

import { RolesPage } from "@/features/identity/components/roles-page"

export const Route = createFileRoute("/_app/settings/roles")({
  component: RolesPage,
})
