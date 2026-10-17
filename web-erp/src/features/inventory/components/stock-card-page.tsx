import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { Field, FieldLabel } from "@/components/ui/field"
import { formatDate } from "@/lib/format/date"
import { formatRupiah, formatUnitCost } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"

import {
  sourceTypeLabel,
  stockCardQuery,
  type StockCardLine,
  type StockCardSearch,
} from "../api/inventory-api"
import {
  BatchFilter,
  LocationFilter,
  ProductFilter,
  StatusFilter,
  StockStatusBadge,
} from "./stock-filters"

const route = getRouteApi("/_app/inventory/stock-card")

const qtyOrBlank = (q: string) => (Number(q) ? formatQuantity(q) : "")

/** Persediaan › Kartu Stok: mutasi per SKU/lokasi/batch dengan saldo qty dan nilai berjalan (INV-01, S-213). */
export function StockCardPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const card = useQuery({
    ...stockCardQuery({ ...search, productId: search.productId ?? "" }),
    enabled: Boolean(search.productId),
  })
  const setSearch = (patch: Partial<StockCardSearch>) =>
    void navigate({ search: (s) => ({ ...s, ...patch, page: 1 }) })
  const uom = card.data?.product.uom ?? ""

  const columns: DataTableColumn<StockCardLine>[] = [
    {
      id: "date",
      header: "Tanggal",
      cell: (l) => (
        <span className="whitespace-nowrap">{formatDate(l.businessDate)}</span>
      ),
    },
    {
      id: "source",
      header: "Dokumen",
      cell: (l) => (
        <div className="flex min-w-40 flex-col">
          <span className="font-medium">{l.sourceLabel || "—"}</span>
          <span className="text-xs text-muted-foreground">
            {sourceTypeLabel(l.sourceType)} · {l.movementTypeLabel}
          </span>
          {l.batchReason ? (
            <span className="text-xs text-amber-700 dark:text-amber-300">
              Batch manual: {l.batchReason}
            </span>
          ) : null}
          {l.note ? (
            <span className="text-xs text-muted-foreground">{l.note}</span>
          ) : null}
        </div>
      ),
    },
    {
      id: "location",
      header: "Lokasi",
      cell: (l) => <span className="whitespace-nowrap">{l.location.code}</span>,
    },
    {
      id: "batch",
      header: "Batch",
      cell: (l) => (
        <span className="whitespace-nowrap">{l.batchNumber ?? "—"}</span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (l) => <StockStatusBadge status={l.status} />,
    },
    {
      id: "in",
      header: "Masuk",
      align: "right",
      cell: (l) => <span className="tabular-nums">{qtyOrBlank(l.qtyIn)}</span>,
    },
    {
      id: "out",
      header: "Keluar",
      align: "right",
      cell: (l) => <span className="tabular-nums">{qtyOrBlank(l.qtyOut)}</span>,
    },
    {
      id: "unitCost",
      header: "HPP/unit",
      align: "right",
      cell: (l) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatUnitCost(l.unitCost)}
        </span>
      ),
    },
    {
      id: "amount",
      header: "Nilai",
      align: "right",
      cell: (l) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatRupiah(l.costAmount)}
        </span>
      ),
    },
    {
      id: "runningQty",
      header: "Saldo qty",
      align: "right",
      cell: (l) => (
        <span className="font-medium whitespace-nowrap tabular-nums">
          {formatQuantity(l.runningQty)}
        </span>
      ),
    },
    {
      id: "runningValue",
      header: "Saldo nilai",
      align: "right",
      cell: (l) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatRupiah(l.runningValue)}
        </span>
      ),
    },
  ]

  const data = card.data
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Kartu Stok</h1>
        <p className="text-sm text-muted-foreground">
          Mutasi stok dari ledger per SKU, dengan filter lokasi, batch, dan
          status. Saldo berjalan dimulai dari saldo awal sebelum tanggal awal.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <ProductFilter
          id="card-product"
          value={search.productId}
          onChange={(productId) => setSearch({ productId, batchId: undefined })}
        />
        <LocationFilter
          id="card-location"
          value={search.locationId}
          onChange={(locationId) => setSearch({ locationId })}
        />
        <BatchFilter
          id="card-batch"
          productId={search.productId}
          value={search.batchId}
          onChange={(batchId) => setSearch({ batchId })}
        />
        <StatusFilter
          id="card-status"
          value={search.status}
          onChange={(status) => setSearch({ status })}
        />
        <Field>
          <FieldLabel htmlFor="card-from">Dari tanggal</FieldLabel>
          <DateInput
            id="card-from"
            value={search.dateFrom}
            onValueChange={(v) => setSearch({ dateFrom: v })}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="card-to">Sampai tanggal</FieldLabel>
          <DateInput
            id="card-to"
            value={search.dateTo}
            onValueChange={(v) => setSearch({ dateTo: v })}
          />
        </Field>
      </div>
      {!search.productId ? (
        <p className="text-sm text-muted-foreground">
          Pilih produk untuk melihat Kartu Stok.
        </p>
      ) : (
        <>
          {data ? (
            <dl className="grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <Summary
                label="Saldo awal"
                qty={data.openingQty}
                value={data.openingValue}
                uom={uom}
              />
              <Summary label="Total masuk" qty={data.qtyIn} uom={uom} />
              <Summary label="Total keluar" qty={data.qtyOut} uom={uom} />
              <Summary
                label="Saldo akhir"
                qty={data.closingQty}
                value={data.closingValue}
                uom={uom}
                strong
              />
            </dl>
          ) : null}
          <DataTable
            columns={columns}
            rows={data?.items}
            getRowId={(l) => l.movementId}
            loading={card.isPending}
            fetching={card.isFetching}
            page={search.page}
            totalPages={data?.totalPages ?? 0}
            totalItems={data?.totalItems ?? 0}
            onSortChange={() => undefined}
            onPageChange={(page) =>
              void navigate({ search: (s) => ({ ...s, page }) })
            }
            emptyMessage="Tidak ada mutasi pada rentang tanggal ini."
          />
        </>
      )}
    </div>
  )
}

function Summary({
  label,
  qty,
  value,
  uom,
  strong = false,
}: {
  label: string
  qty: string
  value?: string
  uom: string
  strong?: boolean
}) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={strong ? "text-lg font-semibold" : "text-lg"}>
        {formatQuantity(qty)} {uom}
      </dd>
      {value !== undefined ? (
        <dd className="text-muted-foreground">{formatRupiah(value)}</dd>
      ) : null}
    </div>
  )
}
