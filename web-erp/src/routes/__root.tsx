import type { QueryClient } from "@tanstack/react-query"
import { createRootRouteWithContext, Outlet } from "@tanstack/react-router"

import { ErrorDialog } from "@/components/common/error-dialog"
import { NotFoundPage, StatusPage } from "@/components/layout/status-page"
import { Toaster } from "@/components/ui/sonner"
import { ReauthDialog } from "@/features/auth/components/reauth-dialog"

export type RouterContext = {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
  errorComponent: () => (
    <StatusPage
      code="500"
      title="Terjadi kesalahan"
      description="Halaman tidak dapat ditampilkan. Muat ulang halaman atau laporkan ke tim IT."
    />
  ),
})

function RootLayout() {
  return (
    <>
      <Outlet />
      <ErrorDialog />
      <ReauthDialog />
      <Toaster richColors position="top-right" />
    </>
  )
}
