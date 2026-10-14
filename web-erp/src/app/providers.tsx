import { QueryClientProvider } from "@tanstack/react-query"
import { RouterProvider } from "@tanstack/react-router"
import { useState } from "react"

import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"

import { createQueryClient } from "./query-client"
import { createAppRouter } from "./router"

export function AppProviders() {
  const [queryClient] = useState(createQueryClient)
  const [router] = useState(() => createAppRouter(queryClient))

  return (
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <TooltipProvider>
          <RouterProvider router={router} />
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  )
}
