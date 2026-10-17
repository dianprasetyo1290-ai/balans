import { z } from "zod"

const returnStatuses = [
  "draft",
  "submitted",
  "verified",
  "in_progress",
  "posted",
  "cancelled",
] as const

const creditNoteStatuses = [
  "posted",
  "completed",
  "pending_approval",
  "awaiting_payment",
  "paid",
] as const

/** Filter daftar Retur Penjualan di URL. */
export const returnListSearchSchema = z.object({
  page: z.number().int().min(1).catch(1),
  q: z.string().optional(),
  status: z.enum(returnStatuses).optional().catch(undefined),
  assignedToMe: z.boolean().optional().catch(undefined),
})

/** Filter daftar Nota Kredit di URL. */
export const creditNoteListSearchSchema = z.object({
  page: z.number().int().min(1).catch(1),
  q: z.string().optional(),
  status: z.enum(creditNoteStatuses).optional().catch(undefined),
})

/** Mode form lapangan retur dari HP. */
export const returnFieldSearchSchema = z.object({
  mode: z.enum(["pickup", "receive"]).catch("receive"),
})
