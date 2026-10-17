import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { SearchIcon } from "lucide-react"
import { useState } from "react"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { Button } from "@/components/ui/button"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { formatDate } from "@/lib/format/date"
import { formatQuantity } from "@/lib/format/number"

import {
  batchesQuery,
  EXPIRY_STATE_LABELS,
  type BatchSearch,
  type BatchSummary,
  type ExpiryState,
} from "../api/inventory-api"
import { ExpiryBadge, ProductFilter } from "./stock-filters"

const route = getRouteApi("/_app/inventory/batches")

const ALL = "all"

const qty = (q: string) => (Number(q) ? formatQuantity(q) : "—")

function daysLabel(days?: number) {
  if (days === undefined) return "—"
  if (days < 0) return `lewat ${-days} hari`
  if (days === 0) return "hari ini"
  return `${days} hari lagi`
}

/** Persediaan › Batch & Kedaluwarsa: batch berurutan FEFO beserta stok per status (INV-04, S-215). */
export function BatchesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const batches = useQuery(batchesQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const setSearch = (patch: Partial<BatchSearch>) =>
    void navigate({ search: (s) => ({ ...s, ...patch, page: 1 }) })

  const columns: DataTableColumn<BatchSummary>[] = [
    {
      id: "batch",
      header: "Batch",
      cell: (b) => (
        <span className="font-medium whitespace-nowrap">{b.batchNumber}</span>
      ),
    },
    {
      id: "product",
      header: "Produk",
      cell: (b) => (
        <div className="flex min-w-44 flex-col">
          <span>{b.product.sku}</span>
          <span className="text-xs text-muted-foreground">
            {b.product.name}
          </span>
        </div>
      ),
    },
    {
      id: "production",
      header: "Produksi",
      cell: (b) => (
        <span className="whitespace-nowrap">
          {b.productionDate ? formatDate(b.productionDate) : "—"}
        </span>
      ),
    },
    {
      id: "expiry",
      header: "Kedaluwarsa",
      cell: (b) => (
        <div className="flex flex-col items-start gap-1 whitespace-nowrap">
          <span>{b.expiryDate ? formatDate(b.expiryDate) : "—"}</span>
          <span className="text-xs text-muted-foreground">
            {daysLabel(b.daysToExpiry)}
          </span>
        </div>
      ),
    },
    {
      id: "state",
      header: "Kondisi",
      cell: (b) => <ExpiryBadge state={b.expiryState} />,
    },
    {
      id: "available",
      header: "Tersedia",
      align: "right",
      cell: (b) => <span className="tabular-nums">{qty(b.qtyAvailable)}</span>,
    },
    {
      id: "quarantine",
      header: "Karantina",
      align: "right",
      cell: (b) => <span className="tabular-nums">{qty(b.qtyQuarantine)}</span>,
    },
    {
      id: "transit",
      header: "Dalam Pengiriman",
      align: "right",
      cell: (b) => <span className="tabular-nums">{qty(b.qtyInTransit)}</span>,
    },
    {
      id: "reserved",
      header: "Dialokasikan",
      align: "right",
      cell: (b) => <span className="tabular-nums">{qty(b.qtyReserved)}</span>,
    },
    {
      id: "total",
      header: "Total",
      align: "right",
      cell: (b) => (
        <span className="font-medium whitespace-nowrap tabular-nums">
          {formatQuantity(b.qtyTotal)} {b.product.uom}
        </span>
      ),
    },
    {
      id: "links",
      header: "",
      align: "right",
      cell: (b) => (
        <div className="flex justify-end gap-3 whitespace-nowrap">
          <Link
            to="/inventory/stock"
            search={{ page: 1, productId: b.product.id, batchId: b.id }}
            className="inline-flex min-h-11 items-center text-primary underline-offset-4 hover:underline"
          >
            Stok
          </Link>
          <Link
            to="/inventory/cost-layers"
            search={{ page: 1, productId: b.product.id, batchId: b.id }}
            className="inline-flex min-h-11 items-center text-primary underline-offset-4 hover:underline"
          >
            HPP
          </Link>
        </div>
      ),
    },
  ]

  const states = Object.keys(EXPIRY_STATE_LABELS) as ExpiryState[]
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Batch & Kedaluwarsa
        </h1>
        <p className="text-sm text-muted-foreground">
          Batch diurutkan dari tanggal kedaluwarsa terdekat (FEFO). Segera
          kedaluwarsa = {batches.data?.nearExpiryDays ?? 90} hari atau kurang.
          Batch kedaluwarsa tidak diusulkan saat alokasi.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <ProductFilter
          id="batch-product"
          value={search.productId}
          onChange={(productId) => setSearch({ productId })}
        />
        <Field>
          <FieldLabel htmlFor="batch-q">Nomor batch</FieldLabel>
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              setSearch({ q: q || undefined })
            }}
          >
            <Input
              id="batch-q"
              value={q}
              placeholder="Cari nomor batch"
              onChange={(e) => setQ(e.target.value)}
            />
            <Button
              type="submit"
              variant="outline"
              size="icon"
              aria-label="Cari"
            >
              <SearchIcon />
            </Button>
          </form>
        </Field>
        <Field>
          <FieldLabel htmlFor="batch-state">Kondisi</FieldLabel>
          <Select
            value={search.expiryState ?? ALL}
            onValueChange={(v) =>
              setSearch({
                expiryState: !v || v === ALL ? undefined : (v as ExpiryState),
              })
            }
          >
            <SelectTrigger id="batch-state" className="w-full">
              <SelectValue>
                {(v: string) =>
                  v === ALL
                    ? "Semua kondisi"
                    : EXPIRY_STATE_LABELS[v as ExpiryState]
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>Semua kondisi</SelectItem>
              {states.map((s) => (
                <SelectItem key={s} value={s}>
                  {EXPIRY_STATE_LABELS[s]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="batch-stock">Stok</FieldLabel>
          <Select
            value={search.withStock === false ? "all" : "with"}
            onValueChange={(v) =>
              setSearch({ withStock: v === "all" ? false : undefined })
            }
          >
            <SelectTrigger id="batch-stock" className="w-full">
              <SelectValue>
                {(v: string) =>
                  v === "all" ? "Termasuk stok habis" : "Masih ada stok"
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="with">Masih ada stok</SelectItem>
              <SelectItem value="all">Termasuk stok habis</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <DataTable
        columns={columns}
        rows={batches.data?.items}
        getRowId={(b) => b.id}
        loading={batches.isPending}
        fetching={batches.isFetching}
        page={search.page}
        totalPages={batches.data?.totalPages ?? 0}
        totalItems={batches.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada batch yang cocok."
      />
    </div>
  )
}
