import { z } from "zod"

import { todayBusinessDate } from "@/lib/format/date"

import type { Period } from "../api/reports-api"

/** Nama read model (backend) → label tampilan. */
export const READ_MODEL_LABELS: Record<string, string> = {
  "accounting.gl": "jurnal",
  "sales.sales_lines": "penjualan",
  "inventory.stock_values": "stok",
}

/** Read model sumber tiap laporan. */
export const RM = {
  gl: ["accounting.gl"],
  sales: ["sales.sales_lines"],
  stock: ["inventory.stock_values"],
} as const

export const LOCATION_TYPE_LABELS: Record<string, string> = {
  inbound: "Inbound",
  finished_goods: "FG Non-Ecommerce",
  ecommerce: "E-commerce Stock",
  petty_stock: "Petty Stock",
  raw_material: "RM",
  goods_material: "GM",
  returns: "Retur",
  event: "Event",
  toll_manufacturer: "Maklon",
  consignment: "Konsinyasi",
  vending_buffer: "Buffer Vending",
  vending_machine: "Mesin Vending",
  in_transit: "Dalam Pengiriman",
}

export const EXPIRY_BUCKET_LABELS: Record<string, string> = {
  expired: "Sudah kedaluwarsa",
  near: "Mendekati kedaluwarsa",
  d31_90: "31–90 hari",
  d91_180: "91–180 hari",
  over_180: "> 180 hari",
  no_expiry: "Tanpa tanggal kedaluwarsa",
}

export const DISPOSITION_LABELS: Record<string, string> = {
  restock: "Restock",
  repack: "Repacking",
  reprocess: "Reprocess",
  marketing_sample: "Sampel Marketing",
  dispose: "Pemusnahan",
}

export const USAGE_PURPOSE_LABELS: Record<string, string> = {
  marketing: "Marketing",
  event: "Event",
  employee_sample: "Sampel Karyawan",
  sales_sampling: "Sampling Sales",
}

export const DIMENSION_LABELS: Record<string, string> = {
  channel: "Channel",
  sub_channel: "Sub-channel",
  division: "Divisi",
  campaign: "Kampanye",
}

export const GROUP_BY_LABELS: Record<string, string> = {
  channel: "Channel",
  sub_channel: "Sub-channel",
  product: "Produk (SKU)",
  category: "Kategori",
  month: "Bulan",
  day: "Hari",
}

/** Awal bulan dari tanggal `YYYY-MM-DD`. */
export function monthStart(date: string): string {
  return `${date.slice(0, 7)}-01`
}

/** Periode default: awal bulan berjalan s.d. hari ini. */
export function resolvePeriod(
  s: { dateFrom?: string; dateTo?: string },
  today = todayBusinessDate()
): Period {
  const dateTo = s.dateTo || today
  return { dateFrom: s.dateFrom || monthStart(dateTo), dateTo }
}

/** Nilai desimal string → number untuk diagram (presisi tampilan saja). */
export function toNumber(v: string | undefined | null): number {
  const n = Number(v ?? 0)
  return Number.isFinite(n) ? n : 0
}

/** Nama bulan singkat dari `YYYY-MM-DD`. */
export function monthLabel(date: string): string {
  const names = [
    "Jan",
    "Feb",
    "Mar",
    "Apr",
    "Mei",
    "Jun",
    "Jul",
    "Agu",
    "Sep",
    "Okt",
    "Nov",
    "Des",
  ]
  const m = Number(date.slice(5, 7))
  return `${names[m - 1] ?? date.slice(5, 7)} ${date.slice(2, 4)}`
}

/** Persentase perubahan dua nilai; null bila pembanding nol. */
export function changePercent(
  current: string,
  previous: string
): number | null {
  const c = toNumber(current)
  const p = toNumber(previous)
  if (p === 0) {
    return null
  }
  return ((c - p) / Math.abs(p)) * 100
}

export const periodSearchSchema = z.object({
  dateFrom: z.string().optional().catch(undefined),
  dateTo: z.string().optional().catch(undefined),
})

export const statementSearchSchema = periodSearchSchema.extend({
  tab: z
    .enum(["balance_sheet", "income_statement", "cash_flow"])
    .optional()
    .catch(undefined),
})

export const dimensionSearchSchema = periodSearchSchema.extend({
  tab: z.enum(["dimension", "margin"]).optional().catch(undefined),
  dimension: z
    .enum(["channel", "sub_channel", "division", "campaign"])
    .optional()
    .catch(undefined),
  groupBy: z.enum(["product", "category"]).optional().catch(undefined),
})

export const salesSearchSchema = periodSearchSchema.extend({
  groupBy: z
    .enum(["channel", "sub_channel", "product", "category", "month", "day"])
    .optional()
    .catch(undefined),
})

export const stockSearchSchema = z.object({
  locationType: z.string().optional().catch(undefined),
  withinDays: z.number().int().min(0).optional().catch(undefined),
})

export const stockMovementSearchSchema = periodSearchSchema.extend({
  locationType: z.string().optional().catch(undefined),
})

export const stockCountSearchSchema = periodSearchSchema.extend({
  varianceOnly: z.boolean().optional().catch(undefined),
})

export const payablesSearchSchema = z.object({
  tab: z.enum(["aging", "outstanding_po"]).optional().catch(undefined),
  asOf: z.string().optional().catch(undefined),
})

export const taxSearchSchema = z.object({
  year: z.number().int().min(2000).max(2100).optional().catch(undefined),
  month: z.number().int().min(1).max(12).optional().catch(undefined),
})

export const settlementSearchSchema = z.object({
  tab: z.enum(["settlements", "receivables"]).optional().catch(undefined),
  status: z.enum(["draft", "posted", "cancelled"]).optional().catch(undefined),
  page: z.number().int().min(1).catch(1),
  q: z.string().optional().catch(undefined),
  open: z.boolean().optional().catch(undefined),
  id: z.uuid().optional().catch(undefined),
})

export type PeriodSearch = z.infer<typeof periodSearchSchema>
export type StatementSearch = z.infer<typeof statementSearchSchema>
export type DimensionSearch = z.infer<typeof dimensionSearchSchema>
export type SalesSearch = z.infer<typeof salesSearchSchema>
export type StockSearch = z.infer<typeof stockSearchSchema>
export type StockMovementSearch = z.infer<typeof stockMovementSearchSchema>
export type StockCountSearch = z.infer<typeof stockCountSearchSchema>
export type PayablesSearch = z.infer<typeof payablesSearchSchema>
export type TaxSearch = z.infer<typeof taxSearchSchema>
export type SettlementSearch = z.infer<typeof settlementSearchSchema>

/** Pembaruan search halaman laporan (dipasang route). */
export type SearchUpdater<T> = (update: (s: T) => T) => void
