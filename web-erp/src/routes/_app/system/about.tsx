import { createFileRoute } from "@tanstack/react-router"

import { AboutSystemPage } from "@/features/system/components/about-system-page"

export const Route = createFileRoute("/_app/system/about")({
  component: AboutSystemPage,
})
