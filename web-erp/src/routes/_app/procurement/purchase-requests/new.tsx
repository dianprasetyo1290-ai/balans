import { createFileRoute } from "@tanstack/react-router"

import { RequestCreatePage } from "@/features/procurement/components/request-form-page"

export const Route = createFileRoute("/_app/procurement/purchase-requests/new")(
  {
    component: RequestCreatePage,
  }
)
