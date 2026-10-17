import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
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
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  ordersByStatusQuery,
  RETURN_ORIGIN_LABELS,
  RETURN_STATUSES,
  RETURNABLE_ORDER_STATUSES,
  returnableReceiptsQuery,
  returnAction,
  returnQuery,
  returnsQuery,
  saveReturn,
  stockLocationsQuery,
  type PurchaseReturn,
  type PurchaseReturnSummary,
  type ReturnableReceipts,
} from "../api/documents-api"
import { DocumentToolbar } from "./document-toolbar"
import type { DetailTab } from "./request-detail-page"
import {
  ApprovalPanel,
  fieldErrors,
  Info,
  LineError,
  OptionSelect,
  ReasonDialog,
} from "./shared"

const listRoute = getRouteApi("/_app/procurement/purchase-returns/")

type StockStatus = "quarantine" | "available"

const STOCK_STATUS_LABELS: Record<StockStatus, string> = {
  quarantine: "Karantina",
  available: "Tersedia",
}

/** Pengadaan › Reject & Waste: retur pembelian ke pemasok/maklon dengan nota debit (S-210). */
export function ReturnListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const returns = useQuery(returnsQuery(search))

  const columns: DataTableColumn<PurchaseReturnSummary>[] = [
    {
      id: "number",
      header: "Nomor RB / DN",
      cell: (r) => (
        <div className="flex flex-col">
          <Link
            to="/procurement/purchase-returns/$id"
            params={{ id: r.id }}
            search={{}}
            className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
          >
            {r.number ?? "Draft"}
          </Link>
          {r.debitNoteNumber ? (
            <span className="text-xs text-muted-foreground">
              {r.debitNoteNumber}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (r) => (
        <span className="whitespace-nowrap">{formatDate(r.businessDate)}</span>
      ),
    },
    {
      id: "po",
      header: "Purchase Order",
      cell: (r) => (
        <div className="flex flex-col">
          <span className="whitespace-nowrap">{r.purchaseOrderNumber}</span>
          <span className="text-xs text-muted-foreground">
            {r.supplierName}
          </span>
        </div>
      ),
    },
    {
      id: "origin",
      header: "Asal",
      cell: (r) => (
        <Badge variant="outline">{RETURN_ORIGIN_LABELS[r.origin]}</Badge>
      ),
    },
    {
      id: "reason",
      header: "Alasan",
      cell: (r) => <span className="line-clamp-2">{r.reason}</span>,
    },
    {
      id: "total",
      header: "Nilai",
      align: "right",
      cell: (r) => formatRupiah(r.totalAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (r) => <StatusBadge status={r.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Reject & Waste
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Retur pembelian barang reject ke pemasok atau maklon dengan nota
            debit. Retur manual disetujui Head Logistik; reject dari QC
            Inbound/Outbound langsung diposting tanpa persetujuan. Posting
            membuat jurnal A-02 dan mengurangi utang (atau Barang Diterima Belum
            Ditagih).
          </p>
        </div>
        {can("procurement.purchase_return.write") ? (
          <Button
            render={<Link to="/procurement/purchase-returns/new" search={{}} />}
          >
            <PlusIcon /> Buat Retur Pembelian
          </Button>
        ) : null}
      </div>
      <DocumentToolbar
        searchLabel="Cari retur"
        searchPlaceholder="Cari nomor RB, DN, PO, pemasok"
        q={search.q}
        status={search.status}
        statuses={RETURN_STATUSES}
        dateFrom={search.dateFrom}
        dateTo={search.dateTo}
        onSearch={(q) =>
          void navigate({ search: (s) => ({ ...s, q, page: 1 }) })
        }
        onStatus={(status) =>
          void navigate({ search: (s) => ({ ...s, status, page: 1 }) })
        }
        onDates={(dateFrom, dateTo) =>
          void navigate({
            search: (s) => ({ ...s, dateFrom, dateTo, page: 1 }),
          })
        }
      />
      <DataTable
        columns={columns}
        rows={returns.data?.items}
        getRowId={(r) => r.id}
        loading={returns.isPending}
        fetching={returns.isFetching}
        page={search.page}
        totalPages={returns.data?.totalPages ?? 0}
        totalItems={returns.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada retur pembelian."
      />
    </div>
  )
}

// --- Form ---------------------------------------------------------------------------------------

type LineDraft = {
  goodsReceiptLineId: string
  include: boolean
  qty: string
  locationId: string
  stockStatus: StockStatus
}

/** Buat retur pembelian manual. */
export function ReturnCreatePage({
  purchaseOrderId,
}: {
  purchaseOrderId?: string
}) {
  const [order, setOrder] = useState<PickerOption | null>(null)
  const orderId = order?.id ?? purchaseOrderId ?? ""
  const returnable = useQuery({
    ...returnableReceiptsQuery(orderId),
    enabled: !!orderId,
  })
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Buat Retur Pembelian
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Pilih PO lalu baris Penerimaan Barang yang dikembalikan beserta lokasi
          dan status stok asalnya. Retur diajukan ke Head Logistik sebelum
          diposting.
        </p>
      </div>
      {!purchaseOrderId ? (
        <Field className="max-w-md">
          <FieldLabel htmlFor="rb-po">Purchase Order</FieldLabel>
          <SearchPicker
            id="rb-po"
            value={order}
            onValueChange={setOrder}
            query={(q) => ordersByStatusQuery(q, RETURNABLE_ORDER_STATUSES)}
            toOption={(o) => ({
              id: o.id,
              label: o.number ?? "Draft",
              description: `${o.supplierName} · ${o.orderTypeLabel}`,
            })}
            placeholder="Pilih PO"
            searchPlaceholder="Cari nomor PO atau pemasok"
          />
        </Field>
      ) : null}
      {orderId ? (
        <LoadingArea loading={returnable.isPending}>
          {returnable.data ? (
            <ReturnForm key={orderId} returnable={returnable.data} />
          ) : null}
        </LoadingArea>
      ) : null}
    </div>
  )
}

/** Ubah draft retur. */
export function ReturnEditPage({ id }: { id: string }) {
  const ret = useQuery(returnQuery(id))
  const returnable = useQuery({
    ...returnableReceiptsQuery(ret.data?.purchaseOrderId ?? ""),
    enabled: !!ret.data,
  })
  return (
    <LoadingArea loading={ret.isPending || returnable.isPending}>
      {ret.data && returnable.data ? (
        ret.data.allowedActions.includes("edit") ? (
          <div className="flex flex-col gap-4">
            <h1 className="text-2xl font-semibold tracking-tight">
              Ubah Draft Retur {ret.data.purchaseOrderNumber}
            </h1>
            <ReturnForm returnable={returnable.data} ret={ret.data} />
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
  returnable,
  ret,
}: {
  returnable: ReturnableReceipts
  ret?: PurchaseReturn
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const locations = useQuery(stockLocationsQuery)
  const [businessDate, setBusinessDate] = useState(
    ret?.businessDate ?? todayBusinessDate()
  )
  const [reason, setReason] = useState(ret?.reason ?? "")
  const [note, setNote] = useState(ret?.note ?? "")
  const [lines, setLines] = useState<LineDraft[]>(() =>
    returnable.lines.map((l) => {
      const r = ret?.lines.find(
        (x) => x.goodsReceiptLineId === l.goodsReceiptLineId
      )
      return {
        goodsReceiptLineId: l.goodsReceiptLineId,
        include: !!r,
        qty: r?.qty ?? "",
        locationId: r?.location.id ?? l.location.id,
        stockStatus: r?.stockStatus ?? "quarantine",
      }
    })
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const included = lines.filter((l) => l.include)
  const locationOptions = (locations.data ?? []).map((l) => ({
    id: l.id,
    code: l.code,
    name: `${l.code} · ${l.name}`,
  }))

  const update = (id: string, patch: Partial<LineDraft>) =>
    setLines((ls) =>
      ls.map((l) => (l.goodsReceiptLineId === id ? { ...l, ...patch } : l))
    )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      saveReturn(
        ret,
        returnable.purchaseOrderId,
        {
          businessDate,
          reason,
          note,
          lines: included.map((l) => ({
            goodsReceiptLineId: l.goodsReceiptLineId,
            locationId: l.locationId,
            stockStatus: l.stockStatus,
            qty: l.qty,
          })),
        },
        key
      ),
    onSuccess: async (saved) => {
      toast.success("Draft retur disimpan")
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      void navigate({
        to: "/procurement/purchase-returns/$id",
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
            mapped[`${included[Number(m[1])].goodsReceiptLineId}.${m[2]}`] = v
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
    if (!reason.trim()) next.reason = "Alasan retur wajib diisi"
    if (!businessDate) next.businessDate = "Tanggal retur wajib diisi"
    if (included.length === 0) next.lines = "Pilih minimal satu baris"
    included.forEach((l) => {
      if (!l.qty || Number(l.qty) <= 0)
        next[`${l.goodsReceiptLineId}.qty`] = "Qty harus lebih dari nol"
    })
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const err = (k: string) => errors[k]
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
      <div className="grid gap-x-6 gap-y-3 rounded-lg border p-4 text-sm sm:grid-cols-2">
        <Info label="Purchase Order" value={returnable.purchaseOrderNumber} />
        <Info label="Pemasok" value={returnable.supplierName} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Field data-invalid={!!err("businessDate")}>
          <FieldLabel htmlFor="rb-date">Tanggal Retur</FieldLabel>
          <DateInput
            id="rb-date"
            value={businessDate}
            onValueChange={setBusinessDate}
            aria-invalid={!!err("businessDate")}
          />
          <FieldError errors={fieldErrors(err("businessDate"))} />
        </Field>
        <Field data-invalid={!!err("reason")} className="lg:col-span-2">
          <FieldLabel htmlFor="rb-reason">Alasan Retur</FieldLabel>
          <Textarea
            id="rb-reason"
            rows={2}
            maxLength={400}
            value={reason}
            aria-invalid={!!err("reason")}
            onChange={(e) => setReason(e.target.value)}
          />
          <FieldError errors={fieldErrors(err("reason"))} />
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor="rb-note">Catatan</FieldLabel>
        <Textarea
          id="rb-note"
          rows={2}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">Barang yang Diretur</h2>
        {err("lines") ? (
          <p className="text-sm text-destructive">{err("lines")}</p>
        ) : null}
        {returnable.lines.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Tidak ada penerimaan PO ini yang bisa diretur.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[60rem]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12" />
                  <TableHead>Penerimaan / Produk</TableHead>
                  <TableHead className="w-28 text-right">
                    Bisa Diretur
                  </TableHead>
                  <TableHead className="w-32 text-right">Qty Retur</TableHead>
                  <TableHead className="w-60">Lokasi Asal</TableHead>
                  <TableHead className="w-36">Status Stok</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {returnable.lines.map((r) => {
                  const l = lines.find(
                    (x) => x.goodsReceiptLineId === r.goodsReceiptLineId
                  )
                  if (!l) return null
                  return (
                    <TableRow key={r.goodsReceiptLineId} className="align-top">
                      <TableCell>
                        <input
                          type="checkbox"
                          className="size-5 accent-primary"
                          aria-label={`Retur ${r.goodsReceiptNumber} ${r.product.sku}`}
                          checked={l.include}
                          onChange={(e) =>
                            update(r.goodsReceiptLineId, {
                              include: e.target.checked,
                            })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <div className="font-medium">
                          {r.goodsReceiptNumber}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {r.product.sku} · {r.product.name}
                          {r.batchNumber ? ` · ${r.batchNumber}` : ""}
                        </div>
                        <LineError
                          message={lineErr(
                            r.goodsReceiptLineId,
                            "receiptLineId"
                          )}
                        />
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(r.returnableQty)}
                      </TableCell>
                      <TableCell>
                        <QuantityInput
                          aria-label={`Qty retur ${r.product.sku}`}
                          value={l.qty}
                          disabled={!l.include}
                          onValueChange={(v) =>
                            update(r.goodsReceiptLineId, { qty: v })
                          }
                          aria-invalid={!!lineErr(r.goodsReceiptLineId, "qty")}
                        />
                        <LineError
                          message={lineErr(r.goodsReceiptLineId, "qty")}
                        />
                      </TableCell>
                      <TableCell>
                        <OptionSelect
                          value={l.locationId}
                          onValueChange={(v) =>
                            update(r.goodsReceiptLineId, { locationId: v })
                          }
                          options={
                            locationOptions.length
                              ? locationOptions
                              : [
                                  {
                                    id: r.location.id,
                                    code: r.location.code,
                                    name: `${r.location.code} · ${r.location.name}`,
                                  },
                                ]
                          }
                          placeholder="Pilih lokasi"
                          aria-label="Lokasi asal"
                        />
                        <LineError
                          message={lineErr(r.goodsReceiptLineId, "locationId")}
                        />
                      </TableCell>
                      <TableCell>
                        <Select
                          value={l.stockStatus}
                          onValueChange={(v) =>
                            update(r.goodsReceiptLineId, {
                              stockStatus: (v ?? "quarantine") as StockStatus,
                            })
                          }
                        >
                          <SelectTrigger
                            className="w-full"
                            aria-label="Status stok asal"
                          >
                            <SelectValue>
                              {(v: StockStatus) => STOCK_STATUS_LABELS[v]}
                            </SelectValue>
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="quarantine">
                              Karantina
                            </SelectItem>
                            <SelectItem value="available">Tersedia</SelectItem>
                          </SelectContent>
                        </Select>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
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
  purchaseOrderId: "Purchase Order",
  origin: "Asal",
  businessDate: "Tanggal retur",
  reason: "Alasan",
  note: "Catatan",
  status: "Status",
  totalAmount: "Nilai retur",
  debitNoteNumber: "Nota debit",
  lines: "Baris",
}

type DetailProps = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail retur pembelian: pengajuan, posting (nota debit, A-02), Riwayat. */
export function ReturnDetailPage({ id, tab, onTabChange }: DetailProps) {
  const ret = useQuery(returnQuery(id))
  return (
    <LoadingArea loading={ret.isPending}>
      {ret.data ? (
        <ReturnDetail ret={ret.data} tab={tab} onTabChange={onTabChange} />
      ) : null}
    </LoadingArea>
  )
}

function ReturnDetail({
  ret,
  tab,
  onTabChange,
}: {
  ret: PurchaseReturn
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<"submit" | "post" | "revise" | null>(
    null
  )
  const [cancelling, setCancelling] = useState(false)
  const can = (a: PurchaseReturn["allowedActions"][number]) =>
    ret.allowedActions.includes(a)
  const messages = {
    submit: "Retur diajukan ke Head Logistik",
    post: "Retur diposting",
    revise: "Retur kembali ke Draft",
    cancel: "Retur dibatalkan",
  }

  const mutation = useIdempotentMutation({
    mutationFn: (
      v: { kind: "submit" | "post" | "revise" | "cancel"; reason?: string },
      key
    ) => returnAction(v.kind, ret.id, ret.version, key, v.reason),
    onSuccess: async (_, v) => {
      setConfirm(null)
      setCancelling(false)
      toast.success(messages[v.kind])
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      await queryClient.invalidateQueries({ queryKey: ["approval"] })
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const dialogs = {
    submit: {
      title: "Ajukan retur pembelian?",
      description: "Nomor RB diberikan dan retur diajukan ke Head Logistik.",
      label: "Ajukan Retur",
    },
    post: {
      title: "Posting retur pembelian?",
      description:
        "Barang keluar dari lokasi asal dengan HPP FIFO, nomor nota debit (DN) diberikan, dan jurnal A-02 dibuat: bagian yang belum ditagih mengurangi Barang Diterima Belum Ditagih, bagian yang sudah ditagih mengurangi utang.",
      label: "Posting Retur",
    },
    revise: {
      title: "Revisi retur?",
      description: "Retur yang ditolak kembali menjadi Draft.",
      label: "Revisi",
    },
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {ret.number ?? "Draft Retur Pembelian"}
            </h1>
            <StatusBadge status={ret.status} />
            <Badge variant="outline">{RETURN_ORIGIN_LABELS[ret.origin]}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            Retur Pembelian · {ret.purchaseOrderNumber} · {ret.supplierName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/purchase-returns/$id/edit"
                  params={{ id: ret.id }}
                />
              }
            >
              Ubah
            </Button>
          ) : null}
          {can("cancel") ? (
            <Button variant="outline" onClick={() => setCancelling(true)}>
              Batalkan
            </Button>
          ) : null}
          {can("revise") ? (
            <Button variant="outline" onClick={() => setConfirm("revise")}>
              Revisi
            </Button>
          ) : null}
          {can("submit") ? (
            <Button onClick={() => setConfirm("submit")}>Ajukan Retur</Button>
          ) : null}
          {can("post") ? (
            <Button onClick={() => setConfirm("post")}>Posting Retur</Button>
          ) : null}
        </div>
      </div>
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
              label="Purchase Order"
              value={
                <Link
                  to="/procurement/purchase-orders/$id"
                  params={{ id: ret.purchaseOrderId }}
                  search={{}}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  {ret.purchaseOrderNumber}
                </Link>
              }
            />
            <Info label="Pemasok" value={ret.supplierName} />
            <Info label="Tanggal Retur" value={formatDate(ret.businessDate)} />
            <Info label="Nota Debit" value={ret.debitNoteNumber ?? "—"} />
            <Info
              label="Nilai Retur (HPP)"
              value={formatRupiah(ret.totalAmount)}
            />
            <Info
              label="Mengurangi Belum Ditagih (2102)"
              value={formatRupiah(ret.unbilledAmount)}
            />
            <Info
              label="Mengurangi Utang (2101)"
              value={formatRupiah(ret.billedAmount)}
            />
            {ret.sourceLabel ? (
              <Info label="Dokumen QC" value={ret.sourceLabel} />
            ) : null}
            <Info label="Alasan" value={ret.reason} />
            <Info
              label="Diposting"
              value={ret.postedAt ? formatDateTime(ret.postedAt) : "—"}
            />
            {ret.journalEntryId ? (
              <Info
                label="Jurnal A-02"
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
            {ret.cancelReason ? (
              <Info label="Alasan Dibatalkan" value={ret.cancelReason} />
            ) : null}
          </dl>
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[56rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Penerimaan / Produk</TableHead>
                  <TableHead>Lokasi Asal</TableHead>
                  <TableHead className="text-right">Qty</TableHead>
                  <TableHead className="text-right">HPP / Unit</TableHead>
                  <TableHead className="text-right">Nilai</TableHead>
                  <TableHead className="text-right">Belum Ditagih</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {ret.lines.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell>
                      <div className="font-medium">{l.goodsReceiptNumber}</div>
                      <div className="text-xs text-muted-foreground">
                        {l.product.sku} · {l.product.name}
                        {l.batchNumber ? ` · ${l.batchNumber}` : ""}
                      </div>
                    </TableCell>
                    <TableCell>
                      {l.location.code}
                      <div className="text-xs text-muted-foreground">
                        {STOCK_STATUS_LABELS[l.stockStatus]}
                      </div>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatQuantity(l.qty)}
                    </TableCell>
                    <TableCell className="text-right">
                      {ret.status === "posted" ? formatRupiah(l.unitCost) : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      {ret.status === "posted" ? formatRupiah(l.amount) : "—"}
                    </TableCell>
                    <TableCell className="text-right">
                      {ret.status === "posted"
                        ? formatRupiah(l.unbilledAmount)
                        : "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {ret.origin === "manual" ? (
            <ApprovalPanel subjectType="purchase_return" subjectId={ret.id} />
          ) : null}
        </>
      ) : (
        <EntityHistory
          entityType="purchase_return"
          entityId={ret.id}
          fieldLabels={RETURN_FIELD_LABELS}
        />
      )}
      {confirm ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => setConfirm(o ? confirm : null)}
          title={dialogs[confirm].title}
          description={dialogs[confirm].description}
          confirmLabel={dialogs[confirm].label}
          loading={mutation.isPending}
          onConfirm={() => mutation.mutate({ kind: confirm })}
        />
      ) : null}
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan retur?"
        description="Retur yang belum diposting dibatalkan."
        confirmLabel="Batalkan Retur"
        required={false}
        destructive
        loading={mutation.isPending}
        onConfirm={(reason) => mutation.mutate({ kind: "cancel", reason })}
      />
    </div>
  )
}
