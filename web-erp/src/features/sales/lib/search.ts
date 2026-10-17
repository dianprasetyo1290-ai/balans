import { z } from "zod"

const statuses = [
  "draft",
  "pending_approval",
  "awaiting_payment",
  "partially_paid",
  "paid",
  "in_progress",
  "in_transit",
  "completed",
  "cancelled",
  "expired",
  "posted",
] as const

const orderTypes = [
  "modern_trade",
  "b2b",
  "social_commerce",
  "marketplace",
  "employee",
] as const

/** Filter daftar pesanan di URL (Desain UI/UX 3.2). */
export const orderListSearchSchema = z.object({
  page: z.number().int().min(1).catch(1),
  sort: z.string().optional(),
  q: z.string().optional(),
  status: z.enum(statuses).optional().catch(undefined),
  orderType: z.enum(orderTypes).optional().catch(undefined),
  dateFrom: z.string().optional(),
  dateTo: z.string().optional(),
  /** Job impor marketplace yang sedang dibuka. */
  job: z.string().optional(),
})

export const orderTypeSearchSchema = z.object({
  orderType: z.enum(orderTypes).catch("modern_trade"),
})

export const documentListSearchSchema = z.object({
  page: z.number().int().min(1).catch(1),
  sort: z.string().optional(),
  q: z.string().optional(),
  status: z.enum(statuses).optional().catch(undefined),
})

export const receivableSearchSchema = z.object({
  tab: z.enum(["invoices", "payments"]).optional().catch(undefined),
  page: z.number().int().min(1).catch(1),
  sort: z.string().optional(),
  q: z.string().optional(),
  overdue: z.boolean().optional().catch(undefined),
  pay: z.string().uuid().optional().catch(undefined),
})

export const agingSearchSchema = z.object({
  asOf: z.string().optional(),
  partyType: z.enum(["customer", "partner"]).optional().catch(undefined),
})

export const detailSearchSchema = z.object({
  tab: z.enum(["summary", "history"]).optional().catch(undefined),
})
