import { createFileRoute, redirect } from "@tanstack/react-router"

import { meQuery } from "@/features/auth/api/auth-api"
import { MfaSetupPage } from "@/features/auth/components/mfa-setup-page"
import { ensureSession } from "@/lib/api/auth-fetch"

export const Route = createFileRoute("/mfa-setup")({
  beforeLoad: async ({ context }) => {
    if (!(await ensureSession())) {
      throw redirect({ to: "/login" })
    }
    const me = await context.queryClient.fetchQuery(meQuery)
    if (!me.mfaEnrollmentRequired) {
      throw redirect({ to: "/" })
    }
  },
  component: MfaSetupPage,
})
