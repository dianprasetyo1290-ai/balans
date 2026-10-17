import { createFileRoute } from "@tanstack/react-router"
import { z } from "zod"

import { DocumentSequencesPage } from "@/features/document/components/document-sequences-page"

export const Route = createFileRoute("/_app/settings/document-sequences")({
  validateSearch: z.object({
    year: z.number().int().min(2000).max(9999).optional().catch(undefined),
  }),
  component: DocumentSequencesPage,
})
