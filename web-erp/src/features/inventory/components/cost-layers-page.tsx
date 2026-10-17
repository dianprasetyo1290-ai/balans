import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { useState } from "react"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatDate } from "@/lib/format/date"
import { formatRupiah, formatUnitCost } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"

import {
  costLayerConsumptionsQuery,
  costLayersQuery,
  sourceTypeLabel,
  type CostLayer,
  type CostLayerSearch,
} from "../api/inventory-api"
import { BatchFilter, ProductFilter } from "./stock-filters"

const route = getRouteApi("/_app/inventory/cost-layers")

/** Persediaan › HPP per Batch: cost layer FIFO per penerimaan dan jejak konsumsinya (INV-03, S-214). */
export function CostLayersPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const layers = useQuery(costLayersQuery(search))
  const [selected, setSelected] = useState<CostLayer | null>(null)
  const setSearch = (patch: Partial<CostLayerSearch>) =>
    void navigate({ search: (s) => ({ ...s, ...patch, page: 1 }) })

  const columns: DataTableColumn<CostLayer>[] = [
    {
      id: "product",
      header: "Produk",
      cell: (l) => (
        <div className="flex min-w-44 flex-col">
          <span className="font-medium">{l.product.sku}</span>
          <span className="text-xs text-muted-foreground">
            {l.product.name}
          </span>
        </div>
      ),
    },
    {
      id: "batch",
      header: "Batch",
      cell: (l) => (
        <div className="flex flex-col whitespace-nowrap">
          <span>{l.batchNumber ?? "—"}</span>
          {l.expiryDate ? (
            <span className="text-xs text-muted-foreground">
              Exp {formatDate(l.expiryDate)}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "source",
      header: "Penerimaan",
      cell: (l) => (
        <div className="flex min-w-36 flex-col">
          <span>{l.sourceLabel || sourceTypeLabel(l.sourceType)}</span>
          <span className="text-xs text-muted-foreground">
            {sourceTypeLabel(l.sourceType)} · {formatDate(l.businessDate)}
          </span>
        </div>
      ),
    },
    {
      id: "received",
      header: "Qty masuk",
      align: "right",
      cell: (l) => (
        <span className="tabular-nums">{formatQuantity(l.receivedQty)}</span>
      ),
    },
    {
      id: "consumed",
      header: "Terpakai",
      align: "right",
      cell: (l) => (
        <span className="tabular-nums">{formatQuantity(l.consumedQty)}</span>
      ),
    },
    {
      id: "remaining",
      header: "Sisa",
      align: "right",
      cell: (l) => (
        <span className="font-medium tabular-nums">
          {formatQuantity(l.remainingQty)}
        </span>
      ),
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
      id: "value",
      header: "Nilai sisa",
      align: "right",
      cell: (l) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatRupiah(l.remainingValue)}
        </span>
      ),
    },
    {
      id: "trace",
      header: "",
      align: "right",
      cell: (l) => (
        <Button
          variant="link"
          className="min-h-11 px-0"
          disabled={Number(l.consumedQty) === 0}
          onClick={() => setSelected(l)}
        >
          Jejak HPP
        </Button>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">HPP per Batch</h1>
        <p className="text-sm text-muted-foreground">
          Setiap penerimaan membentuk cost layer. Barang keluar mengonsumsi cost
          layer batch yang benar-benar keluar secara FIFO (penerimaan lebih awal
          lebih dulu).
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <ProductFilter
          id="layer-product"
          value={search.productId}
          onChange={(productId) => setSearch({ productId, batchId: undefined })}
        />
        <BatchFilter
          id="layer-batch"
          productId={search.productId}
          value={search.batchId}
          onChange={(batchId) => setSearch({ batchId })}
        />
        <Field>
          <FieldLabel htmlFor="layer-open">Tampilkan</FieldLabel>
          <Select
            value={search.openOnly ? "open" : "all"}
            onValueChange={(v) =>
              setSearch({ openOnly: v === "open" ? true : undefined })
            }
          >
            <SelectTrigger id="layer-open" className="w-full">
              <SelectValue>
                {(v: string) =>
                  v === "open" ? "Hanya yang masih bersisa" : "Semua layer"
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Semua layer</SelectItem>
              <SelectItem value="open">Hanya yang masih bersisa</SelectItem>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <DataTable
        columns={columns}
        rows={layers.data?.items}
        getRowId={(l) => l.id}
        loading={layers.isPending}
        fetching={layers.isFetching}
        page={search.page}
        totalPages={layers.data?.totalPages ?? 0}
        totalItems={layers.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada cost layer yang cocok."
      />
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => !open && setSelected(null)}
      >
        {selected ? <ConsumptionPanel layer={selected} /> : null}
      </Dialog>
    </div>
  )
}

function ConsumptionPanel({ layer }: { layer: CostLayer }) {
  const trace = useQuery(costLayerConsumptionsQuery(layer.id))
  const items = trace.data?.items ?? []
  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          Jejak HPP {layer.batchNumber ?? layer.product.sku}
        </DialogTitle>
        <DialogDescription>
          {layer.sourceLabel || sourceTypeLabel(layer.sourceType)} ·{" "}
          {formatQuantity(layer.receivedQty)} {layer.product.uom} @{" "}
          {formatUnitCost(layer.unitCost)}
        </DialogDescription>
      </DialogHeader>
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Tanggal</TableHead>
              <TableHead>Dokumen</TableHead>
              <TableHead>Lokasi</TableHead>
              <TableHead className="text-right">Qty</TableHead>
              <TableHead className="text-right">HPP</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {trace.isPending ? (
              <TableRow>
                <TableCell colSpan={5} className="py-6 text-center">
                  Memuat…
                </TableCell>
              </TableRow>
            ) : null}
            {!trace.isPending && items.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={5}
                  className="py-6 text-center text-muted-foreground"
                >
                  Belum ada konsumsi.
                </TableCell>
              </TableRow>
            ) : null}
            {items.map((c) => (
              <TableRow key={c.id}>
                <TableCell className="whitespace-nowrap">
                  {formatDate(c.businessDate)}
                </TableCell>
                <TableCell>
                  <div className="flex flex-col">
                    <span>{c.sourceLabel || "—"}</span>
                    <span className="text-xs text-muted-foreground">
                      {sourceTypeLabel(c.sourceType)}
                    </span>
                  </div>
                </TableCell>
                <TableCell className="whitespace-nowrap">
                  {c.location.code}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatQuantity(c.qty)}
                </TableCell>
                <TableCell className="text-right whitespace-nowrap tabular-nums">
                  {formatRupiah(c.amount)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </DialogContent>
  )
}
