import { createFileRoute, Outlet, redirect } from "@tanstack/react-router"

import { AppSidebar } from "@/components/layout/app-sidebar"
import { Separator } from "@/components/ui/separator"
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { meQuery } from "@/features/auth/api/auth-api"
import { NotificationBell } from "@/features/notification/components/notification-bell"
import { systemInfoQuery } from "@/features/system/api/system-queries"
import { ensureSession } from "@/lib/api/auth-fetch"

/**
 * Layout utama ERP. Semua halaman di dalamnya butuh sesi: access token diambil ulang dari cookie
 * refresh token saat halaman dimuat ulang; user wajib MFA diarahkan ke aktivasi MFA (S-101, S-104).
 */
export const Route = createFileRoute("/_app")({
  beforeLoad: async ({ context, location }) => {
    if (!(await ensureSession())) {
      throw redirect({ to: "/login", search: { redirect: location.href } })
    }
    const me = await context.queryClient.fetchQuery(meQuery)
    if (me.mfaEnrollmentRequired) {
      throw redirect({ to: "/mfa-setup" })
    }
  },
  loader: async ({ context }) => {
    const [info, me] = await Promise.all([
      context.queryClient.ensureQueryData(systemInfoQuery),
      context.queryClient.ensureQueryData(meQuery),
    ])
    return { info, me }
  },
  component: AppLayout,
})

function AppLayout() {
  const { info, me } = Route.useLoaderData()

  return (
    <SidebarProvider>
      <AppSidebar
        enabledModules={info.enabledModules}
        version={info.version}
        permissions={me.permissions}
        user={me}
      />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" aria-label="Buka atau tutup menu" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <span className="text-sm text-muted-foreground">Balans ERP</span>
          <div className="ml-auto flex items-center gap-2">
            {info.environment !== "production" ? (
              <span className="rounded-md bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-800 dark:text-amber-300">
                {info.environment === "staging" ? "Staging" : "Lokal"}
              </span>
            ) : null}
            {info.enabledModules.includes("notification") ? (
              <NotificationBell />
            ) : null}
          </div>
        </header>
        <main className="flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
