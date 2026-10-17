import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, SearchIcon, SmartphoneIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
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
import { usePermissions } from "@/features/auth/api/auth-api"
import { openFile } from "@/features/party/api/party-api"
import type { DetailTab } from "@/features/procurement/components/request-detail-page"
import {
  fieldErrors,
  Info,
  LineError,
  OptionSelect,
  ReasonDialog,
} from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { returnReasonsQuery } from "../api/sales-api"
import {
  MAX_PROOF_FILE_BYTES,
  ORDER_TYPE_LABELS,
} from "../api/sales-documents-api"
import {
  pickupUsersQuery,
  requestReturnPhotoUpload,
  RETURN_STATUSES,
  returnSourceQuery,
  returnSourcesQuery,
  salesReturnAction,
  salesReturnQuery,
  salesReturnsQuery,
  saveSalesReturn,
  SETTLEMENT_LABELS,
  type ReturnActionInput,
  type ReturnListSearch,
  type SalesReturn,
  type SalesReturnAction,
  type SalesReturnSettlement,
  type SalesReturnSourceDetail,
  type SalesReturnSummary,
} from "../api/sales-returns-api"

const PHOTO_TYPES = "image/jpeg,image/png,image/webp,application/pdf"

function useInvalidateSales() {
  const queryClient = useQueryClient()
  return async () => {
    await queryClient.invalidateQueries({ queryKey: ["sales"] })
    await queryClient.invalidateQueries({ queryKey: ["approval"] })
  }
}

// --- Daftar -------------------------------------------------------------------------------------

function SearchBox({
  initial,
  onSearch,
}: {
  initial?: string
  onSearch: (q: string | undefined) => void
}) {
  const [q, setQ] = useState(initial ?? "")
  return (
    <form
      className="relative w-full sm:w-72"
      onSubmit={(e) => {
        e.preventDefault()
        onSearch(q || undefined)
      }}
    >
      <SearchIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        aria-label="Cari retur"
        placeholder="Cari nomor RJ, SO, pelanggan/mitra"
        className="pl-8"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
    </form>
  )
}

const returnColumns: DataTableColumn<SalesReturnSummary>[] = [
  {
    id: "number",
    header: "Nomor RJ",
    cell: (r) => (
      <Link
        to="/sales/returns/$id"
        params={{ id: r.id }}
        search={{}}
        className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
      >
        {r.number ?? "Draft"}
      </Link>
    ),
  },
  {
    id: "date",
    header: "Tanggal",
    cell: (r) => (
      <span className="whitespace-nowrap">{formatDate(r.returnDate)}</span>
    ),
  },
  {
    id: "order",
    header: "Sales Order",
    cell: (r) => (
      <div className="flex flex-col">
        <span className="whitespace-nowrap">{r.salesOrderNumber}</span>
        <span className="text-xs text-muted-foreground">{r.partyName}</span>
      </div>
    ),
  },
  {
    id: "type",
    header: "Channel",
    cell: (r) => (
      <Badge variant="secondary">{ORDER_TYPE_LABELS[r.orderType]}</Badge>
    ),
  },
  {
    id: "settlement",
    header: "Penyelesaian",
    cell: (r) => SETTLEMENT_LABELS[r.settlementMethod],
  },
  {
    id: "amount",
    header: "Nilai",
    align: "right",
    cell: (r) => formatRupiah(r.returnAmount),
  },
  {
    id: "status",
    header: "Status",
    cell: (r) => <StatusBadge status={r.status} />,
  },
]

