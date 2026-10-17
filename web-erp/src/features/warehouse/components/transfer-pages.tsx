import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, SearchIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
import { StatusBadge } from "@/components/common/status-badge"
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
import { usePermissions } from "@/features/auth/api/auth-api"
import type { Product } from "@/features/catalog/api/catalog-api"
import { ProductPicker } from "@/features/catalog/components/product-picker"
import {
  locationsQuery,
  type Location,
} from "@/features/warehouse/api/warehouse-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  actOnTransfer,
  batchLabel,
  createTransfer,
  inTransitQuery,
  locationLabel,
  TRANSFER_ACTION_LABELS,
  transferQuery,
  transfersQuery,
  type StockInTransitLine,
  type StockTransfer,
  type StockTransferAction,
  type StockTransferStatus,
  type StockTransferSummary,
  type StockTransferType,
  type TransferSearch,
} from "../api/documents-api"
import { Info } from "./document-parts"

const ALL = "all"

const TEXT: Record<
  StockTransferType,
  {
    title: string
    description: string
    create: string
    base: "/warehouse/stock-transfers" | "/warehouse/petty-stock-requests"
  }
> = {
  direct: {
    title: "Transfer Stok",
    description:
      "Antar lokasi dalam satu gudang langsung diposting. Antar gudang, ke maklon, atau ke mitra berstatus Dalam Pengiriman sampai diterima.",
    create: "Buat Transfer Stok",
    base: "/warehouse/stock-transfers",
  },
  request: {
    title: "Permintaan Petty Stock",
    description:
      "Fulfillment Leader meminta FG Non-Ecommerce → Petty Stock; Logistik Leader me-release, Staf Gudang menyiapkan, Fulfillment Leader mengonfirmasi qty diterima.",
    create: "Buat Permintaan Petty Stock",
    base: "/warehouse/petty-stock-requests",
  },
}

const STATUS_OPTIONS: Record<StockTransferType, StockTransferStatus[]> = {
  direct: [
    "draft",
    "in_transit",
    "partially_received",
    "received",
    "completed",
    "cancelled",
  ],
  request: ["draft", "submitted", "in_progress", "completed", "cancelled"],
}

const STATUS_TEXT: Record<StockTransferStatus, string> = {
  draft: "Draft",
  submitted: "Diajukan",
  in_progress: "Diproses",
  in_transit: "Dalam Pengiriman",
  partially_received: "Diterima Sebagian",
  received: "Diterima",
  completed: "Selesai",
  cancelled: "Dibatalkan",
}

