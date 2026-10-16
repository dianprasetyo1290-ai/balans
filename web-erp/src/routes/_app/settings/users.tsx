import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { UsersPage } from "@/features/identity/components/users-page"

export const Route = createFileRoute("/_app/settings/users")({
  validateSearch: z.object({
    page: z.number().int().min(1).catch(1),
    sort: z.string().optional(),
    q: z.string().optional(),
    isActive: z.boolean().optional(),
  }),
  component: UsersPage,
})
