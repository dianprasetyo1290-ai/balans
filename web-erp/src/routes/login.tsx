import { createFileRoute, redirect } from "@tanstack/react-router"
import { z } from "zod"

import { LoginPage } from "@/features/auth/components/login-page"
import { ensureSession } from "@/lib/api/auth-fetch"

export const Route = createFileRoute("/login")({
  validateSearch: z.object({ redirect: z.string().optional() }),
  beforeLoad: async ({ search }) => {
    if (await ensureSession()) {
      throw redirect({
        to:
          search.redirect && search.redirect.startsWith("/")
            ? search.redirect
            : "/",
      })
    }
  },
  component: LoginRoute,
})

function LoginRoute() {
  const { redirect: to } = Route.useSearch()
  return <LoginPage redirect={to} />
}
