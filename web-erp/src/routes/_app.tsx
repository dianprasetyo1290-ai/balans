import { createFileRoute, Outlet } from "@tanstack/react-router"

import { AppSidebar } from "@/components/layout/app-sidebar"
import { Separator } from "@/components/ui/separator"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { systemInfoQuery } from "@/features/system/api/system-queries"

/**
 * Layout utama ERP. Login dan pengecekan sesi ditambahkan di beforeLoad saat modul identity
 * dibangun (R1.1); selama Fase 0 akses dibatasi di Caddy.
 */
export const Route = createFileRoute("/_app")({
  loader: ({ context }) => context.queryClient.ensureQueryData(systemInfoQuery),
  component: AppLayout,
})

function AppLayout() {
  const info = Route.useLoaderData()

  return (
    <SidebarProvider>
      <AppSidebar enabledModules={info.enabledModules} version={info.version} />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" aria-label="Buka atau tutup menu" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <span className="text-sm text-muted-foreground">Balans ERP</span>
          {info.environment !== "production" ? (
            <span className="ml-auto rounded-md bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-800 dark:text-amber-300">
              {info.environment === "staging" ? "Staging" : "Lokal"}
            </span>
          ) : null}
        </header>
        <main className="flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