/** Daftar Transfer Stok / Permintaan Petty Stock (S-216, S-226). */
export function TransferListPage({
  type,
  search,
  onSearchChange,
}: {
  type: StockTransferType
  search: TransferSearch
  onSearchChange: (patch: Partial<TransferSearch>) => void
}) {
  const { can } = usePermissions()
  const list = useQuery(transfersQuery(type, search))
  const [q, setQ] = useState(search.q ?? "")
  const [formOpen, setFormOpen] = useState(false)
  const text = TEXT[type]
  const columns: DataTableColumn<StockTransferSummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (t) => (
        <Link
          to={`${text.base}/$id`}
          params={{ id: t.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {t.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (t) => (
        <span className="whitespace-nowrap">{formatDate(t.businessDate)}</span>
      ),
    },
    { id: "from", header: "Dari", cell: (t) => locationLabel(t.from) },
    { id: "to", header: "Ke", cell: (t) => locationLabel(t.to) },
    {
      id: "qty",
      header: "Total Qty",
      align: "right",
      cell: (t) => `${t.totalQty} (${t.lineCount} baris)`,
    },
    { id: "by", header: "Dibuat oleh", cell: (t) => t.requestedByName },
    {
      id: "status",
      header: "Status",
      cell: (t) => <StatusBadge status={t.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {text.title}
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            {text.description}
          </p>
        </div>
        {can("warehouse.stock_transfer.write") ? (
          <Button className="min-h-11" onClick={() => setFormOpen(true)}>
            <PlusIcon /> {text.create}
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange({ q: q || undefined, page: 1 })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari nomor"
            placeholder="Cari nomor atau catatan"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            onSearchChange({
              status: v === ALL ? undefined : (v as StockTransferStatus),
              page: 1,
            })
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua status"
                  : STATUS_TEXT[v as StockTransferStatus]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {STATUS_OPTIONS[type].map((s) => (
              <SelectItem key={s} value={s}>
                {STATUS_TEXT[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        caption={text.title}
        columns={columns}
        rows={list.data?.items}
        getRowId={(t) => t.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange({ page })}
        emptyMessage="Belum ada dokumen."
      />
      {formOpen ? (
        <TransferFormDialog type={type} onClose={() => setFormOpen(false)} />
      ) : null}
    </div>
  )
}

type FormLine = { key: string; product: Product | null; qty: string }

function LocationSelect({
  id,
  label,
  value,
  onChange,
  options,
  error,
}: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  options: Location[]
  error?: string
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select
        value={value || null}
        onValueChange={(v) => onChange(String(v ?? ""))}
      >
        <SelectTrigger id={id} className="min-h-11 w-full">
          <SelectValue>
            {(v: string | null) => {
              const l = options.find((x) => x.id === v)
              return l ? `${l.code} · ${l.name}` : "Pilih lokasi"
            }}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {options.map((l) => (
            <SelectItem key={l.id} value={l.id}>
              {l.code} · {l.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {error ? <FieldError>{error}</FieldError> : null}
    </Field>
  )
}

function TransferFormDialog({
  type,
  onClose,
}: {
  type: StockTransferType
  onClose: () => void
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const locations = useQuery(locationsQuery({ page: 1 }))
  const all = (locations.data?.items ?? []).filter(
    (l) => l.isActive && !l.deletedAt && l.locationType !== "inbound"
  )
  const fromOptions =
    type === "request"
      ? all.filter((l) => l.locationType === "finished_goods")
      : all
  const toOptions =
    type === "request"
      ? all.filter((l) => l.locationType === "petty_stock")
      : all
  const [from, setFrom] = useState("")
  const [to, setTo] = useState("")
  const [date, setDate] = useState(todayBusinessDate())
  const [note, setNote] = useState("")
  const [lines, setLines] = useState<FormLine[]>([
    { key: crypto.randomUUID(), product: null, qty: "" },
  ])
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createTransfer(
        {
          transferType: type,
          fromLocationId: from,
          toLocationId: to,
          businessDate: date,
          note: note || undefined,
          lines: lines.flatMap((l) =>
            l.product ? [{ productId: l.product.id, qty: l.qty || "0" }] : []
          ),
        },
        key
      ),
    onSuccess: async (t) => {
      toast.success("Draft disimpan")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
      void navigate({ to: `${TEXT[type].base}/$id`, params: { id: t.id } })
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setErrors(p.fields)
      } else {
        showErrorDialog(p)
      }
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{TEXT[type].create}</DialogTitle>
          <DialogDescription>
            Batch dialokasikan FEFO saat diposting, dikirim, atau di-release.
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
          <div className="grid gap-3 sm:grid-cols-2">
            <LocationSelect
              id="trf-from"
              label="Dari lokasi"
              value={from}
              onChange={setFrom}
              options={fromOptions}
              error={errors.fromLocationId}
            />
            <LocationSelect
              id="trf-to"
              label="Ke lokasi"
              value={to}
              onChange={setTo}
              options={toOptions}
              error={errors.toLocationId}
            />
            <Field>
              <FieldLabel htmlFor="trf-date">Tanggal</FieldLabel>
              <DateInput id="trf-date" value={date} onValueChange={setDate} />
              {errors.businessDate ? (
                <FieldError>{errors.businessDate}</FieldError>
              ) : null}
            </Field>
          </div>
          <div className="flex flex-col gap-2">
            <span className="text-sm font-medium">Baris</span>
            {lines.map((l, i) => (
              <div key={l.key} className="flex flex-wrap items-start gap-2">
                <div className="min-w-0 flex-1">
                  <ProductPicker
                    value={l.product}
                    isBundle={false}
                    types={type === "request" ? ["finished_good"] : undefined}
                    onValueChange={(p) =>
                      setLines(
                        lines.map((x) =>
                          x.key === l.key ? { ...x, product: p } : x
                        )
                      )
                    }
                  />
                  {errors[`lines[${i}].productId`] ? (
                    <FieldError>{errors[`lines[${i}].productId`]}</FieldError>
                  ) : null}
                </div>
                <div className="w-36">
                  <QuantityInput
                    aria-label={`Qty baris ${i + 1}`}
                    value={l.qty}
                    unit={l.product?.uom}
                    className="min-h-11"
                    onValueChange={(v) =>
                      setLines(
                        lines.map((x) =>
                          x.key === l.key ? { ...x, qty: v } : x
                        )
                      )
                    }
                  />
                  {errors[`lines[${i}].qty`] ? (
                    <FieldError>{errors[`lines[${i}].qty`]}</FieldError>
                  ) : null}
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-11"
                  aria-label={`Hapus baris ${i + 1}`}
                  disabled={lines.length === 1}
                  onClick={() => setLines(lines.filter((x) => x.key !== l.key))}
                >
                  <Trash2Icon />
                </Button>
              </div>
            ))}
            {errors.lines ? <FieldError>{errors.lines}</FieldError> : null}
            <Button
              type="button"
              variant="outline"
              className="min-h-11 self-start"
              onClick={() =>
                setLines([
                  ...lines,
                  { key: crypto.randomUUID(), product: null, qty: "" },
                ])
              }
            >
              <PlusIcon /> Tambah baris
            </Button>
          </div>
          <Field>
            <FieldLabel htmlFor="trf-note">Catatan</FieldLabel>
            <Textarea
              id="trf-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
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
              Simpan Draft
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

type DetailTab = "lines" | "history"

/** Detail Transfer Stok / Permintaan Petty Stock: baris per batch, aksi sesuai allowedActions, Riwayat. */
export function TransferDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const transfer = useQuery(transferQuery(id))
  const t = transfer.data
  if (transfer.isError) {
    return (
      <p className="text-sm text-destructive">
        Dokumen tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }
  return (
    <LoadingArea loading={transfer.isPending}>
      {t ? (
        <div className="flex flex-col gap-4">
          <TransferHeader transfer={t} />
          <PageTabs
            label="Bagian transfer"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "lines", label: "Baris" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "lines" ? (
            <TransferLines transfer={t} />
          ) : (
            <EntityHistory entityType="stock_transfer" entityId={t.id} />
          )}
        </div>
      ) : null}
    </LoadingArea>
  )
}

const PRIMARY_ACTIONS: StockTransferAction[] = [
  "submit",
  "release",
  "prepare",
  "post",
  "ship",
  "receive",
  "complete",
]

function TransferHeader({ transfer: t }: { transfer: StockTransfer }) {
  const queryClient = useQueryClient()
  const [receiving, setReceiving] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const act = useIdempotentMutation({
    mutationFn: (action: StockTransferAction, key) =>
      actOnTransfer(t.id, { action, version: t.version }, key),
    onSuccess: async () => {
      toast.success("Dokumen diperbarui")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const title =
    t.number ??
    (t.transferType === "request"
      ? "Draft Permintaan Petty Stock"
      : "Draft Transfer Stok")
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          <StatusBadge status={t.status} />
        </div>
        <div className="flex flex-wrap gap-2">
          {t.allowedActions.includes("cancel") ? (
            <Button
              variant="outline"
              className="min-h-11"
              onClick={() => setCancelling(true)}
            >
              Batalkan
            </Button>
          ) : null}
          {PRIMARY_ACTIONS.filter((a) => t.allowedActions.includes(a)).map(
            (a) =>
              a === "receive" ? (
                <Button
                  key={a}
                  className="min-h-11"
                  onClick={() => setReceiving(true)}
                >
                  {TRANSFER_ACTION_LABELS[a]}
                </Button>
              ) : (
                <LoadingButton
                  key={a}
                  className="min-h-11"
                  loading={act.isPending && act.variables === a}
                  onClick={() => act.mutate(a)}
                >
                  {TRANSFER_ACTION_LABELS[a]}
                </LoadingButton>
              )
          )}
        </div>
      </div>
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <Info label="Dari" value={locationLabel(t.from)} />
        <Info label="Ke" value={locationLabel(t.to)} />
        <Info label="Tanggal" value={formatDate(t.businessDate)} />
        <Info
          label="Pengiriman"
          value={
            t.requiresShipping
              ? "Dalam Pengiriman sampai diterima"
              : "Langsung (satu gudang)"
          }
        />
        <Info label="Dibuat oleh" value={t.requestedByName} />
        {t.releasedAt ? (
          <Info
            label="Release"
            value={`${t.releasedByName} · ${formatDateTime(t.releasedAt)}`}
          />
        ) : null}
        {t.preparedAt ? (
          <Info
            label="Disiapkan"
            value={`${t.preparedByName} · ${formatDateTime(t.preparedAt)}`}
          />
        ) : null}
        {t.shippedAt ? (
          <Info
            label="Dikirim"
            value={`${t.shippedByName} · ${formatDateTime(t.shippedAt)}`}
          />
        ) : null}
        {t.receivedAt ? (
          <Info
            label="Diterima"
            value={`${t.receivedByName} · ${formatDateTime(t.receivedAt)}`}
          />
        ) : null}
        {t.cancelReason ? (
          <Info label="Alasan batal" value={t.cancelReason} />
        ) : null}
        {t.note ? <Info label="Catatan" value={t.note} /> : null}
      </dl>
      {receiving ? (
        <ReceiveDialog transfer={t} onClose={() => setReceiving(false)} />
      ) : null}
      {cancelling ? (
        <CancelDialog transfer={t} onClose={() => setCancelling(false)} />
      ) : null}
    </div>
  )
}

function TransferLines({ transfer: t }: { transfer: StockTransfer }) {
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[48rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Produk</TableHead>
            <TableHead>Batch</TableHead>
            <TableHead className="text-right">Qty</TableHead>
            <TableHead className="text-right">Diterima</TableHead>
            <TableHead className="text-right">Kekurangan</TableHead>
            <TableHead className="text-right">Nilai (FIFO)</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {t.lines.map((l) => (
            <TableRow key={l.id}>
              <TableCell>
                <span className="font-medium">{l.product.sku}</span>{" "}
                {l.product.name}
              </TableCell>
              <TableCell>
                {l.batch ? batchLabel(l.batch) : "FEFO otomatis"}
              </TableCell>
              <TableCell className="text-right tabular-nums">{l.qty}</TableCell>
              <TableCell className="text-right tabular-nums">
                {l.receivedQty}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {l.shortageQty}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {Number(l.costAmount) ? formatRupiah(l.costAmount) : "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function ReceiveDialog({
  transfer: t,
  onClose,
}: {
  transfer: StockTransfer
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const outstanding = (l: StockTransfer["lines"][number]) =>
    t.transferType === "request"
      ? l.qty
      : String(Number(l.qty) - Number(l.receivedQty) - Number(l.shortageQty))
  const [qty, setQty] = useState<Record<string, string>>(() =>
    Object.fromEntries(t.lines.map((l) => [l.id, outstanding(l)]))
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      actOnTransfer(
        t.id,
        {
          action: "receive",
          version: t.version,
          lines: t.lines.map((l) => ({ lineId: l.id, qty: qty[l.id] || "0" })),
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Penerimaan dicatat")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setErrors(p.fields)
      } else {
        showErrorDialog(p)
      }
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Konfirmasi Terima</DialogTitle>
          <DialogDescription>
            {t.transferType === "request"
              ? "Cek ulang barang lalu isi qty yang benar-benar diterima. Kekurangan tetap di lokasi asal."
              : "Isi qty yang diterima. Sisa tetap Dalam Pengiriman sampai diterima atau transfer diselesaikan."}
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            setErrors({})
            save.mutate()
          }}
        >
          {t.lines.map((l, i) => (
            <Field key={l.id}>
              <FieldLabel htmlFor={`rcv-${l.id}`}>
                {l.product.sku} · {l.batch ? batchLabel(l.batch) : "FEFO"}{" "}
                (maks. {outstanding(l)})
              </FieldLabel>
              <QuantityInput
                id={`rcv-${l.id}`}
                value={qty[l.id] ?? ""}
                unit={l.product.uom}
                className="min-h-11"
                onValueChange={(v) => setQty({ ...qty, [l.id]: v })}
              />
              {errors[`lines[${i}].qty`] ? (
                <FieldError>{errors[`lines[${i}].qty`]}</FieldError>
              ) : null}
            </Field>
          ))}
          {errors.lines ? <FieldError>{errors.lines}</FieldError> : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              className="min-h-11"
            >
              Konfirmasi Terima
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CancelDialog({
  transfer: t,
  onClose,
}: {
  transfer: StockTransfer
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [reason, setReason] = useState("")
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      actOnTransfer(
        t.id,
        { action: "cancel", version: t.version, reason },
        key
      ),
    onSuccess: async () => {
      toast.success("Dokumen dibatalkan")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Batalkan dokumen?</DialogTitle>
          <DialogDescription>
            Dokumen yang dibatalkan tidak bisa diproses lagi.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="cancel-reason">Alasan</FieldLabel>
          <Textarea
            id="cancel-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </Field>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Kembali
          </Button>
          <LoadingButton
            variant="destructive"
            loading={save.isPending}
            onClick={() => save.mutate()}
          >
            Batalkan
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Gudang › Stok dalam Pengiriman: baris transfer antar gudang/eksternal yang belum diterima penuh (WMS-10). */
export function InTransitPage({
  page,
  onPageChange,
}: {
  page: number
  onPageChange: (p: number) => void
}) {
  const list = useQuery(inTransitQuery(page))
  const columns: DataTableColumn<StockInTransitLine>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (l) => (
        <Link
          to="/warehouse/stock-transfers/$id"
          params={{ id: l.transferId }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {l.number}
        </Link>
      ),
    },
    {
      id: "shipped",
      header: "Dikirim",
      cell: (l) =>
        l.shippedAt ? formatDateTime(l.shippedAt) : formatDate(l.businessDate),
    },
    {
      id: "route",
      header: "Dari → Ke",
      cell: (l) => `${l.from.code} → ${l.to.code}`,
    },
    {
      id: "product",
      header: "Produk",
      cell: (l) => `${l.product.sku} ${l.product.name}`,
    },
    { id: "batch", header: "Batch", cell: (l) => batchLabel(l.batch) },
    {
      id: "outstanding",
      header: "Belum Diterima",
      align: "right",
      cell: (l) => `${l.outstandingQty} dari ${l.qty}`,
    },
    {
      id: "value",
      header: "Nilai",
      align: "right",
      cell: (l) => formatRupiah(l.costAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (l) => <StatusBadge status={l.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Stok dalam Pengiriman
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Stok berstatus Dalam Pengiriman di lokasi tujuan sampai penerima
          mengonfirmasi.
        </p>
      </div>
      <DataTable
        caption="Stok dalam Pengiriman"
        columns={columns}
        rows={list.data?.items}
        getRowId={(l) => l.lineId}
        loading={list.isPending}
        fetching={list.isFetching}
        page={page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={onPageChange}
        emptyMessage="Tidak ada stok Dalam Pengiriman."
      />
    </div>
  )
}
