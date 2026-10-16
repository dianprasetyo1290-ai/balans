import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { SetPasswordPage } from "@/features/auth/components/password-pages"

export const Route = createFileRoute("/set-password")({
  validateSearch: z.object({ token: z.string().catch("") }),
  component: SetPasswordRoute,
})

function SetPasswordRoute() {
  const { token } = Route.useSearch()
  return <SetPasswordPage token={token} />
}
