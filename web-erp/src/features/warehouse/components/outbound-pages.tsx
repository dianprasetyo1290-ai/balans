import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { PrinterIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { createPortal } from "react-dom"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
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
import { Textarea } from "@/components/ui/textarea"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { api, unwrap } from "@/lib/api/client"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  batchLabel,
  locationLabel,
  MAX_PHOTO_BYTES,
} from "../api/documents-api"
import {
  distributePackages,
  dropOutboundLine,
  OUTBOUND_LINE_STATUS,
  outboundQuery,
  outboundsQuery,
  packableLines,
  packOutbound,
  pickOutbound,
  reallocateOutboundLine,
  recordOutboundQc,
  type Outbound,
  type OutboundLine,
  type OutboundLineStatus,
  type OutboundStatus,
  type OutboundSummary,
} from "../api/outbound-api"
import { Info } from "./document-parts"

const listRoute = getRouteApi("/_app/warehouse/outbounds/")
const ALL = "all"

const TONES = {
  neutral: "bg-muted text-muted-foreground",
  info: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  warning: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  success: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  danger: "bg-destructive/10 text-destructive",
} as const

export function OutboundLineBadge({ status }: { status: OutboundLineStatus }) {
  const s = OUTBOUND_LINE_STATUS[status]
  return (
    <Badge
      variant="outline"
      className={cn("border-transparent", TONES[s.tone])}
    >
      {s.label}
    </Badge>
  )
}

function onProblem(setErrors?: (f: Record<string, string>) => void) {
  return (err: unknown) => {
    const p = parseProblem(err)
    if (p.fields && setErrors) {
      setErrors(p.fields)
    } else {
      showErrorDialog(p)
    }
  }
}

