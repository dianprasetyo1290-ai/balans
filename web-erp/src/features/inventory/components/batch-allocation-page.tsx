import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { useState } from "react"

import { QuantityInput } from "@/components/common/quantity-input"
import { Button } from "@/components/ui/button"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { parseProblem } from "@/lib/api/problem"
import { formatDate } from "@/lib/format/date"
import { formatQuantity } from "@/lib/format/number"

import { batchSuggestionQuery, type SuggestedBatch } from "../api/inventory-api"
import { ExpiryBadge, LocationFilter, ProductFilter } from "./stock-filters"

const route = getRouteApi("/_app/inventory/batch-allocation")

/**
 * Persediaan › Alokasi Batch (FEFO): saran batch kedaluwarsa terdekat untuk mengambil qty dari satu lokasi
 * (S-215). Batch kedaluwarsa dan stok Karantina tidak diusulkan; pilihan manual di dokumen wajib alasan.
 */
export function BatchAllocationPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [qty, setQty] = useState(search.qty ?? "")
  const ready = Boolean(search.productId && search.locationId && search.qty)
  const suggestion = useQuery({
    ...batchSuggestionQuery(
      search.productId ?? "",
      search.locationId ?? "",
      search.qty ?? ""
    ),
    enabled: ready,
  })
  const data = suggestion.data
  const problem = suggestion.error ? parseProblem(suggestion.error) : null

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Alokasi Batch (FEFO)
        </h1>
        <p className="text-sm text-muted-foreground">
          Saran batch dengan tanggal kedaluwarsa terdekat (First Expired First
          Out) untuk picking dan transfer. Batch yang sudah kedaluwarsa dan stok
          Karantina tidak diusulkan. Memilih batch lain di dokumen wajib mengisi
          alasan.
        </p>
      </div>
      <form
        className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4 lg:items-end"
        onSubmit={(e) => {
          e.preventDefault()
          void navigate({ search: (s) => ({ ...s, qty: qty || undefined }) })
        }}
      >
        <ProductFilter
          id="fefo-product"
          value={search.productId}
          onChange={(productId) =>
            void navigate({ search: (s) => ({ ...s, productId }) })
          }
        />
        <LocationFilter
          id="fefo-location"
          value={search.locationId}
          allLabel="Pilih lokasi"
          onChange={(locationId) =>
            void navigate({ search: (s) => ({ ...s, locationId }) })
          }
        />
        <Field>
          <FieldLabel htmlFor="fefo-qty">Qty dibutuhkan</FieldLabel>
          <QuantityInput
            id="fefo-qty"
            value={qty}
            onValueChange={setQty}
            unit={data?.product.uom}
          />
        </Field>
        <Button type="submit" className="min-h-11 sm:min-h-9">
          Lihat Saran
        </Button>
      </form>
      {!ready ? (
        <p className="text-sm text-muted-foreground">
          Pilih produk, lokasi, dan qty untuk melihat saran batch.
        </p>
      ) : null}
      {problem ? (
        <p className="text-sm text-destructive" role="alert">
          {Object.values(problem.fields ?? {})[0] ?? problem.detail}
        </p>
      ) : null}
      {data ? (
        <>
          <dl className="grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-4">
            <Info label="Diminta" value={data.requestedQty} />
            <Info label="Bisa dialokasikan" value={data.allocatedQty} />
            <Info
              label="Kurang"
              value={data.shortfallQty}
              danger={Number(data.shortfallQty) > 0}
            />
            <Info
              label="Stok Karantina (tidak diusulkan)"
              value={data.quarantineQty}
            />
          </dl>
          <SuggestionTable
            caption="Batch Tersedia (urutan FEFO)"
            rows={data.items}
            emptyMessage="Tidak ada stok Tersedia yang belum kedaluwarsa di lokasi ini."
          />
          {data.expired.length > 0 ? (
            <SuggestionTable
              caption="Batch kedaluwarsa (tidak diusulkan)"
              rows={data.expired}
              expired
            />
          ) : null}
        </>
      ) : null}
    </div>
  )
}

function Info({
  label,
  value,
  danger = false,
}: {
  label: string
  value: string
  danger?: boolean
}) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd
        className={
          danger ? "text-lg font-semibold text-destructive" : "text-lg"
        }
      >
        {formatQuantity(value)}
      </dd>
    </div>
  )
}

function SuggestionTable({
  caption,
  rows,
  expired = false,
  emptyMessage,
}: {
  caption: string
  rows: SuggestedBatch[]
  expired?: boolean
  emptyMessage?: string
}) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-base font-medium">{caption}</h2>
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Urutan</TableHead>
              <TableHead>Batch</TableHead>
              <TableHead>Produksi</TableHead>
              <TableHead>Kedaluwarsa</TableHead>
              <TableHead className="text-right">Stok Tersedia</TableHead>
              {!expired ? (
                <TableHead className="text-right">Saran ambil</TableHead>
              ) : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="py-6 text-center text-muted-foreground"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            ) : null}
            {rows.map((b, i) => (
              <TableRow
                key={b.batchId ?? `none-${i}`}
                data-suggested={Number(b.suggestedQty) > 0 || undefined}
                className={
                  Number(b.suggestedQty) > 0 ? "bg-emerald-500/5" : undefined
                }
              >
                <TableCell>{i + 1}</TableCell>
                <TableCell className="font-medium whitespace-nowrap">
                  {b.batchNumber || "Tanpa batch"}
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {b.productionDate ? formatDate(b.productionDate) : "—"}
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  <span className="mr-2">
                    {b.expiryDate ? formatDate(b.expiryDate) : "—"}
                  </span>
                  {expired ? <ExpiryBadge state="expired" /> : null}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatQuantity(b.availableQty)}
                </TableCell>
                {!expired ? (
                  <TableCell className="text-right font-medium tabular-nums">
                    {Number(b.suggestedQty)
                      ? formatQuantity(b.suggestedQty)
                      : "—"}
                  </TableCell>
                ) : null}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  )
}