/** Penjualan › Retur Penjualan (S-314, S-317). */
export function SalesReturnListPage({
  search,
  onSearchChange,
}: {
  search: ReturnListSearch
  onSearchChange: (u: (s: ReturnListSearch) => ReturnListSearch) => void
}) {
  const { can } = usePermissions()
  const list = useQuery(salesReturnsQuery(search))
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Retur Penjualan
          </h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Retur dari baris Sales Order yang sudah dikirim. Retur MT/reseller
            diverifikasi Logistik Leader lalu diambil Merchandiser bila perlu;
            barang masuk Inbound (Karantina) lalu Gudang Retur. Setelah QC,
            jurnal A-13 dibuat bersama nota kredit atau refund.
          </p>
        </div>
        {can("sales.sales_return.write") ? (
          <Button render={<Link to="/sales/returns/new" />}>
            <PlusIcon /> Buat Retur Penjualan
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <SearchBox
          initial={search.q}
          onSearch={(q) => onSearchChange((s) => ({ ...s, q, page: 1 }))}
        />
        <Select
          value={search.status ?? "all"}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              status:
                v && v !== "all"
                  ? (v as ReturnListSearch["status"])
                  : undefined,
              page: 1,
            }))
          }
        >
          <SelectTrigger className="w-full sm:w-52" aria-label="Status">
            <SelectValue>
              {(v: string) =>
                v === "all" ? "Semua status" : statusLabel(v).label
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua status</SelectItem>
            {RETURN_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {statusLabel(s).label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-5 accent-primary"
            checked={!!search.assignedToMe}
            onChange={(e) =>
              onSearchChange((s) => ({
                ...s,
                assignedToMe: e.target.checked || undefined,
                page: 1,
              }))
            }
          />
          Tugas pickup saya
        </label>
      </div>
      <DataTable
        columns={returnColumns}
        rows={list.data?.items}
        getRowId={(r) => r.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada retur penjualan."
      />
    </div>
  )
}

// --- Form ---------------------------------------------------------------------------------------

type LineDraft = {
  salesOrderLineId: string
  include: boolean
  qty: string
  batchId: string
  returnReasonId: string
  note: string
}

/** Buat Retur Penjualan: pilih Sales Order terkirim lalu baris yang diretur. */
export function SalesReturnCreatePage() {
  const [order, setOrder] = useState<PickerOption | null>(null)
  const source = useQuery({
    ...returnSourceQuery(order?.id ?? ""),
    enabled: !!order,
  })
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Buat Retur Penjualan
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Qty retur tidak boleh melebihi qty yang sudah dikirim dikurangi retur
          sebelumnya. Nilai retur memakai harga jual bersih dan HPP asal
          pengiriman.
        </p>
      </div>
      <Field className="max-w-md">
        <FieldLabel htmlFor="rj-order">Sales Order</FieldLabel>
        <SearchPicker
          id="rj-order"
          value={order}
          onValueChange={setOrder}
          query={returnSourcesQuery}
          toOption={(o) => ({
            id: o.id,
            label: o.number,
            description: `${o.partyName} · ${ORDER_TYPE_LABELS[o.orderType]} · ${formatDate(o.businessDate)}`,
          })}
          placeholder="Pilih Sales Order"
          searchPlaceholder="Cari nomor SO, pelanggan, mitra"
        />
      </Field>
      {order ? (
        <LoadingArea loading={source.isPending}>
          {source.data ? (
            <ReturnForm key={order.id} source={source.data} />
          ) : null}
        </LoadingArea>
      ) : null}
    </div>
  )
}

/** Ubah draft Retur Penjualan. */
export function SalesReturnEditPage({ id }: { id: string }) {
  const ret = useQuery(salesReturnQuery(id))
  const source = useQuery({
    ...returnSourceQuery(ret.data?.salesOrderId ?? ""),
    enabled: !!ret.data,
  })
  return (
    <LoadingArea loading={ret.isPending || source.isPending}>
      {ret.data && source.data ? (
        ret.data.allowedActions.includes("edit") ? (
          <div className="flex flex-col gap-4">
            <h1 className="text-2xl font-semibold tracking-tight">
              Ubah Draft Retur {ret.data.salesOrderNumber}
            </h1>
            <ReturnForm source={source.data} ret={ret.data} />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            Hanya retur berstatus Draft yang bisa diubah.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

function ReturnForm({
  source,
  ret,
}: {
  source: SalesReturnSourceDetail
  ret?: SalesReturn
}) {
  const navigate = useNavigate()
  const invalidate = useInvalidateSales()
  const reasons = useQuery(returnReasonsQuery())
  const reasonOptions = (reasons.data ?? [])
    .filter((r) => r.isActive && r.appliesTo !== "purchase_return")
    .map((r) => ({ id: r.id, code: r.code, name: `${r.code} · ${r.name}` }))
  const marketplace = source.order.orderType === "marketplace"
  const [returnDate, setReturnDate] = useState(
    ret?.returnDate ?? todayBusinessDate()
  )
  const [settlement, setSettlement] = useState<SalesReturnSettlement>(
    ret?.settlementMethod ?? (marketplace ? "marketplace" : "credit_note")
  )
  const [note, setNote] = useState(ret?.note ?? "")
  const [lines, setLines] = useState<LineDraft[]>(() =>
    source.lines.map((l) => {
      const r = ret?.lines.find(
        (x) => x.salesOrderLineId === l.salesOrderLineId
      )
      return {
        salesOrderLineId: l.salesOrderLineId,
        include: !!r,
        qty: r?.qty ?? "",
        batchId: r?.batchId ?? "",
        returnReasonId: r?.returnReason.id ?? "",
        note: r?.note ?? "",
      }
    })
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const included = lines.filter((l) => l.include)
  const update = (id: string, patch: Partial<LineDraft>) =>
    setLines((ls) =>
      ls.map((l) => (l.salesOrderLineId === id ? { ...l, ...patch } : l))
    )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      saveSalesReturn(
        ret,
        source.order.id,
        {
          returnDate,
          settlementMethod: marketplace ? undefined : settlement,
          note,
          lines: included.map((l) => ({
            salesOrderLineId: l.salesOrderLineId,
            qty: l.qty,
            returnReasonId: l.returnReasonId,
            batchId: l.batchId || undefined,
            note: l.note || undefined,
          })),
        },
        key
      ),
    onSuccess: async (saved) => {
      toast.success("Draft retur disimpan")
      await invalidate()
      void navigate({
        to: "/sales/returns/$id",
        params: { id: saved.id },
        search: {},
      })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        const mapped: Record<string, string> = {}
        for (const [k, v] of Object.entries(problem.fields)) {
          const m = /^lines\.(\d+)\.(.+)$/.exec(k)
          if (m && included[Number(m[1])])
            mapped[`${included[Number(m[1])].salesOrderLineId}.${m[2]}`] = v
          else mapped[k] = v
        }
        setErrors(mapped)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    const next: Record<string, string> = {}
    if (!returnDate) next.returnDate = "Tanggal retur wajib diisi"
    if (included.length === 0)
      next.lines = "Pilih minimal satu barang yang diretur"
    included.forEach((l) => {
      if (!l.qty || Number(l.qty) <= 0)
        next[`${l.salesOrderLineId}.qty`] = "Qty harus lebih dari nol"
      if (!l.returnReasonId)
        next[`${l.salesOrderLineId}.returnReasonId`] = "Pilih alasan retur"
    })
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const lineErr = (id: string, f: string) => errors[`${id}.${f}`]

  return (
    <form
      noValidate
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <dl className="grid gap-x-6 gap-y-3 rounded-lg border p-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <Info label="Sales Order" value={source.order.number} />
        <Info label="Pelanggan / Mitra" value={source.order.partyName} />
        <Info label="Invoice" value={source.invoiceNumber ?? "—"} />
        <Info
          label="Alur"
          value={
            source.requiresVerification
              ? "Retur MT/reseller: verifikasi Logistik Leader"
              : "Langsung masuk Inbound saat diajukan"
          }
        />
      </dl>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Field data-invalid={!!errors.returnDate}>
          <FieldLabel htmlFor="rj-date">Tanggal Retur</FieldLabel>
          <DateInput
            id="rj-date"
            value={returnDate}
            onValueChange={setReturnDate}
            aria-invalid={!!errors.returnDate}
          />
          <FieldError errors={fieldErrors(errors.returnDate)} />
        </Field>
        <Field data-invalid={!!errors.settlementMethod}>
          <FieldLabel htmlFor="rj-settlement">Penyelesaian</FieldLabel>
          {marketplace ? (
            <p className="text-sm text-muted-foreground">
              {SETTLEMENT_LABELS.marketplace}
            </p>
          ) : (
            <Select
              value={settlement}
              onValueChange={(v) =>
                setSettlement((v ?? "credit_note") as SalesReturnSettlement)
              }
            >
              <SelectTrigger id="rj-settlement" className="w-full">
                <SelectValue>
                  {(v: SalesReturnSettlement) => SETTLEMENT_LABELS[v]}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="credit_note">
                  Nota kredit (potong piutang)
                </SelectItem>
                <SelectItem value="refund">Refund (uang kembali)</SelectItem>
              </SelectContent>
            </Select>
          )}
          <FieldError errors={fieldErrors(errors.settlementMethod)} />
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor="rj-note">Catatan</FieldLabel>
        <Textarea
          id="rj-note"
          rows={2}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">Barang yang Diretur</h2>
        {errors.lines ? (
          <p className="text-sm text-destructive">{errors.lines}</p>
        ) : null}
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[64rem]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-12" />
                <TableHead>Produk</TableHead>
                <TableHead className="w-28 text-right">Terkirim</TableHead>
                <TableHead className="w-28 text-right">Bisa Diretur</TableHead>
                <TableHead className="w-32 text-right">Qty Retur</TableHead>
                <TableHead className="w-44">Batch</TableHead>
                <TableHead className="w-60">Alasan Retur</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {source.lines.map((s) => {
                const l = lines.find(
                  (x) => x.salesOrderLineId === s.salesOrderLineId
                )
                if (!l) return null
                const id = s.salesOrderLineId
                return (
                  <TableRow key={id} className="align-top">
                    <TableCell>
                      <input
                        type="checkbox"
                        className="size-5 accent-primary"
                        aria-label={`Retur ${s.product.sku}`}
                        checked={l.include}
                        disabled={Number(s.availableQty) <= 0 && !l.include}
                        onChange={(e) =>
                          update(id, { include: e.target.checked })
                        }
                      />
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">
                        {s.product.sku} · {s.product.name}
                      </div>
                      <div className="text-xs text-muted-foreground">
                        {formatRupiah(s.unitPrice)} / {s.product.uom}
                        {s.isBundle ? " · bundel" : ""}
                      </div>
                      <LineError message={lineErr(id, "salesOrderLineId")} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatQuantity(s.shippedQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatQuantity(s.availableQty)}
                    </TableCell>
                    <TableCell>
                      <QuantityInput
                        aria-label={`Qty retur ${s.product.sku}`}
                        value={l.qty}
                        disabled={!l.include}
                        onValueChange={(v) => update(id, { qty: v })}
                        aria-invalid={!!lineErr(id, "qty")}
                      />
                      <LineError message={lineErr(id, "qty")} />
                    </TableCell>
                    <TableCell>
                      {s.batches.length > 1 ? (
                        <OptionSelect
                          value={l.batchId}
                          onValueChange={(v) => update(id, { batchId: v })}
                          options={s.batches.map((b) => ({
                            id: b.id,
                            code: b.number,
                            name: b.number,
                          }))}
                          placeholder="Batch"
                          aria-label="Batch"
                        />
                      ) : (
                        <span className="text-sm">
                          {s.batches[0]?.number ?? "—"}
                        </span>
                      )}
                      <LineError message={lineErr(id, "batchId")} />
                    </TableCell>
                    <TableCell>
                      <OptionSelect
                        value={l.returnReasonId}
                        onValueChange={(v) => update(id, { returnReasonId: v })}
                        options={reasonOptions}
                        placeholder="Pilih alasan"
                        aria-label="Alasan retur"
                      />
                      <LineError message={lineErr(id, "returnReasonId")} />
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </section>
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => window.history.back()}
        >
          Batal
        </Button>
        <LoadingButton
          type="submit"
          loading={save.isPending}
          loadingText="Menyimpan…"
        >
          Simpan Draft
        </LoadingButton>
      </div>
    </form>
  )
}

// --- Detail -------------------------------------------------------------------------------------

const RETURN_FIELD_LABELS: Record<string, string> = {
  status: "Status",
  salesOrder: "Sales Order",
  settlementMethod: "Penyelesaian",
  returnDate: "Tanggal retur",
  returnAmount: "Nilai retur",
  costAmount: "HPP",
  pickupRequired: "Perlu pickup",
  inbound: "Inbound",
  lines: "Baris",
}

type DetailProps = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail Retur Penjualan: alur verifikasi, pickup, terima, QC, nota kredit, Riwayat. */
export function SalesReturnDetailPage({ id, tab, onTabChange }: DetailProps) {
  const ret = useQuery(salesReturnQuery(id))
  return (
    <LoadingArea loading={ret.isPending}>
      {ret.data ? (
        <ReturnDetail ret={ret.data} tab={tab} onTabChange={onTabChange} />
      ) : null}
    </LoadingArea>
  )
}

function FileList({ files }: { files: SalesReturn["documentationFiles"] }) {
  if (files.length === 0) return <span>—</span>
  return (
    <ul className="flex flex-col gap-1">
      {files.map((f) => (
        <li key={f.id}>
          <button
            type="button"
            className="text-left text-primary underline-offset-4 hover:underline"
            onClick={() => void openFile(f.id)}
          >
            {f.name || "File"}
          </button>
        </li>
      ))}
    </ul>
  )
}

type DialogKind = "submit" | "send_back" | "verify" | "assign_pickup" | "cancel"

function ReturnDetail({
  ret,
  tab,
  onTabChange,
}: {
  ret: SalesReturn
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const invalidate = useInvalidateSales()
  const [dialog, setDialog] = useState<DialogKind | null>(null)
  const can = (a: SalesReturnAction) => ret.allowedActions.includes(a)
  const messages: Record<ReturnActionInput["kind"], string> = {
    submit: ret.requiresVerification
      ? "Retur diajukan ke Logistik Leader"
      : "Retur diajukan; barang masuk Inbound",
    send_back: "Retur dikembalikan ke Draft",
    verify: "Retur terverifikasi",
    assign_pickup: "Merchandiser ditugaskan",
    record_pickup: "Pickup dicatat",
    receive: "Barang diterima",
    cancel: "Retur dibatalkan",
  }
  const mutation = useIdempotentMutation({
    mutationFn: (input: ReturnActionInput, key) =>
      salesReturnAction(ret.id, ret.version, input, key),
    onSuccess: async (_, input) => {
      setDialog(null)
      toast.success(messages[input.kind])
      await invalidate()
    },
    onError: (err) => {
      setDialog(null)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {ret.number ?? "Draft Retur Penjualan"}
            </h1>
            <StatusBadge status={ret.status} />
            <Badge variant="secondary">
              {ORDER_TYPE_LABELS[ret.orderType]}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            Retur Penjualan · {ret.salesOrderNumber} · {ret.partyName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link to="/sales/returns/$id/edit" params={{ id: ret.id }} />
              }
            >
              Ubah
            </Button>
          ) : null}
          {can("cancel") ? (
            <Button variant="outline" onClick={() => setDialog("cancel")}>
              Batalkan
            </Button>
          ) : null}
          {can("send_back") ? (
            <Button variant="outline" onClick={() => setDialog("send_back")}>
              Kembalikan
            </Button>
          ) : null}
          {can("verify") ? (
            <Button onClick={() => setDialog("verify")}>Verifikasi</Button>
          ) : null}
          {can("assign_pickup") ? (
            <Button onClick={() => setDialog("assign_pickup")}>
              Tugaskan Pickup
            </Button>
          ) : null}
          {can("record_pickup") ? (
            <Button
              render={
                <Link
                  to="/sales/returns/$id/field"
                  params={{ id: ret.id }}
                  search={{ mode: "pickup" }}
                />
              }
            >
              <SmartphoneIcon /> Catat Pickup
            </Button>
          ) : null}
          {can("receive") ? (
            <Button
              render={
                <Link
                  to="/sales/returns/$id/field"
                  params={{ id: ret.id }}
                  search={{ mode: "receive" }}
                />
              }
            >
              <SmartphoneIcon /> Terima Barang
            </Button>
          ) : null}
          {can("submit") ? (
            <Button onClick={() => setDialog("submit")}>Ajukan Retur</Button>
          ) : null}
        </div>
      </div>
      {ret.sendBackNote && ret.status === "draft" ? (
        <p className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm">
          Dikembalikan Logistik Leader: {ret.sendBackNote}
        </p>
      ) : null}
      <PageTabs
        label="Bagian retur"
        value={tab}
        onValueChange={onTabChange}
        tabs={[
          { id: "summary", label: "Ringkasan" },
          { id: "history", label: "Riwayat" },
        ]}
      />
      {tab === "summary" ? (
        <>
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <Info
              label="Sales Order"
              value={
                <Link
                  to="/sales/orders/$id"
                  params={{ id: ret.salesOrderId }}
                  search={{}}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  {ret.salesOrderNumber}
                </Link>
              }
            />
            <Info
              label="Invoice"
              value={
                ret.invoiceId ? (
                  <Link
                    to="/sales/invoices/$id"
                    params={{ id: ret.invoiceId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    {ret.invoiceNumber}
                  </Link>
                ) : (
                  "—"
                )
              }
            />
            <Info label="Tanggal Retur" value={formatDate(ret.returnDate)} />
            <Info
              label="Penyelesaian"
              value={SETTLEMENT_LABELS[ret.settlementMethod]}
            />
            <Info
              label="Nilai Retur (4103)"
              value={formatRupiah(ret.returnAmount)}
            />
            <Info
              label="HPP Asal (1314)"
              value={formatRupiah(ret.costAmount)}
            />
            {ret.requiresVerification ? (
              <>
                <Info
                  label="Diverifikasi"
                  value={
                    ret.verifiedAt
                      ? `${ret.verifiedByName ?? ""} · ${formatDateTime(ret.verifiedAt)}`
                      : "—"
                  }
                />
                <Info
                  label="Pickup"
                  value={
                    !ret.verifiedAt
                      ? "—"
                      : ret.pickupRequired
                        ? ret.pickedUpAt
                          ? `Diambil ${ret.pickupUserName ?? ""} · ${formatDateTime(ret.pickedUpAt)}`
                          : ret.pickupUserName
                            ? `Ditugaskan ke ${ret.pickupUserName}`
                            : "Menunggu penugasan Merchandiser"
                        : "Dikirim partner"
                  }
                />
                <Info
                  label="Foto Pickup"
                  value={<FileList files={ret.pickupFiles} />}
                />
              </>
            ) : null}
            <Info
              label="Diterima"
              value={
                ret.receivedAt
                  ? `${ret.receivedByName ?? ""} · ${formatDateTime(ret.receivedAt)}`
                  : "—"
              }
            />
            <Info
              label="Foto & Form Retur"
              value={<FileList files={ret.documentationFiles} />}
            />
            <Info
              label="Inbound"
              value={
                ret.inboundId ? (
                  <Link
                    to="/warehouse/inbounds/$id"
                    params={{ id: ret.inboundId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    {ret.inboundNumber}
                  </Link>
                ) : (
                  "—"
                )
              }
            />
            <Info
              label="Nota Kredit"
              value={
                ret.creditNote ? (
                  <span className="flex flex-wrap items-center gap-2">
                    <Link
                      to="/sales/credit-notes/$id"
                      params={{ id: ret.creditNote.id }}
                      search={{}}
                      className="text-primary underline-offset-4 hover:underline"
                    >
                      {ret.creditNote.number}
                    </Link>
                    <StatusBadge status={ret.creditNote.status} />
                  </span>
                ) : (
                  "—"
                )
              }
            />
            {ret.journalEntryId ? (
              <Info
                label="Jurnal A-13"
                value={
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: ret.journalEntryId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                }
              />
            ) : null}
            <Info label="Catatan" value={ret.note || "—"} />
            {ret.verifyNote ? (
              <Info label="Catatan Verifikasi" value={ret.verifyNote} />
            ) : null}
            {ret.pickupNote ? (
              <Info label="Catatan Pickup" value={ret.pickupNote} />
            ) : null}
            {ret.receiveNote ? (
              <Info label="Catatan Cek Ulang" value={ret.receiveNote} />
            ) : null}
            {ret.cancelReason ? (
              <Info label="Alasan Dibatalkan" value={ret.cancelReason} />
            ) : null}
          </dl>
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[60rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Produk</TableHead>
                  <TableHead>Alasan</TableHead>
                  <TableHead className="text-right">Qty</TableHead>
                  <TableHead className="text-right">Harga Bersih</TableHead>
                  <TableHead className="text-right">Nilai</TableHead>
                  <TableHead className="text-right">HPP</TableHead>
                  <TableHead className="text-right">Lulus QC</TableHead>
                  <TableHead className="text-right">Ditolak QC</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ret.lines.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell>
                      <div className="font-medium">
                        {l.product.sku} · {l.product.name}
                      </div>
                      {l.batchNumber ? (
                        <div className="text-xs text-muted-foreground">
                          Batch {l.batchNumber}
                        </div>
                      ) : null}
                    </TableCell>
                    <TableCell>{l.returnReason.name}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatQuantity(l.qty)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatRupiah(l.unitPrice)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatRupiah(l.lineAmount)}
                    </TableCell>
                    <TableCell className="text-right">
                      {formatRupiah(l.costAmount)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {l.qcDone ? formatQuantity(l.passedQty) : "—"}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {l.qcDone ? formatQuantity(l.rejectedQty) : "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      ) : (
        <EntityHistory
          entityType="sales_return"
          entityId={ret.id}
          fieldLabels={RETURN_FIELD_LABELS}
        />
      )}
      {dialog === "submit" ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => setDialog(o ? "submit" : null)}
          title="Ajukan retur penjualan?"
          description={
            ret.requiresVerification
              ? "Nomor RJ diberikan dan retur menunggu verifikasi Logistik Leader sesuai SOP retur."
              : "Nomor RJ diberikan, barang dicatat masuk lokasi Inbound berstatus Karantina, dan Inbound dibuat untuk QC Staf Gudang."
          }
          confirmLabel="Ajukan Retur"
          loading={mutation.isPending}
          onConfirm={() => mutation.mutate({ kind: "submit" })}
        />
      ) : null}
      <ReasonDialog
        open={dialog === "cancel"}
        onOpenChange={(o) => setDialog(o ? "cancel" : null)}
        title="Batalkan retur?"
        description="Retur yang barangnya belum masuk Inbound dibatalkan."
        confirmLabel="Batalkan Retur"
        required
        destructive
        loading={mutation.isPending}
        onConfirm={(reason) => mutation.mutate({ kind: "cancel", reason })}
      />
      <Dialog
        open={dialog === "send_back"}
        onOpenChange={(o) => setDialog(o ? "send_back" : null)}
      >
        {dialog === "send_back" ? (
          <NoteForm
            title="Kembalikan retur ke Sales?"
            description="Retur tidak sesuai SOP retur. Catatan wajib diisi agar Sales bisa memperbaiki."
            confirmLabel="Kembalikan"
            required
            loading={mutation.isPending}
            onCancel={() => setDialog(null)}
            onConfirm={(note) => mutation.mutate({ kind: "send_back", note })}
          />
        ) : null}
      </Dialog>
      <Dialog
        open={dialog === "verify"}
        onOpenChange={(o) => setDialog(o ? "verify" : null)}
      >
        {dialog === "verify" ? (
          <VerifyForm
            loading={mutation.isPending}
            onCancel={() => setDialog(null)}
            onConfirm={(pickupRequired, note) =>
              mutation.mutate({ kind: "verify", pickupRequired, note })
            }
          />
        ) : null}
      </Dialog>
      <Dialog
        open={dialog === "assign_pickup"}
        onOpenChange={(o) => setDialog(o ? "assign_pickup" : null)}
      >
        {dialog === "assign_pickup" ? (
          <AssignPickupForm
            loading={mutation.isPending}
            onCancel={() => setDialog(null)}
            onConfirm={(pickupUserId) =>
              mutation.mutate({ kind: "assign_pickup", pickupUserId })
            }
          />
        ) : null}
      </Dialog>
    </div>
  )
}

function NoteForm({
  title,
  description,
  confirmLabel,
  required,
  loading,
  onCancel,
  onConfirm,
}: {
  title: string
  description: string
  confirmLabel: string
  required: boolean
  loading: boolean
  onCancel: () => void
  onConfirm: (note: string) => void
}) {
  const [note, setNote] = useState("")
  const [error, setError] = useState<string>()
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <Field data-invalid={!!error}>
        <FieldLabel htmlFor="rj-action-note">Catatan</FieldLabel>
        <Textarea
          id="rj-action-note"
          value={note}
          maxLength={1000}
          aria-invalid={!!error}
          onChange={(e) => setNote(e.target.value)}
        />
        <FieldError errors={fieldErrors(error)} />
      </Field>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Batal
        </Button>
        <LoadingButton
          loading={loading}
          loadingText="Memproses…"
          onClick={() => {
            if (required && !note.trim()) {
              setError("Catatan wajib diisi")
              return
            }
            onConfirm(note.trim())
          }}
        >
          {confirmLabel}
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

function VerifyForm({
  loading,
  onCancel,
  onConfirm,
}: {
  loading: boolean
  onCancel: () => void
  onConfirm: (pickupRequired: boolean, note: string) => void
}) {
  const [pickup, setPickup] = useState<"pickup" | "partner">("pickup")
  const [note, setNote] = useState("")
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Verifikasi retur MT</DialogTitle>
        <DialogDescription>
          Pastikan retur sesuai SOP retur, lalu tentukan barang diambil
          Merchandiser atau dikirim partner.
        </DialogDescription>
      </DialogHeader>
      <fieldset className="flex flex-col gap-2 text-sm">
        <legend className="mb-1 font-medium">Pengambilan barang</legend>
        <label className="flex min-h-11 items-center gap-2">
          <input
            type="radio"
            name="rj-pickup"
            className="size-5 accent-primary"
            checked={pickup === "pickup"}
            onChange={() => setPickup("pickup")}
          />
          Diambil Merchandiser (pickup)
        </label>
        <label className="flex min-h-11 items-center gap-2">
          <input
            type="radio"
            name="rj-pickup"
            className="size-5 accent-primary"
            checked={pickup === "partner"}
            onChange={() => setPickup("partner")}
          />
          Dikirim partner ke gudang
        </label>
      </fieldset>
      <Field>
        <FieldLabel htmlFor="rj-verify-note">Catatan (opsional)</FieldLabel>
        <Textarea
          id="rj-verify-note"
          value={note}
          maxLength={1000}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Batal
        </Button>
        <LoadingButton
          loading={loading}
          loadingText="Memproses…"
          onClick={() => onConfirm(pickup === "pickup", note.trim())}
        >
          Verifikasi
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

function AssignPickupForm({
  loading,
  onCancel,
  onConfirm,
}: {
  loading: boolean
  onCancel: () => void
  onConfirm: (userId: string) => void
}) {
  const users = useQuery(pickupUsersQuery)
  const [userId, setUserId] = useState("")
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Tugaskan Merchandiser</DialogTitle>
        <DialogDescription>
          Merchandiser mengecek barang di lokasi partner sebelum mengambilnya.
        </DialogDescription>
      </DialogHeader>
      <Field>
        <FieldLabel>Merchandiser</FieldLabel>
        <OptionSelect
          value={userId}
          onValueChange={setUserId}
          options={(users.data ?? []).map((u) => ({
            id: u.id,
            code: u.name,
            name: u.name,
          }))}
          placeholder="Pilih Merchandiser"
          aria-label="Merchandiser"
        />
      </Field>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          Batal
        </Button>
        <LoadingButton
          loading={loading}
          loadingText="Memproses…"
          disabled={!userId}
          onClick={() => onConfirm(userId)}
        >
          Tugaskan
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

// --- Form lapangan (HP) -------------------------------------------------------------------------

/** Kumpulan foto: setiap unggahan menambah daftar (foto barang, form retur bertanda tangan). */
function PhotoList({
  label,
  files,
  onChange,
}: {
  label: string
  files: { id: string; name: string }[]
  onChange: (files: { id: string; name: string }[]) => void
}) {
  const [error, setError] = useState<string>()
  return (
    <div className="flex flex-col gap-2">
      <FileUpload
        label={label}
        accept={PHOTO_TYPES}
        maxSizeBytes={MAX_PROOF_FILE_BYTES}
        requestUpload={requestReturnPhotoUpload}
        onUploaded={(id, file) => {
          setError(undefined)
          onChange([...files, { id, name: file.name }])
        }}
        onError={setError}
      />
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      {files.length > 0 ? (
        <ul className="flex flex-col gap-1 text-sm">
          {files.map((f) => (
            <li
              key={f.id}
              className="flex min-h-11 items-center justify-between gap-2 rounded-md border px-3"
            >
              <span className="truncate">{f.name}</span>
              <Button
                type="button"
                variant="ghost"
                onClick={() => onChange(files.filter((x) => x.id !== f.id))}
              >
                Hapus
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

/**
 * Form lapangan dari HP: Merchandiser mencatat pickup di lokasi partner (foto wajib) atau Sales/Gudang
 * menerima barang retur MT (cek ulang qty, foto barang & form retur) sehingga barang masuk Inbound.
 */
export function SalesReturnFieldPage({
  id,
  mode,
}: {
  id: string
  mode: "pickup" | "receive"
}) {
  const ret = useQuery(salesReturnQuery(id))
  return (
    <LoadingArea loading={ret.isPending}>
      {ret.data ? (
        <FieldForm key={ret.data.version} ret={ret.data} mode={mode} />
      ) : null}
    </LoadingArea>
  )
}

function FieldForm({
  ret,
  mode,
}: {
  ret: SalesReturn
  mode: "pickup" | "receive"
}) {
  const navigate = useNavigate()
  const invalidate = useInvalidateSales()
  const [files, setFiles] = useState<{ id: string; name: string }[]>([])
  const [note, setNote] = useState("")
  const [qtys, setQtys] = useState<Record<string, string>>(() =>
    Object.fromEntries(ret.lines.map((l) => [l.id, l.qty]))
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const allowed =
    mode === "pickup"
      ? ret.allowedActions.includes("record_pickup")
      : ret.allowedActions.includes("receive")

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      salesReturnAction(
        ret.id,
        ret.version,
        mode === "pickup"
          ? {
              kind: "record_pickup",
              note,
              photoFileIds: files.map((f) => f.id),
            }
          : {
              kind: "receive",
              note,
              documentationFileIds: files.map((f) => f.id),
              lines: ret.lines
                .filter((l) => qtys[l.id] && qtys[l.id] !== l.qty)
                .map((l) => ({ lineId: l.id, qty: qtys[l.id] })),
            },
        key
      ),
    onSuccess: async () => {
      toast.success(
        mode === "pickup"
          ? "Pickup dicatat"
          : "Barang diterima dan masuk Inbound"
      )
      await invalidate()
      void navigate({
        to: "/sales/returns/$id",
        params: { id: ret.id },
        search: {},
      })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    if (files.length === 0) {
      setErrors({
        files:
          mode === "pickup"
            ? "Unggah minimal satu foto barang di lokasi partner"
            : "Unggah foto barang dan form retur",
      })
      return
    }
    setErrors({})
    save.mutate()
  }

  if (!allowed) {
    return (
      <p className="text-sm text-muted-foreground">
        Aksi ini tidak tersedia untuk retur {ret.number} saat ini.
      </p>
    )
  }

  return (
    <form
      noValidate
      className="mx-auto flex w-full max-w-lg flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <div>
        <h1 className="text-xl font-semibold tracking-tight">
          {mode === "pickup" ? "Pickup Retur" : "Terima Barang Retur"}{" "}
          {ret.number}
        </h1>
        <p className="text-sm text-muted-foreground">
          {ret.partyName} · {ret.salesOrderNumber}
        </p>
      </div>
      <ul className="flex flex-col gap-3">
        {ret.lines.map((l) => (
          <li key={l.id} className="flex flex-col gap-2 rounded-lg border p-3">
            <div>
              <div className="font-medium">
                {l.product.sku} · {l.product.name}
              </div>
              <div className="text-xs text-muted-foreground">
                {l.returnReason.name}
                {l.batchNumber ? ` · Batch ${l.batchNumber}` : ""} · Qty retur{" "}
                {formatQuantity(l.qty)} {l.product.uom}
              </div>
            </div>
            {mode === "receive" ? (
              <Field
                data-invalid={!!errors[`lines.${ret.lines.indexOf(l)}.qty`]}
              >
                <FieldLabel htmlFor={`rj-qty-${l.id}`}>
                  Qty diterima (cek ulang)
                </FieldLabel>
                <QuantityInput
                  id={`rj-qty-${l.id}`}
                  className="h-11"
                  value={qtys[l.id] ?? ""}
                  onValueChange={(v) => setQtys((q) => ({ ...q, [l.id]: v }))}
                  unit={l.product.uom}
                />
              </Field>
            ) : null}
          </li>
        ))}
      </ul>
      {Object.entries(errors)
        .filter(([k]) => k.startsWith("lines."))
        .map(([k, v]) => (
          <p key={k} className="text-sm text-destructive">
            {v}
          </p>
        ))}
      <Field data-invalid={!!errors.files}>
        <FieldLabel>
          {mode === "pickup"
            ? "Foto barang di lokasi partner"
            : "Foto barang & form retur"}
        </FieldLabel>
        <PhotoList
          label={
            mode === "pickup"
              ? "Ambil / pilih foto"
              : "Ambil foto barang atau form"
          }
          files={files}
          onChange={setFiles}
        />
        <FieldError
          errors={fieldErrors(
            errors.files ?? errors.photoFileIds ?? errors.documentationFileIds
          )}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="rj-field-note">Catatan</FieldLabel>
        <Textarea
          id="rj-field-note"
          rows={3}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <LoadingButton
        type="submit"
        className="h-12 w-full"
        loading={save.isPending}
        loadingText="Menyimpan…"
      >
        {mode === "pickup" ? "Simpan Pickup" : "Terima & Kirim ke Inbound"}
      </LoadingButton>
      <Button
        type="button"
        variant="outline"
        className="h-11 w-full"
        render={
          <Link to="/sales/returns/$id" params={{ id: ret.id }} search={{}} />
        }
      >
        Kembali ke Detail
      </Button>
    </form>
  )
}
