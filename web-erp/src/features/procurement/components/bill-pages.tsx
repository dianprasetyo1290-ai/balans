import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import Big from "big.js"
import { PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatPercent, formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  BILL_STATUSES,
  BILLABLE_ORDER_STATUSES,
  billableReceiptsQuery,
  billAction,
  billQuery,
  billsQuery,
  MATCH_STATUS_LABELS,
  ordersByStatusQuery,
  saveBill,
  WITHHOLDING_LABELS,
  type BillableReceipts,
  type SupplierBill,
  type SupplierBillSummary,
  type WithholdingType,
} from "../api/documents-api"
import { lineAmount, priceVariancePercent } from "../lib/procurement-lib"
import { DocumentToolbar } from "./document-toolbar"
import type { DetailTab } from "./request-detail-page"
import {
  ApprovalPanel,
  fieldErrors,
  Info,
  LineError,
  ReasonDialog,
} from "./shared"

const listRoute = getRouteApi("/_app/procurement/supplier-bills/")

/** Pengadaan › Tagihan Pemasok: tagihan dengan 3-way match PO–GR–tagihan (S-208). */
export function BillListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const bills = useQuery(billsQuery(search))

  const columns: DataTableColumn<SupplierBillSummary>[] = [
    {
      id: "number",
      header: "No. Tagihan",
      cell: (b) => (
        <Link
          to="/procurement/supplier-bills/$id"
          params={{ id: b.id }}
          search={{}}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {b.number}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal / Jatuh Tempo",
      cell: (b) => (
        <div className="flex flex-col whitespace-nowrap">
          <span>{formatDate(b.billDate)}</span>
          <span className="text-xs text-muted-foreground">
            {formatDate(b.dueDate)}
          </span>
        </div>
      ),
    },
    {
      id: "po",
      header: "Purchase Order",
      cell: (b) => (
        <div className="flex flex-col">
          <span className="whitespace-nowrap">{b.purchaseOrderNumber}</span>
          <span className="text-xs text-muted-foreground">
            {b.supplierName}
          </span>
        </div>
      ),
    },
    {
      id: "match",
      header: "3-Way Match",
      cell: (b) => (
        <Badge
          variant={
            b.matchStatus === "exceeds_tolerance" ? "destructive" : "outline"
          }
        >
          {MATCH_STATUS_LABELS[b.matchStatus]}
        </Badge>
      ),
    },
    {
      id: "total",
      header: "Utang",
      align: "right",
      cell: (b) => formatRupiah(b.totalPayableAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (b) => (
        <div className="flex flex-col gap-1">
          <StatusBadge status={b.status} />
          {b.paymentBlocked &&
          b.status !== "draft" &&
          b.status !== "cancelled" ? (
            <span className="text-xs text-destructive">
              Pembayaran diblokir
            </span>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Tagihan Pemasok
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Tagihan pemasok/maklon dicocokkan dengan PO dan Penerimaan Barang
            (toleransi qty 0%, harga 1%). Di luar toleransi butuh persetujuan
            Head Finance & Accounting dan pembayaran diblokir. Posting membuat
            jurnal A-18 dan memotong uang muka otomatis.
          </p>
        </div>
        {can("procurement.supplier_bill.write") ? (
          <Button
            render={<Link to="/procurement/supplier-bills/new" search={{}} />}
          >
            <PlusIcon /> Catat Tagihan
          </Button>
        ) : null}
      </div>
      <DocumentToolbar
        searchLabel="Cari tagihan"
        searchPlaceholder="Cari nomor tagihan, PO, pemasok"
        q={search.q}
        status={search.status}
        statuses={BILL_STATUSES}
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
        rows={bills.data?.items}
        getRowId={(b) => b.id}
        loading={bills.isPending}
        fetching={bills.isFetching}
        page={search.page}
        totalPages={bills.data?.totalPages ?? 0}
        totalItems={bills.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada tagihan pemasok."
      />
    </div>
  )
}

// --- Form ---------------------------------------------------------------------------------------

type LineDraft = {
  goodsReceiptLineId: string
  include: boolean
  qty: string
  unitPrice: string
}

/** Catat tagihan pemasok untuk satu PO. */
export function BillCreatePage({
  purchaseOrderId,
}: {
  purchaseOrderId?: string
}) {
  const [order, setOrder] = useState<PickerOption | null>(null)
  const orderId = order?.id ?? purchaseOrderId ?? ""
  const billable = useQuery({
    ...billableReceiptsQuery(orderId),
    enabled: !!orderId,
  })
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Catat Tagihan Pemasok
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Pilih PO lalu baris Penerimaan Barang yang ditagih. Qty tagihan tidak
          boleh melebihi qty diterima; selisih qty atau harga di atas toleransi
          diajukan ke Head Finance & Accounting saat posting.
        </p>
      </div>
      {!purchaseOrderId ? (
        <Field className="max-w-md">
          <FieldLabel htmlFor="bill-po">Purchase Order</FieldLabel>
          <SearchPicker
            id="bill-po"
            value={order}
            onValueChange={setOrder}
            query={(q) => ordersByStatusQuery(q, BILLABLE_ORDER_STATUSES)}
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
        <LoadingArea loading={billable.isPending}>
          {billable.data ? (
            <BillForm key={orderId} billable={billable.data} />
          ) : null}
        </LoadingArea>
      ) : null}
    </div>
  )
}

/** Ubah draft tagihan. */
export function BillEditPage({ id }: { id: string }) {
  const bill = useQuery(billQuery(id))
  const billable = useQuery({
    ...billableReceiptsQuery(bill.data?.purchaseOrderId ?? ""),
    enabled: !!bill.data,
  })
  return (
    <LoadingArea loading={bill.isPending || billable.isPending}>
      {bill.data && billable.data ? (
        bill.data.allowedActions.includes("edit") ? (
          <div className="flex flex-col gap-4">
            <h1 className="text-2xl font-semibold tracking-tight">
              Ubah Tagihan {bill.data.number}
            </h1>
            <BillForm billable={billable.data} bill={bill.data} />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            Hanya tagihan berstatus Draft yang bisa diubah.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

function BillForm({
  billable,
  bill,
}: {
  billable: BillableReceipts
  bill?: SupplierBill
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [number, setNumber] = useState(bill?.number ?? "")
  const [billDate, setBillDate] = useState(
    bill?.billDate ?? todayBusinessDate()
  )
  const [dueDate, setDueDate] = useState(bill?.dueDate ?? "")
  const [withholdingType, setWithholdingType] = useState<WithholdingType>(
    bill?.withholdingType ?? billable.defaultWithholdingType
  )
  const [withholdingRate, setWithholdingRate] = useState(
    String(Number(bill?.withholdingRate ?? billable.defaultWithholdingRate))
  )
  const [note, setNote] = useState(bill?.note ?? "")
  const [lines, setLines] = useState<LineDraft[]>(() =>
    billable.lines.map((l) => {
      const b = bill?.lines.find(
        (x) => x.goodsReceiptLineId === l.goodsReceiptLineId
      )
      const open = new Big(l.openQty).minus(l.pendingQty)
      return {
        goodsReceiptLineId: l.goodsReceiptLineId,
        include: bill ? !!b : open.gt(0),
        qty: b?.qty ?? (open.gt(0) ? open.toFixed(3) : ""),
        unitPrice: b?.unitPrice ?? l.poUnitPrice,
      }
    })
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const included = lines.filter((l) => l.include)
  const subtotal = included.reduce(
    (s, l) => s.plus(lineAmount(l.qty, l.unitPrice)),
    new Big(0)
  )
  const tolerance = Number(billable.priceTolerancePercent)

  const update = (id: string, patch: Partial<LineDraft>) =>
    setLines((ls) =>
      ls.map((l) => (l.goodsReceiptLineId === id ? { ...l, ...patch } : l))
    )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      saveBill(
        bill,
        billable.purchaseOrderId,
        {
          number,
          billDate,
          dueDate: dueDate || null,
          withholdingType,
          withholdingRate:
            withholdingType === "none" ? "0" : withholdingRate || "0",
          note,
          lines: included.map((l) => ({
            goodsReceiptLineId: l.goodsReceiptLineId,
            qty: l.qty,
            unitPrice: l.unitPrice || "0",
          })),
        },
        key
      ),
    onSuccess: async (saved) => {
      toast.success("Draft tagihan disimpan")
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      void navigate({
        to: "/procurement/supplier-bills/$id",
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
    if (!number.trim()) next.number = "Nomor tagihan pemasok wajib diisi"
    if (!billDate) next.billDate = "Tanggal tagihan wajib diisi"
    if (included.length === 0)
      next.lines = "Pilih minimal satu baris penerimaan"
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
      <div className="grid gap-x-6 gap-y-3 rounded-lg border p-4 text-sm sm:grid-cols-3">
        <Info label="Purchase Order" value={billable.purchaseOrderNumber} />
        <Info label="Pemasok" value={billable.supplierName} />
        <Info label="Termin" value={`${billable.paymentTermDays} hari`} />
        <Info
          label="Sisa Uang Muka (dipotong otomatis)"
          value={formatRupiah(billable.outstandingAdvanceAmount)}
        />
        <Info
          label="Toleransi"
          value={`Qty ${formatPercent(billable.qtyTolerancePercent)}, harga ${formatPercent(billable.priceTolerancePercent)}`}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Field data-invalid={!!err("number")}>
          <FieldLabel htmlFor="bill-number">No. Tagihan Pemasok</FieldLabel>
          <Input
            id="bill-number"
            value={number}
            maxLength={60}
            aria-invalid={!!err("number")}
            onChange={(e) => setNumber(e.target.value)}
          />
          <FieldError errors={fieldErrors(err("number"))} />
        </Field>
        <Field data-invalid={!!err("billDate")}>
          <FieldLabel htmlFor="bill-date">Tanggal Tagihan</FieldLabel>
          <DateInput
            id="bill-date"
            value={billDate}
            onValueChange={setBillDate}
            aria-invalid={!!err("billDate")}
          />
          <FieldError errors={fieldErrors(err("billDate"))} />
        </Field>
        <Field data-invalid={!!err("dueDate")}>
          <FieldLabel htmlFor="bill-due">Jatuh Tempo</FieldLabel>
          <DateInput
            id="bill-due"
            value={dueDate}
            placeholder="Tanggal tagihan + termin"
            onValueChange={setDueDate}
            aria-invalid={!!err("dueDate")}
          />
          <FieldError errors={fieldErrors(err("dueDate"))} />
        </Field>
        <Field data-invalid={!!err("withholdingType")}>
          <FieldLabel htmlFor="bill-wht">PPh Dipotong</FieldLabel>
          <Select
            value={withholdingType}
            onValueChange={(v) =>
              setWithholdingType((v ?? "none") as WithholdingType)
            }
          >
            <SelectTrigger id="bill-wht" className="w-full">
              <SelectValue>
                {(v: WithholdingType) => WITHHOLDING_LABELS[v]}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {(Object.keys(WITHHOLDING_LABELS) as WithholdingType[]).map(
                (w) => (
                  <SelectItem key={w} value={w}>
                    {WITHHOLDING_LABELS[w]}
                  </SelectItem>
                )
              )}
            </SelectContent>
          </Select>
          <FieldError errors={fieldErrors(err("withholdingType"))} />
        </Field>
        {withholdingType !== "none" ? (
          <Field data-invalid={!!err("withholdingRate")}>
            <FieldLabel htmlFor="bill-rate">Tarif PPh (%)</FieldLabel>
            <Input
              id="bill-rate"
              inputMode="decimal"
              value={withholdingRate}
              aria-invalid={!!err("withholdingRate")}
              onChange={(e) =>
                setWithholdingRate(
                  e.target.value.replace(/[^\d.]/g, "").slice(0, 8)
                )
              }
            />
            <FieldError errors={fieldErrors(err("withholdingRate"))} />
          </Field>
        ) : null}
      </div>
      <Field>
        <FieldLabel htmlFor="bill-note">Catatan</FieldLabel>
        <Textarea
          id="bill-note"
          rows={2}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">Baris Penerimaan yang Ditagih</h2>
        {err("lines") ? (
          <p className="text-sm text-destructive">{err("lines")}</p>
        ) : null}
        {billable.lines.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Semua penerimaan PO ini sudah ditagih.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[64rem]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-12" />
                  <TableHead>Penerimaan / Produk</TableHead>
                  <TableHead className="w-28 text-right">
                    Belum Ditagih
                  </TableHead>
                  <TableHead className="w-32 text-right">Qty Tagihan</TableHead>
                  <TableHead className="w-28 text-right">Harga PO</TableHead>
                  <TableHead className="w-40 text-right">
                    Harga Tagihan
                  </TableHead>
                  <TableHead className="w-36 text-right">Jumlah</TableHead>
                  <TableHead className="w-36">Match</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {billable.lines.map((r) => {
                  const l = lines.find(
                    (x) => x.goodsReceiptLineId === r.goodsReceiptLineId
                  )
                  if (!l) return null
                  const open = new Big(r.openQty).minus(r.pendingQty)
                  const variance = priceVariancePercent(
                    l.unitPrice,
                    r.poUnitPrice
                  )
                  const qtyMismatch =
                    l.include && l.qty !== "" && !new Big(l.qty || "0").eq(open)
                  const priceMismatch =
                    l.include && variance.abs().gt(tolerance)
                  return (
                    <TableRow key={r.goodsReceiptLineId} className="align-top">
                      <TableCell>
                        <input
                          type="checkbox"
                          className="size-5 accent-primary"
                          aria-label={`Tagih ${r.goodsReceiptNumber} ${r.product.sku}`}
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
                        {formatQuantity(open.toFixed(3))}
                        {Number(r.pendingQty) > 0 ? (
                          <div className="text-xs text-muted-foreground">
                            {formatQuantity(r.pendingQty)} menunggu persetujuan
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <QuantityInput
                          aria-label={`Qty tagihan ${r.product.sku}`}
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
                      <TableCell className="text-right">
                        {formatRupiah(r.poUnitPrice)}
                      </TableCell>
                      <TableCell>
                        <MoneyInput
                          aria-label={`Harga tagihan ${r.product.sku}`}
                          value={l.unitPrice}
                          disabled={!l.include}
                          onValueChange={(v) =>
                            update(r.goodsReceiptLineId, { unitPrice: v })
                          }
                        />
                        <LineError
                          message={lineErr(r.goodsReceiptLineId, "unitPrice")}
                        />
                      </TableCell>
                      <TableCell className="text-right">
                        {l.include
                          ? formatRupiah(lineAmount(l.qty, l.unitPrice))
                          : "—"}
                      </TableCell>
                      <TableCell>
                        {!l.include ? null : qtyMismatch || priceMismatch ? (
                          <Badge variant="destructive">
                            {qtyMismatch
                              ? "Selisih qty"
                              : `Harga ${variance.toFixed(2)}%`}
                          </Badge>
                        ) : (
                          <Badge variant="outline">Cocok</Badge>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        )}
        <p className="text-right text-sm">
          Subtotal tagihan:{" "}
          <span className="font-semibold">
            {formatRupiah(subtotal.toFixed(2))}
          </span>
        </p>
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

const BILL_FIELD_LABELS: Record<string, string> = {
  number: "No. tagihan",
  purchaseOrderId: "Purchase Order",
  billDate: "Tanggal tagihan",
  dueDate: "Jatuh tempo",
  subtotalAmount: "Subtotal",
  withholdingType: "PPh",
  withholdingAmount: "PPh dipotong",
  advanceAppliedAmount: "Potongan uang muka",
  totalPayableAmount: "Utang",
  matchStatus: "3-way match",
  status: "Status",
  note: "Catatan",
  lines: "Baris",
}

type DetailProps = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail tagihan pemasok: hasil 3-way match per baris, persetujuan selisih, posting A-18, Riwayat. */
export function BillDetailPage({ id, tab, onTabChange }: DetailProps) {
  const bill = useQuery(billQuery(id))
  return (
    <LoadingArea loading={bill.isPending}>
      {bill.data ? (
        <BillDetail bill={bill.data} tab={tab} onTabChange={onTabChange} />
      ) : null}
    </LoadingArea>
  )
}

function BillDetail({
  bill,
  tab,
  onTabChange,
}: {
  bill: SupplierBill
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<"post" | "revise" | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const can = (a: SupplierBill["allowedActions"][number]) =>
    bill.allowedActions.includes(a)
  const exceeds = bill.lines.some((l) => l.exceedsTolerance)

  const mutation = useIdempotentMutation({
    mutationFn: (
      v: { kind: "post" | "revise" | "cancel"; reason?: string },
      key
    ) => billAction(v.kind, bill.id, bill.version, key, v.reason),
    onSuccess: async (saved, v) => {
      setConfirm(null)
      setCancelling(false)
      toast.success(
        v.kind === "post"
          ? saved.status === "pending_approval"
            ? "Tagihan diajukan ke Head Finance & Accounting"
            : "Tagihan diposting"
          : v.kind === "revise"
            ? "Tagihan kembali ke Draft"
            : "Tagihan dibatalkan"
      )
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      await queryClient.invalidateQueries({ queryKey: ["approval"] })
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {bill.number}
            </h1>
            <StatusBadge status={bill.status} />
            <Badge
              variant={
                bill.matchStatus === "exceeds_tolerance"
                  ? "destructive"
                  : "outline"
              }
            >
              {MATCH_STATUS_LABELS[bill.matchStatus]}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            Tagihan Pemasok · {bill.purchaseOrderNumber} · {bill.supplierName}
          </p>
          {bill.paymentBlocked &&
          bill.status !== "draft" &&
          bill.status !== "cancelled" ? (
            <p className="text-sm text-destructive">
              Pembayaran diblokir sampai selisih disetujui.
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/supplier-bills/$id/edit"
                  params={{ id: bill.id }}
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
          {can("post") ? (
            <Button onClick={() => setConfirm("post")}>Posting Tagihan</Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian tagihan"
        value={tab}
        onValueChange={onTabChange}
        tabs={[
          { id: "summary", label: "Ringkasan" },
          { id: "history", label: "Riwayat" },
        ]}
      />
      {tab === "summary" ? (
        <>
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info
              label="Purchase Order"
              value={
                <Link
                  to="/procurement/purchase-orders/$id"
                  params={{ id: bill.purchaseOrderId }}
                  search={{}}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  {bill.purchaseOrderNumber}
                </Link>
              }
            />
            <Info label="Pemasok" value={bill.supplierName} />
            <Info label="Tanggal Tagihan" value={formatDate(bill.billDate)} />
            <Info label="Jatuh Tempo" value={formatDate(bill.dueDate)} />
            <Info label="Subtotal" value={formatRupiah(bill.subtotalAmount)} />
            <Info
              label="Barang Diterima Belum Ditagih (2102)"
              value={formatRupiah(bill.grniAmount)}
            />
            <Info
              label="Selisih Harga (5103)"
              value={formatRupiah(bill.priceVarianceAmount)}
            />
            <Info
              label={`${WITHHOLDING_LABELS[bill.withholdingType]}${bill.withholdingType === "none" ? "" : ` ${formatPercent(bill.withholdingRate)}`}`}
              value={formatRupiah(bill.withholdingAmount)}
            />
            <Info
              label="Potongan Uang Muka"
              value={formatRupiah(bill.advanceAppliedAmount)}
            />
            <Info label="Utang" value={formatRupiah(bill.totalPayableAmount)} />
            <Info label="Sudah Dibayar" value={formatRupiah(bill.paidAmount)} />
            <Info label="Sisa Utang" value={formatRupiah(bill.openAmount)} />
            <Info
              label="Diposting"
              value={bill.postedAt ? formatDateTime(bill.postedAt) : "—"}
            />
            {bill.journalEntryId ? (
              <Info
                label="Jurnal A-18"
                value={
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: bill.journalEntryId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                }
              />
            ) : null}
            {bill.advanceJournalEntryId ? (
              <Info
                label="Jurnal A-19 (potong uang muka)"
                value={
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: bill.advanceJournalEntryId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                }
              />
            ) : null}
            <Info label="Catatan" value={bill.note || "—"} />
            {bill.cancelReason ? (
              <Info label="Alasan Dibatalkan" value={bill.cancelReason} />
            ) : null}
          </dl>
          <section className="flex flex-col gap-2">
            <h2 className="text-sm font-semibold">
              3-Way Match PO – Penerimaan – Tagihan
            </h2>
            <p className="text-xs text-muted-foreground">
              Toleransi qty {formatPercent(bill.qtyTolerancePercent)}, harga{" "}
              {formatPercent(bill.priceTolerancePercent)}.
            </p>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[64rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Penerimaan / Produk</TableHead>
                    <TableHead className="text-right">Qty GR</TableHead>
                    <TableHead className="text-right">Qty Tagihan</TableHead>
                    <TableHead className="text-right">Harga PO</TableHead>
                    <TableHead className="text-right">Harga Tagihan</TableHead>
                    <TableHead className="text-right">Selisih</TableHead>
                    <TableHead className="text-right">Jumlah</TableHead>
                    <TableHead>Match</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {bill.lines.map((l) => (
                    <TableRow key={l.id}>
                      <TableCell>
                        <Link
                          to="/procurement/goods-receipts/$id"
                          params={{ id: l.goodsReceiptId }}
                          search={{}}
                          className="font-medium text-primary underline-offset-4 hover:underline"
                        >
                          {l.goodsReceiptNumber}
                        </Link>
                        <div className="text-xs text-muted-foreground">
                          {l.product.sku} · {l.product.name}
                          {l.batchNumber ? ` · ${l.batchNumber}` : ""}
                        </div>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.receiptOpenQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.qty)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatRupiah(l.poUnitPrice)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatRupiah(l.unitPrice)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.priceVarianceAmount)}
                        <div className="text-xs text-muted-foreground">
                          {formatPercent(l.priceVariancePercent)}
                        </div>
                      </TableCell>
                      <TableCell className="text-right">
                        {formatRupiah(l.amount)}
                      </TableCell>
                      <TableCell>
                        {l.exceedsTolerance ? (
                          <Badge variant="destructive">
                            {[
                              l.qtyMismatch ? "Qty" : "",
                              l.priceMismatch ? "Harga" : "",
                            ]
                              .filter(Boolean)
                              .join(" & ")}{" "}
                            di luar toleransi
                          </Badge>
                        ) : (
                          <Badge variant="outline">Cocok</Badge>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </section>
          {bill.advances.length > 0 ? (
            <section className="flex flex-col gap-1 text-sm">
              <h2 className="font-semibold">Uang Muka Dipotong</h2>
              {bill.advances.map((a) => (
                <p key={a.advanceId}>
                  {a.advanceNumber}: {formatRupiah(a.amount)}
                </p>
              ))}
            </section>
          ) : null}
          {bill.submittedAt || exceeds ? (
            <ApprovalPanel
              subjectType="supplier_bill_exception"
              subjectId={bill.id}
            />
          ) : null}
        </>
      ) : (
        <EntityHistory
          entityType="supplier_bill"
          entityId={bill.id}
          fieldLabels={BILL_FIELD_LABELS}
        />
      )}
      <ConfirmDialog
        open={confirm === "post"}
        onOpenChange={(o) => setConfirm(o ? "post" : null)}
        title="Posting tagihan pemasok?"
        description={
          exceeds
            ? "Ada baris di luar toleransi 3-way match. Tagihan diajukan ke Head Finance & Accounting; jurnal dan utang dibentuk setelah disetujui, pembayaran diblokir sampai saat itu."
            : "Jurnal A-18 dibuat (Barang Diterima Belum Ditagih → Utang Usaha, PPh sesuai pemasok), uang muka PO dipotong otomatis, dan utang terbentuk."
        }
        confirmLabel={
          exceeds ? "Ajukan Persetujuan Selisih" : "Posting Tagihan"
        }
        loading={mutation.isPending}
        onConfirm={() => mutation.mutate({ kind: "post" })}
      />
      <ConfirmDialog
        open={confirm === "revise"}
        onOpenChange={(o) => setConfirm(o ? "revise" : null)}
        title="Revisi tagihan?"
        description="Tagihan yang ditolak kembali menjadi Draft agar bisa diperbaiki."
        confirmLabel="Revisi"
        loading={mutation.isPending}
        onConfirm={() => mutation.mutate({ kind: "revise" })}
      />
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan tagihan?"
        description="Tagihan Draft atau Ditolak dibatalkan; nomor tagihan bisa dipakai lagi."
        confirmLabel="Batalkan Tagihan"
        required={false}
        destructive
        loading={mutation.isPending}
        onConfirm={(reason) => mutation.mutate({ kind: "cancel", reason })}
      />
    </div>
  )
}
