import { useQuery } from "@tanstack/react-query"

import { Badge } from "@/components/ui/badge"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { productQuery } from "@/features/catalog/api/catalog-api"
import { ProductPicker } from "@/features/catalog/components/product-picker"
import { locationsQuery } from "@/features/warehouse/api/warehouse-api"
import { formatDate } from "@/lib/format/date"
import { cn } from "@/lib/utils"

import {
  EXPIRY_STATE_LABELS,
  productBatchesQuery,
  STOCK_STATUS_LABELS,
  STOCK_STATUSES,
  type ExpiryState,
  type StockStatus,
} from "../api/inventory-api"

const ALL = "all"

const STATUS_TONES: Record<StockStatus, string> = {
  available: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  quarantine: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  in_transit: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  reserved: "bg-violet-500/10 text-violet-700 dark:text-violet-300",
}

const EXPIRY_TONES: Record<ExpiryState, string> = {
  expired: "bg-destructive/10 text-destructive",
  near_expiry: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  ok: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  no_expiry: "bg-muted text-muted-foreground",
}

/** Badge status stok: Tersedia, Karantina, Dalam Pengiriman, Dialokasikan. */
export function StockStatusBadge({ status }: { status: StockStatus }) {
  return (
    <Badge
      variant="outline"
      className={cn("border-transparent", STATUS_TONES[status])}
      data-status={status}
    >
      {STOCK_STATUS_LABELS[status]}
    </Badge>
  )
}

/** Badge kondisi kedaluwarsa batch. */
export function ExpiryBadge({ state }: { state: ExpiryState }) {
  return (
    <Badge
      variant="outline"
      className={cn("border-transparent", EXPIRY_TONES[state])}
      data-expiry={state}
    >
      {EXPIRY_STATE_LABELS[state]}
    </Badge>
  )
}

/** Tanggal kedaluwarsa + badge kondisinya. */
export function ExpiryCell({
  expiryDate,
  state,
}: {
  expiryDate?: string
  state?: ExpiryState
}) {
  if (!expiryDate) {
    return <span className="text-muted-foreground">—</span>
  }
  return (
    <div className="flex flex-col items-start gap-1 whitespace-nowrap">
      <span>{formatDate(expiryDate)}</span>
      {state && state !== "ok" ? <ExpiryBadge state={state} /> : null}
    </div>
  )
}

/** Filter produk dengan pemilih Cari Produk; produk terpilih dibaca dari URL. */
export function ProductFilter({
  id,
  value,
  onChange,
  label = "Produk",
}: {
  id: string
  value?: string
  onChange: (id: string | undefined) => void
  label?: string
}) {
  const product = useQuery({
    ...productQuery(value ?? ""),
    enabled: Boolean(value),
  })
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex gap-2">
        <ProductPicker
          id={id}
          value={value && product.data ? product.data : null}
          isBundle={false}
          placeholder="Semua produk"
          onValueChange={(p) => onChange(p.id)}
        />
        {value ? (
          <button
            type="button"
            className="min-h-9 shrink-0 rounded-md px-2 text-sm text-muted-foreground hover:bg-accent"
            onClick={() => onChange(undefined)}
          >
            Hapus
          </button>
        ) : null}
      </div>
    </Field>
  )
}

/** Filter lokasi (internal dan eksternal). */
export function LocationFilter({
  id,
  value,
  onChange,
  allLabel = "Semua lokasi",
}: {
  id: string
  value?: string
  onChange: (id: string | undefined) => void
  allLabel?: string
}) {
  const locations = useQuery(locationsQuery({ page: 1 }))
  const items = locations.data?.items ?? []
  const label = (v: string) => {
    if (v === ALL) return allLabel
    const l = items.find((x) => x.id === v)
    return l ? `${l.code} · ${l.name}` : "Memuat…"
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>Lokasi</FieldLabel>
      <Select
        value={value ?? ALL}
        onValueChange={(v) => onChange(!v || v === ALL ? undefined : String(v))}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue>{(v: string) => label(v)}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>{allLabel}</SelectItem>
          {items.map((l) => (
            <SelectItem key={l.id} value={l.id}>
              {l.code} · {l.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}

/** Filter batch produk terpilih. */
export function BatchFilter({
  id,
  productId,
  value,
  onChange,
}: {
  id: string
  productId?: string
  value?: string
  onChange: (id: string | undefined) => void
}) {
  const batches = useQuery({
    ...productBatchesQuery(productId ?? ""),
    enabled: Boolean(productId),
  })
  const items = batches.data?.items ?? []
  const label = (v: string) => {
    if (v === ALL) return productId ? "Semua batch" : "Pilih produk dulu"
    return items.find((b) => b.id === v)?.batchNumber ?? "Memuat…"
  }
  return (
    <Field>
      <FieldLabel htmlFor={id}>Batch</FieldLabel>
      <Select
        value={value ?? ALL}
        disabled={!productId}
        onValueChange={(v) => onChange(!v || v === ALL ? undefined : String(v))}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue>{(v: string) => label(v)}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Semua batch</SelectItem>
          {items.map((b) => (
            <SelectItem key={b.id} value={b.id}>
              {b.batchNumber}
              {b.expiryDate ? ` · ${formatDate(b.expiryDate)}` : ""}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}

/** Filter status stok. */
export function StatusFilter({
  id,
  value,
  onChange,
}: {
  id: string
  value?: StockStatus
  onChange: (s: StockStatus | undefined) => void
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>Status stok</FieldLabel>
      <Select
        value={value ?? ALL}
        onValueChange={(v) =>
          onChange(!v || v === ALL ? undefined : (v as StockStatus))
        }
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue>
            {(v: string) =>
              v === ALL ? "Semua status" : STOCK_STATUS_LABELS[v as StockStatus]
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Semua status</SelectItem>
          {STOCK_STATUSES.map((s) => (
            <SelectItem key={s} value={s}>
              {STOCK_STATUS_LABELS[s]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}
