import { createFileRoute } from "@tanstack/react-router"

import { ForbiddenPage } from "@/components/layout/status-page"

export const Route = createFileRoute("/forbidden")({
  component: ForbiddenPage,
})