/** Gudang › Outbound: picking, packing, dan QC barang keluar (S-310). */
export function OutboundListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const list = useQuery(outboundsQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const columns: DataTableColumn<OutboundSummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (o) => (
        <Link
          to="/warehouse/outbounds/$id"
          params={{ id: o.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {o.number}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (o) => (
        <span className="whitespace-nowrap">{formatDate(o.businessDate)}</span>
      ),
    },
    {
      id: "source",
      header: "Dokumen Asal",
      cell: (o) => (
        <div className="flex flex-col">
          <span>{o.sourceTypeLabel}</span>
          <span className="text-xs text-muted-foreground">
            {o.sourceLabel || "—"}
          </span>
        </div>
      ),
    },
    {
      id: "recipient",
      header: "Penerima",
      cell: (o) => o.recipientName || "—",
    },
    {
      id: "location",
      header: "Lokasi",
      cell: (o) => locationLabel(o.location),
    },
    {
      id: "pending",
      header: "Belum Selesai",
      align: "right",
      cell: (o) => (
        <span className="whitespace-nowrap">
          {o.pendingCount} dari {o.lineCount}
          {o.heldCount > 0 ? (
            <span className="block text-xs text-amber-700">
              {o.heldCount} ditahan
            </span>
          ) : null}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (o) => <StatusBadge status={o.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Outbound</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Semua barang keluar disiapkan Staf Gudang (picking per batch & packing
          koli) lalu dicek ulang (QC) oleh Fulfillment Leader atau Merchandiser
          yang bukan penyiap barang.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            void navigate({
              search: (s) => ({ ...s, q: q || undefined, page: 1 }),
            })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari Outbound"
            placeholder="Cari nomor, DO, atau penerima"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                status: v === ALL ? undefined : (v as OutboundStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === "in_progress"
                  ? "Diproses"
                  : v === "completed"
                    ? "Selesai"
                    : v === "cancelled"
                      ? "Dibatalkan"
                      : "Semua status"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            <SelectItem value="in_progress">Diproses</SelectItem>
            <SelectItem value="completed">Selesai</SelectItem>
            <SelectItem value="cancelled">Dibatalkan</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <DataTable
        caption="Daftar Outbound"
        columns={columns}
        rows={list.data?.items}
        getRowId={(o) => o.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada Outbound."
      />
    </div>
  )
}

type DetailTab = "lines" | "packages" | "history"

const PICKING_PRINT = `
@page { size: A4 portrait; margin: 12mm; }
@media print {
  body > *:not(#picking-print-root) { display: none !important; }
  #picking-print-root { display: block !important; }
  html, body { background: #fff !important; }
}`

/** Detail Outbound: picking list per batch, packing koli, QC per baris (HP-friendly), dan Riwayat. */
export function OutboundDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const query = useQuery(outboundQuery(id))
  const queryClient = useQueryClient()
  const [qcLine, setQcLine] = useState<OutboundLine | null>(null)
  const [dropLine, setDropLine] = useState<OutboundLine | null>(null)
  const [packing, setPacking] = useState(false)
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["warehouse"] })
  const pick = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      pickOutbound(id, query.data?.version ?? 0, key),
    onSuccess: async () => {
      toast.success("Picking dicatat")
      await refresh()
    },
    onError: onProblem(),
  })
  const reallocate = useIdempotentMutation({
    mutationFn: (lineId: string, key) =>
      reallocateOutboundLine(id, lineId, key),
    onSuccess: async () => {
      toast.success("Stok pengganti dialokasikan")
      await refresh()
    },
    onError: onProblem(),
  })
  const ob = query.data
  if (query.isError) {
    return (
      <p className="text-sm text-destructive">
        Outbound tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }
  return (
    <LoadingArea loading={query.isPending}>
      {ob ? (
        <div className="flex flex-col gap-4">
          <style>{PICKING_PRINT}</style>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">
                {ob.number}
              </h1>
              <StatusBadge status={ob.status} />
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                className="min-h-11"
                onClick={() => window.print()}
              >
                <PrinterIcon /> Cetak Picking List
              </Button>
              {ob.allowedActions.includes("pick") ? (
                <LoadingButton
                  className="min-h-11"
                  loading={pick.isPending}
                  onClick={() => pick.mutate()}
                >
                  Konfirmasi Picking
                </LoadingButton>
              ) : null}
              {ob.allowedActions.includes("pack") ? (
                <Button
                  className="min-h-11"
                  variant={
                    ob.needsRepack || ob.packages.length === 0
                      ? "default"
                      : "outline"
                  }
                  onClick={() => setPacking(true)}
                >
                  {ob.packages.length > 0
                    ? "Packing Ulang"
                    : "Packing & Catat Koli"}
                </Button>
              ) : null}
            </div>
          </div>
          {ob.needsRepack ? (
            <p className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200">
              Isi barang berubah setelah QC (stok pengganti). Catat ulang koli
              sebelum Outbound bisa diselesaikan.
            </p>
          ) : null}
          {ob.heldCount > 0 ? (
            <p className="rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200">
              {ob.heldCount} baris ditahan karena stok pengganti kurang.
              Sales/CS sudah mendapat notifikasi. Alokasi ulang setelah stok
              masuk, atau kirim sebagian atas keputusan user.
            </p>
          ) : null}
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info
              label="Dokumen Asal"
              value={`${ob.sourceTypeLabel} ${ob.sourceLabel}`}
            />
            <Info label="Tanggal" value={formatDate(ob.businessDate)} />
            <Info label="Lokasi" value={locationLabel(ob.location)} />
            <Info label="Penerima" value={ob.recipientName} />
            <Info label="Disiapkan oleh" value={ob.preparedByName} />
            <Info
              label="Packing"
              value={ob.packedAt ? formatDateTime(ob.packedAt) : ""}
            />
            <Info
              label="Koli"
              value={ob.packages.length ? `${ob.packages.length} koli` : ""}
            />
            <Info
              label="Selesai"
              value={ob.completedAt ? formatDateTime(ob.completedAt) : ""}
            />
          </dl>
          <PageTabs
            label="Bagian Outbound"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "lines", label: "Picking & QC" },
              { id: "packages", label: "Koli" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "lines" ? (
            <OutboundLines
              outbound={ob}
              onQc={setQcLine}
              onDrop={setDropLine}
              onReallocate={(l) => reallocate.mutate(l.id)}
              reallocating={reallocate.isPending}
            />
          ) : tab === "packages" ? (
            <PackagesView outbound={ob} />
          ) : (
            <EntityHistory entityType="outbound" entityId={ob.id} />
          )}
          {qcLine ? (
            <QcDialog
              outbound={ob}
              line={qcLine}
              onClose={() => setQcLine(null)}
            />
          ) : null}
          {dropLine ? (
            <DropDialog
              outbound={ob}
              line={dropLine}
              onClose={() => setDropLine(null)}
            />
          ) : null}
          {packing ? (
            <PackDialog outbound={ob} onClose={() => setPacking(false)} />
          ) : null}
          {createPortal(
            <div id="picking-print-root" className="hidden">
              <PickingList outbound={ob} />
            </div>,
            document.body
          )}
        </div>
      ) : null}
    </LoadingArea>
  )
}

function PickingList({ outbound }: { outbound: Outbound }) {
  const lines = outbound.lines.filter(
    (l) => l.status === "pending" || l.status === "picked"
  )
  return (
    <div className="bg-white p-4 text-sm text-black">
      <h1 className="text-lg font-semibold">Picking List {outbound.number}</h1>
      <p>
        {outbound.sourceTypeLabel} {outbound.sourceLabel} ·{" "}
        {locationLabel(outbound.location)} · {formatDate(outbound.businessDate)}
      </p>
      <p>Penerima: {outbound.recipientName}</p>
      <table className="mt-3 w-full border-collapse">
        <thead>
          <tr>
            {[
              "No",
              "SKU",
              "Produk",
              "Batch",
              "Kedaluwarsa",
              "Qty",
              "Diambil",
            ].map((h) => (
              <th key={h} className="border px-2 py-1 text-left">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {lines.map((l) => (
            <tr key={l.id}>
              <td className="border px-2 py-1">{l.lineNo}</td>
              <td className="border px-2 py-1">{l.product.sku}</td>
              <td className="border px-2 py-1">{l.product.name}</td>
              <td className="border px-2 py-1">{batchLabel(l.batch)}</td>
              <td className="border px-2 py-1">
                {l.batch?.expiryDate ? formatDate(l.batch.expiryDate) : "—"}
              </td>
              <td className="border px-2 py-1 text-right">
                {Number(l.allocatedQty)} {l.product.uom}
              </td>
              <td className="border px-2 py-1" />
            </tr>
          ))}
        </tbody>
      </table>
      <div className="mt-8 grid grid-cols-2 gap-8">
        <div>
          Disiapkan (Staf Gudang)
          <div className="mt-12 border-t" />
        </div>
        <div>
          Dicek ulang (QC)
          <div className="mt-12 border-t" />
        </div>
      </div>
    </div>
  )
}

function OutboundLines({
  outbound,
  onQc,
  onDrop,
  onReallocate,
  reallocating,
}: {
  outbound: Outbound
  onQc: (l: OutboundLine) => void
  onDrop: (l: OutboundLine) => void
  onReallocate: (l: OutboundLine) => void
  reallocating: boolean
}) {
  return (
    <div className="flex flex-col gap-4">
      {/* HP: kartu per baris dengan tombol besar; layar lebar: tabel. */}
      <ul className="flex flex-col gap-2 md:hidden" aria-label="Baris Outbound">
        {outbound.lines.map((l) => (
          <li key={l.id} className="flex flex-col gap-2 rounded-lg border p-3">
            <div className="flex items-start justify-between gap-2">
              <div>
                <div className="font-medium">
                  {l.lineNo}. {l.product.sku}
                </div>
                <div className="text-xs text-muted-foreground">
                  {l.product.name}
                </div>
              </div>
              <OutboundLineBadge status={l.status} />
            </div>
            <div className="text-sm">
              Batch {batchLabel(l.batch)}
              {l.batch?.expiryDate
                ? ` · ED ${formatDate(l.batch.expiryDate)}`
                : ""}{" "}
              · {Number(l.allocatedQty)} {l.product.uom}
            </div>
            {l.rejectedQty !== "0.000" ? (
              <div className="text-xs text-destructive">
                Ditolak {Number(l.rejectedQty)} — {l.rejectReason}
                {l.returnLabel ? ` · Retur ${l.returnLabel}` : ""}
              </div>
            ) : null}
            <LineActions
              line={l}
              onQc={onQc}
              onDrop={onDrop}
              onReallocate={onReallocate}
              reallocating={reallocating}
            />
          </li>
        ))}
      </ul>
      <div className="hidden overflow-x-auto rounded-lg border md:block">
        <Table className="min-w-[60rem]">
          <TableHeader>
            <TableRow>
              <TableHead>No</TableHead>
              <TableHead>Produk</TableHead>
              <TableHead>Batch</TableHead>
              <TableHead className="text-right">Qty</TableHead>
              <TableHead className="text-right">Lulus</TableHead>
              <TableHead className="text-right">Ditolak</TableHead>
              <TableHead>Status</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {outbound.lines.map((l) => (
              <TableRow key={l.id}>
                <TableCell>{l.lineNo}</TableCell>
                <TableCell>
                  <div className="flex flex-col">
                    <span className="font-medium">{l.product.sku}</span>
                    <span className="text-xs text-muted-foreground">
                      {l.product.name}
                    </span>
                    {l.replacementOfLineId ? (
                      <span className="text-xs text-muted-foreground">
                        Pengganti QC
                      </span>
                    ) : null}
                  </div>
                </TableCell>
                <TableCell>
                  {batchLabel(l.batch)}
                  {l.batch?.expiryDate ? (
                    <span className="block text-xs text-muted-foreground">
                      ED {formatDate(l.batch.expiryDate)}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {l.allocatedQty}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {l.passedQty}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {l.rejectedQty}
                  {l.rejectReason ? (
                    <span className="block text-xs text-muted-foreground">
                      {l.rejectReason}
                    </span>
                  ) : null}
                  {l.returnLabel ? (
                    <span className="block text-xs text-muted-foreground">
                      Retur {l.returnLabel}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell>
                  <OutboundLineBadge status={l.status} />
                </TableCell>
                <TableCell>
                  <LineActions
                    line={l}
                    onQc={onQc}
                    onDrop={onDrop}
                    onReallocate={onReallocate}
                    reallocating={reallocating}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      {outbound.checks.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h2 className="font-medium">Catatan QC</h2>
          <ul className="flex flex-col gap-1 text-sm">
            {outbound.checks.map((c) => (
              <li key={c.id}>
                {c.result === "passed" ? "Lulus" : "Ditolak"} {Number(c.qty)} ·{" "}
                {c.checkedByName} · {formatDateTime(c.checkedAt)}
                {c.note ? ` — ${c.note}` : ""}
                {c.photoFileIds.length
                  ? ` · ${c.photoFileIds.length} foto`
                  : ""}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  )
}

function LineActions({
  line,
  onQc,
  onDrop,
  onReallocate,
  reallocating,
}: {
  line: OutboundLine
  onQc: (l: OutboundLine) => void
  onDrop: (l: OutboundLine) => void
  onReallocate: (l: OutboundLine) => void
  reallocating: boolean
}) {
  if (line.allowedActions.length === 0) {
    return line.checkedAt ? (
      <span className="text-xs whitespace-nowrap text-muted-foreground">
        {line.checkedByName} · {formatDateTime(line.checkedAt)}
      </span>
    ) : null
  }
  return (
    <div className="flex flex-wrap gap-2">
      {line.allowedActions.includes("qc") ? (
        <Button
          size="sm"
          className="min-h-11 flex-1 md:flex-none"
          onClick={() => onQc(line)}
        >
          QC
        </Button>
      ) : null}
      {line.allowedActions.includes("reallocate") ? (
        <LoadingButton
          size="sm"
          variant="outline"
          className="min-h-11"
          loading={reallocating}
          onClick={() => onReallocate(line)}
        >
          Alokasi Ulang
        </LoadingButton>
      ) : null}
      {line.allowedActions.includes("drop") ? (
        <Button
          size="sm"
          variant="ghost"
          className="min-h-11"
          onClick={() => onDrop(line)}
        >
          Kirim Sebagian
        </Button>
      ) : null}
    </div>
  )
}

function PackagesView({ outbound }: { outbound: Outbound }) {
  if (outbound.packages.length === 0) {
    return <p className="text-sm text-muted-foreground">Koli belum dicatat.</p>
  }
  return (
    <ul className="grid gap-3 sm:grid-cols-2">
      {outbound.packages.map((p) => (
        <li key={p.packageNumber} className="rounded-lg border p-3 text-sm">
          <div className="font-medium">
            Koli {p.packageNumber}/{p.packageCount}
            {p.weightGrams
              ? ` · ${(p.weightGrams / 1000).toLocaleString("id-ID")} kg`
              : ""}
          </div>
          <ul className="mt-1 text-muted-foreground">
            {p.items.map((i) => (
              <li key={i.lineId}>
                {i.product.sku} · {batchLabel(i.batch)} × {Number(i.qty)}
              </li>
            ))}
          </ul>
        </li>
      ))}
    </ul>
  )
}

function PackDialog({
  outbound,
  onClose,
}: {
  outbound: Outbound
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const lines = packableLines(outbound)
  const [count, setCount] = useState(
    String(Math.max(outbound.packages.length, 1))
  )
  const [weights, setWeights] = useState<Record<number, string>>({})
  const n = Math.max(1, Math.min(Number(count) || 1, Math.max(lines.length, 1)))
  const packages = distributePackages(
    lines.map((l) => ({ lineId: l.line.id, qty: l.qty })),
    n
  ).map((p, i) =>
    weights[i]
      ? { ...p, weightGrams: Math.round(Number(weights[i]) * 1000) }
      : p
  )
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      packOutbound(outbound.id, outbound.version, packages, key),
    onSuccess: async () => {
      toast.success("Koli dicatat")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
    },
    onError: onProblem(),
  })
  const lineOf = (id: string) => lines.find((l) => l.line.id === id)?.line
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Packing & Koli</DialogTitle>
          <DialogDescription>
            Semua barang yang diambil masuk koli. Setiap baris masuk utuh ke
            satu koli; label koli dicetak dari Logistik › Label Koli.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            save.mutate()
          }}
        >
          <Field>
            <FieldLabel htmlFor="pack-count">Jumlah koli</FieldLabel>
            <Input
              id="pack-count"
              type="number"
              inputMode="numeric"
              min={1}
              max={Math.max(lines.length, 1)}
              className="min-h-11"
              value={count}
              onChange={(e) => setCount(e.target.value)}
            />
          </Field>
          {packages.map((p, i) => (
            <div key={i} className="rounded-md border p-3 text-sm">
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">
                  Koli {i + 1}/{packages.length}
                </span>
                <Input
                  aria-label={`Berat koli ${i + 1} (kg)`}
                  placeholder="Berat (kg)"
                  inputMode="decimal"
                  className="min-h-11 w-32"
                  value={weights[i] ?? ""}
                  onChange={(e) =>
                    setWeights({ ...weights, [i]: e.target.value })
                  }
                />
              </div>
              <ul className="mt-1 text-muted-foreground">
                {p.items.map((it) => {
                  const l = lineOf(it.lineId)
                  return (
                    <li key={it.lineId}>
                      {l?.product.sku} · {batchLabel(l?.batch)} ×{" "}
                      {Number(it.qty)}
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              className="min-h-11"
            >
              Simpan Koli
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

const CHECKS = [
  { key: "batch", label: "Batch sesuai" },
  { key: "expiry", label: "Tanggal kedaluwarsa sesuai" },
  { key: "condition", label: "Kondisi barang baik" },
  { key: "seal", label: "Segel utuh" },
] as const

type CheckKey = (typeof CHECKS)[number]["key"]

function QcDialog({
  outbound,
  line,
  onClose,
}: {
  outbound: Outbound
  line: OutboundLine
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [passed, setPassed] = useState(line.pickedQty)
  const [rejected, setRejected] = useState("0")
  const [checks, setChecks] = useState<Record<CheckKey, boolean>>({
    batch: true,
    expiry: true,
    condition: true,
    seal: true,
  })
  const [reason, setReason] = useState("")
  const [note, setNote] = useState("")
  const [photos, setPhotos] = useState<string[]>([])
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      recordOutboundQc(
        outbound.id,
        line.id,
        {
          passedQty: passed || "0",
          rejectedQty: rejected || "0",
          checks,
          rejectReason: reason || undefined,
          note: note || undefined,
          photoFileIds: photos,
        },
        key
      ),
    onSuccess: async (o) => {
      toast.success(
        o.heldCount > 0
          ? "QC disimpan; stok pengganti kurang, baris ditahan"
          : "Hasil QC disimpan"
      )
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            QC {line.product.sku} · {batchLabel(line.batch)}
          </DialogTitle>
          <DialogDescription>
            Qty diambil {Number(line.pickedQty)} {line.product.uom}. Barang
            ditolak masuk Karantina dan stok pengganti FEFO diambil otomatis.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setErrors({})
            save.mutate()
          }}
        >
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-1 text-sm font-medium">Checklist QC</legend>
            {CHECKS.map((c) => (
              <label
                key={c.key}
                className="flex min-h-11 items-center gap-3 text-sm"
              >
                <input
                  type="checkbox"
                  className="size-5"
                  checked={checks[c.key]}
                  onChange={(e) =>
                    setChecks({ ...checks, [c.key]: e.target.checked })
                  }
                />
                {c.label}
              </label>
            ))}
            {errors.checks ? <FieldError>{errors.checks}</FieldError> : null}
          </fieldset>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="oqc-passed">Qty lulus</FieldLabel>
              <QuantityInput
                id="oqc-passed"
                value={passed}
                unit={line.product.uom}
                onValueChange={setPassed}
                className="min-h-11"
              />
              {errors.passedQty ? (
                <FieldError>{errors.passedQty}</FieldError>
              ) : null}
            </Field>
            <Field>
              <FieldLabel htmlFor="oqc-rejected">Qty ditolak</FieldLabel>
              <QuantityInput
                id="oqc-rejected"
                value={rejected}
                unit={line.product.uom}
                onValueChange={setRejected}
                className="min-h-11"
              />
              {errors.rejectedQty ? (
                <FieldError>{errors.rejectedQty}</FieldError>
              ) : null}
            </Field>
          </div>
          {Number(rejected) > 0 ? (
            <Field>
              <FieldLabel htmlFor="oqc-reason">Alasan penolakan</FieldLabel>
              <Textarea
                id="oqc-reason"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
              {errors.rejectReason ? (
                <FieldError>{errors.rejectReason}</FieldError>
              ) : null}
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="oqc-note">Catatan</FieldLabel>
            <Textarea
              id="oqc-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>Foto ({photos.length}/5)</FieldLabel>
            <FileUpload
              label="Ambil Foto"
              accept="image/*"
              maxSizeBytes={MAX_PHOTO_BYTES}
              disabled={photos.length >= 5}
              requestUpload={(f) => requestOutboundPhoto(f)}
              onUploaded={(fileId) => setPhotos((p) => [...p, fileId])}
              onError={(m) => toast.error(m)}
            />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              className="min-h-11"
            >
              Simpan Hasil QC
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function DropDialog({
  outbound,
  line,
  onClose,
}: {
  outbound: Outbound
  line: OutboundLine
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      dropOutboundLine(outbound.id, line.id, reason, key),
    onSuccess: async () => {
      toast.success("Baris tidak ikut dikirim")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Kirim Sebagian</DialogTitle>
          <DialogDescription>
            {Number(line.allocatedQty)} {line.product.uom} {line.product.sku}{" "}
            tidak ikut dikirim dengan dokumen ini. Sisa pesanan tetap terbuka
            untuk DO berikutnya.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="drop-reason">Alasan / keputusan user</FieldLabel>
          <Textarea
            id="drop-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          {errors.reason ? <FieldError>{errors.reason}</FieldError> : null}
        </Field>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            className="min-h-11"
            loading={save.isPending}
            onClick={() => save.mutate()}
          >
            Kirim Sebagian
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

async function requestOutboundPhoto(file: File) {
  const target = await unwrap(
    api.POST("/api/v1/files", {
      params: { header: { "Idempotency-Key": crypto.randomUUID() } },
      body: {
        ownerType: "outbound_quality_check",
        fileName: file.name,
        contentType: file.type || "image/jpeg",
        sizeBytes: file.size,
      },
    })
  )
  return { uploadUrl: target.uploadUrl, fileId: target.fileId }
}
