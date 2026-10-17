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
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  inboundLocationsQuery,
  ordersByStatusQuery,
  RECEIPT_STATUSES,
  RECEIVABLE_ORDER_STATUSES,
  receiptAction,
  receiptQuery,
  receiptsQuery,
  REJECT_DISPOSITION_LABELS,
  saveReceipt,
  type GoodsReceipt,
  type GoodsReceiptSummary,
  type RejectDisposition,
} from "../api/documents-api"
import {
  ORDER_TYPE_LABELS,
  orderQuery,
  type PurchaseOrder,
} from "../api/procurement-api"
import { DocumentToolbar } from "./document-toolbar"
import type { DetailTab } from "./request-detail-page"
import {
  fieldErrors,
  Info,
  LineError,
  OptionSelect,
  ReasonDialog,
} from "./shared"

const listRoute = getRouteApi("/_app/procurement/goods-receipts/")

/** Pengadaan › Penerimaan Barang: daftar GR per PO (S-206, S-207). */
export function ReceiptListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const receipts = useQuery(receiptsQuery(search))

  const columns: DataTableColumn<GoodsReceiptSummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (r) => (
        <Link
          to="/procurement/goods-receipts/$id"
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
            {r.supplierName} · {ORDER_TYPE_LABELS[r.orderType]}
          </span>
        </div>
      ),
    },
    { id: "dn", header: "Surat Jalan", cell: (r) => r.deliveryNote || "—" },
    {
      id: "value",
      header: "Nilai Persediaan",
      align: "right",
      cell: (r) => formatRupiah(r.inventoryAmount),
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
            Penerimaan Barang
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Pencatatan barang yang diterima terhadap Purchase Order (sebagian
            boleh). Barang baik masuk lokasi Inbound berstatus Karantina sampai
            QC; posting membuat jurnal A-01 (atau A-03 untuk FG maklon).
          </p>
        </div>
        {can("procurement.goods_receipt.write") ? (
          <Button
            render={<Link to="/procurement/goods-receipts/new" search={{}} />}
          >
            <PlusIcon /> Terima Barang
          </Button>
        ) : null}
      </div>
      <DocumentToolbar
        searchLabel="Cari penerimaan"
        searchPlaceholder="Cari nomor GR, PO, pemasok, surat jalan"
        q={search.q}
        status={search.status}
        statuses={RECEIPT_STATUSES}
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
        rows={receipts.data?.items}
        getRowId={(r) => r.id}
        loading={receipts.isPending}
        fetching={receipts.isFetching}
        page={search.page}
        totalPages={receipts.data?.totalPages ?? 0}
        totalItems={receipts.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada penerimaan barang."
      />
    </div>
  )
}

// --- Form ---------------------------------------------------------------------------------------

type LineDraft = {
  orderLineId: string
  goodQty: string
  rejectQty: string
  rejectDisposition: RejectDisposition
  batchNumber: string
  productionDate: string
  expiryDate: string
  note: string
}

type MaterialDraft = {
  productId: string
  usedGoodQty: string
  usedRejectQty: string
}

const isToll = (o: PurchaseOrder) =>
  o.orderType === "toll_manufacturing" || o.orderType === "reprocess"

/** Buat Penerimaan Barang (opsional dari detail PO). */
export function ReceiptCreatePage({
  purchaseOrderId,
}: {
  purchaseOrderId?: string
}) {
  const [order, setOrder] = useState<PickerOption | null>(null)
  const orderId = order?.id ?? purchaseOrderId ?? ""
  const po = useQuery({ ...orderQuery(orderId), enabled: !!orderId })
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Terima Barang</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Pilih Purchase Order yang disetujui, isi qty baik dan reject per baris
          beserta batch, tanggal produksi, dan kedaluwarsa. Qty diterima tidak
          boleh melebihi sisa PO (toleransi 0%).
        </p>
      </div>
      {!purchaseOrderId ? (
        <Field className="max-w-md">
          <FieldLabel htmlFor="gr-po">Purchase Order</FieldLabel>
          <SearchPicker
            id="gr-po"
            value={order}
            onValueChange={setOrder}
            query={(q) => ordersByStatusQuery(q, RECEIVABLE_ORDER_STATUSES)}
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
        <LoadingArea loading={po.isPending}>
          {po.data ? <ReceiptForm key={po.data.id} order={po.data} /> : null}
        </LoadingArea>
      ) : null}
    </div>
  )
}

/** Ubah draft Penerimaan Barang. */
export function ReceiptEditPage({ id }: { id: string }) {
  const receipt = useQuery(receiptQuery(id))
  const po = useQuery({
    ...orderQuery(receipt.data?.purchaseOrderId ?? ""),
    enabled: !!receipt.data,
  })
  return (
    <LoadingArea loading={receipt.isPending || po.isPending}>
      {receipt.data && po.data ? (
        receipt.data.allowedActions.includes("edit") ? (
          <div className="flex flex-col gap-4">
            <h1 className="text-2xl font-semibold tracking-tight">
              Ubah Draft Penerimaan {receipt.data.purchaseOrderNumber}
            </h1>
            <ReceiptForm order={po.data} receipt={receipt.data} />
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            Hanya Penerimaan Barang berstatus Draft yang bisa diubah.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

function initialLines(
  order: PurchaseOrder,
  receipt?: GoodsReceipt
): LineDraft[] {
  return order.lines
    .filter((l) => l.product && (receipt || Number(l.outstandingQty) > 0))
    .map((l) => {
      const r = receipt?.lines.find((x) => x.purchaseOrderLineId === l.id)
      return {
        orderLineId: l.id,
        goodQty: r?.goodQty ?? "",
        rejectQty: r && Number(r.rejectQty) > 0 ? r.rejectQty : "",
        rejectDisposition: r?.rejectDisposition ?? "absorbed",
        batchNumber: r?.batchNumber ?? "",
        productionDate: r?.productionDate ?? "",
        expiryDate: r?.expiryDate ?? "",
        note: r?.note ?? "",
      }
    })
}

function ReceiptForm({
  order,
  receipt,
}: {
  order: PurchaseOrder
  receipt?: GoodsReceipt
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const locations = useQuery(inboundLocationsQuery)
  const toll = isToll(order)
  const [businessDate, setBusinessDate] = useState(
    receipt?.businessDate ?? todayBusinessDate()
  )
  const [locationId, setLocationId] = useState(receipt?.location.id ?? "")
  const [deliveryNote, setDeliveryNote] = useState(receipt?.deliveryNote ?? "")
  const [note, setNote] = useState(receipt?.note ?? "")
  const [lines, setLines] = useState(() => initialLines(order, receipt))
  const [materials, setMaterials] = useState<MaterialDraft[]>(() =>
    order.materials.map((m) => {
      const r = receipt?.materials.find((x) => x.product.id === m.product.id)
      return {
        productId: m.product.id,
        usedGoodQty: r?.usedGoodQty ?? "",
        usedRejectQty: r?.usedRejectQty ?? "",
      }
    })
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const filled = lines.filter((l) => l.goodQty !== "" && Number(l.goodQty) > 0)

  const update = (id: string, patch: Partial<LineDraft>) =>
    setLines((ls) =>
      ls.map((l) => (l.orderLineId === id ? { ...l, ...patch } : l))
    )
  const updateMaterial = (id: string, patch: Partial<MaterialDraft>) =>
    setMaterials((ms) =>
      ms.map((m) => (m.productId === id ? { ...m, ...patch } : m))
    )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      saveReceipt(
        receipt,
        order.id,
        {
          businessDate,
          locationId,
          deliveryNote,
          note,
          lines: filled.map((l) => ({
            purchaseOrderLineId: l.orderLineId,
            goodQty: l.goodQty,
            rejectQty: l.rejectQty || "0",
            rejectDisposition: l.rejectDisposition,
            batchNumber: l.batchNumber || undefined,
            productionDate: l.productionDate || null,
            expiryDate: l.expiryDate || null,
            note: l.note,
          })),
          materials: toll
            ? materials
                .filter(
                  (m) =>
                    Number(m.usedGoodQty || 0) + Number(m.usedRejectQty || 0) >
                    0
                )
                .map((m) => ({
                  productId: m.productId,
                  usedGoodQty: m.usedGoodQty || "0",
                  usedRejectQty: m.usedRejectQty || "0",
                }))
            : [],
        },
        key
      ),
    onSuccess: async (saved) => {
      toast.success("Draft penerimaan disimpan")
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      void navigate({
        to: "/procurement/goods-receipts/$id",
        params: { id: saved.id },
        search: {},
      })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        // Field baris dari server memakai indeks baris yang dikirim; petakan ke baris PO.
        const mapped: Record<string, string> = {}
        for (const [k, v] of Object.entries(problem.fields)) {
          const m = /^lines\.(\d+)\.(.+)$/.exec(k)
          if (m && filled[Number(m[1])])
            mapped[`${filled[Number(m[1])].orderLineId}.${m[2]}`] = v
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
    if (!locationId) next.locationId = "Lokasi Inbound wajib dipilih"
    if (!businessDate) next.businessDate = "Tanggal terima wajib diisi"
    if (filled.length === 0) next.lines = "Isi qty baik minimal satu baris"
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
        <Info label="Purchase Order" value={order.number ?? "—"} />
        <Info label="Pemasok" value={order.supplierName} />
        <Info label="Jenis" value={order.orderTypeLabel} />
        {order.tollLocation ? (
          <Info
            label="Lokasi Maklon"
            value={`${order.tollLocation.code} · ${order.tollLocation.name}`}
          />
        ) : null}
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Field data-invalid={!!err("businessDate")}>
          <FieldLabel htmlFor="gr-date">Tanggal Terima</FieldLabel>
          <DateInput
            id="gr-date"
            value={businessDate}
            onValueChange={setBusinessDate}
            aria-invalid={!!err("businessDate")}
          />
          <FieldError errors={fieldErrors(err("businessDate"))} />
        </Field>
        <Field data-invalid={!!err("locationId")}>
          <FieldLabel htmlFor="gr-location">Lokasi Inbound</FieldLabel>
          <OptionSelect
            id="gr-location"
            value={locationId}
            onValueChange={setLocationId}
            options={(locations.data ?? []).map((l) => ({
              id: l.id,
              code: l.code,
              name: `${l.code} · ${l.name}`,
            }))}
            placeholder="Pilih lokasi Inbound"
            aria-invalid={!!err("locationId")}
          />
          <FieldError errors={fieldErrors(err("locationId"))} />
        </Field>
        <Field data-invalid={!!err("deliveryNote")}>
          <FieldLabel htmlFor="gr-dn">No. Surat Jalan</FieldLabel>
          <Input
            id="gr-dn"
            value={deliveryNote}
            maxLength={100}
            onChange={(e) => setDeliveryNote(e.target.value)}
          />
          <FieldError errors={fieldErrors(err("deliveryNote"))} />
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor="gr-note">Catatan</FieldLabel>
        <Textarea
          id="gr-note"
          rows={2}
          maxLength={1000}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">
          {toll ? "FG dari Maklon" : "Barang Diterima"}
        </h2>
        {err("lines") ? (
          <p className="text-sm text-destructive">{err("lines")}</p>
        ) : null}
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[72rem]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-60">Produk</TableHead>
                <TableHead className="w-28 text-right">Sisa PO</TableHead>
                <TableHead className="w-32 text-right">Qty Baik</TableHead>
                <TableHead className="w-32 text-right">Qty Reject</TableHead>
                <TableHead className="w-56">Perlakuan Reject</TableHead>
                <TableHead className="w-36">No. Batch</TableHead>
                <TableHead className="w-40">Tgl Produksi</TableHead>
                <TableHead className="w-40">Tgl Kedaluwarsa</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {lines.map((l) => {
                const ol = order.lines.find((x) => x.id === l.orderLineId)
                const batch = !!ol?.product?.hasBatch
                return (
                  <TableRow key={l.orderLineId} className="align-top">
                    <TableCell>
                      <div className="font-medium">{ol?.product?.sku}</div>
                      <div className="text-xs text-muted-foreground">
                        {ol?.product?.name}
                      </div>
                      <LineError
                        message={lineErr(l.orderLineId, "orderLineId")}
                      />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatQuantity(ol?.outstandingQty)} {ol?.product?.uom}
                    </TableCell>
                    <TableCell>
                      <QuantityInput
                        aria-label={`Qty baik ${ol?.product?.sku ?? ""}`}
                        value={l.goodQty}
                        onValueChange={(v) =>
                          update(l.orderLineId, { goodQty: v })
                        }
                        aria-invalid={!!lineErr(l.orderLineId, "goodQty")}
                      />
                      <LineError message={lineErr(l.orderLineId, "goodQty")} />
                    </TableCell>
                    <TableCell>
                      <QuantityInput
                        aria-label={`Qty reject ${ol?.product?.sku ?? ""}`}
                        value={l.rejectQty}
                        onValueChange={(v) =>
                          update(l.orderLineId, { rejectQty: v })
                        }
                      />
                      <LineError
                        message={lineErr(l.orderLineId, "rejectQty")}
                      />
                    </TableCell>
                    <TableCell>
                      <Select
                        value={l.rejectDisposition}
                        onValueChange={(v) =>
                          update(l.orderLineId, {
                            rejectDisposition: (v ??
                              "absorbed") as RejectDisposition,
                          })
                        }
                      >
                        <SelectTrigger
                          className="w-full"
                          aria-label="Perlakuan reject"
                        >
                          <SelectValue>
                            {(v: RejectDisposition) =>
                              REJECT_DISPOSITION_LABELS[v]
                            }
                          </SelectValue>
                        </SelectTrigger>
                        <SelectContent>
                          {(
                            Object.keys(
                              REJECT_DISPOSITION_LABELS
                            ) as RejectDisposition[]
                          ).map((d) => (
                            <SelectItem key={d} value={d}>
                              {REJECT_DISPOSITION_LABELS[d]}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell>
                      {batch ? (
                        <>
                          <Input
                            aria-label="Nomor batch"
                            value={l.batchNumber}
                            maxLength={40}
                            aria-invalid={
                              !!lineErr(l.orderLineId, "batchNumber")
                            }
                            onChange={(e) =>
                              update(l.orderLineId, {
                                batchNumber: e.target.value.toUpperCase(),
                              })
                            }
                          />
                          <LineError
                            message={lineErr(l.orderLineId, "batchNumber")}
                          />
                        </>
                      ) : (
                        <span className="text-xs text-muted-foreground">
                          Tanpa batch
                        </span>
                      )}
                    </TableCell>
                    <TableCell>
                      {batch ? (
                        <>
                          <DateInput
                            value={l.productionDate}
                            onValueChange={(v) =>
                              update(l.orderLineId, { productionDate: v })
                            }
                            aria-invalid={
                              !!lineErr(l.orderLineId, "productionDate")
                            }
                          />
                          <LineError
                            message={lineErr(l.orderLineId, "productionDate")}
                          />
                        </>
                      ) : null}
                    </TableCell>
                    <TableCell>
                      {batch ? (
                        <>
                          <DateInput
                            value={l.expiryDate}
                            onValueChange={(v) =>
                              update(l.orderLineId, { expiryDate: v })
                            }
                            aria-invalid={
                              !!lineErr(l.orderLineId, "expiryDate")
                            }
                          />
                          <LineError
                            message={lineErr(l.orderLineId, "expiryDate")}
                          />
                        </>
                      ) : null}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      </section>
      {toll ? (
        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-semibold">
            Pemakaian{" "}
            {order.orderType === "reprocess" ? "Bahan GM" : "Packaging"} di
            Maklon
          </h2>
          <p className="text-sm text-muted-foreground">
            Isi sesuai laporan maklon: qty terpakai baik + reject keluar dari
            lokasi maklon dan masuk HPP batch; sisanya tetap di lokasi maklon.
          </p>
          {err("materials") ? (
            <p className="text-sm text-destructive">{err("materials")}</p>
          ) : null}
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[40rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Bahan</TableHead>
                  <TableHead className="w-28 text-right">Rencana</TableHead>
                  <TableHead className="w-36 text-right">
                    Terpakai Baik
                  </TableHead>
                  <TableHead className="w-36 text-right">
                    Terpakai Reject
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {materials.map((m) => {
                  const pm = order.materials.find(
                    (x) => x.product.id === m.productId
                  )
                  return (
                    <TableRow key={m.productId}>
                      <TableCell>
                        <div className="font-medium">{pm?.product.sku}</div>
                        <div className="text-xs text-muted-foreground">
                          {pm?.product.name}
                        </div>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(pm?.plannedQty)}
                      </TableCell>
                      <TableCell>
                        <QuantityInput
                          aria-label={`Terpakai baik ${pm?.product.sku ?? ""}`}
                          value={m.usedGoodQty}
                          onValueChange={(v) =>
                            updateMaterial(m.productId, { usedGoodQty: v })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <QuantityInput
                          aria-label={`Terpakai reject ${pm?.product.sku ?? ""}`}
                          value={m.usedRejectQty}
                          onValueChange={(v) =>
                            updateMaterial(m.productId, { usedRejectQty: v })
                          }
                        />
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        </section>
      ) : null}
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

const RECEIPT_FIELD_LABELS: Record<string, string> = {
  purchaseOrderId: "Purchase Order",
  businessDate: "Tanggal terima",
  locationId: "Lokasi Inbound",
  deliveryNote: "Surat jalan",
  note: "Catatan",
  status: "Status",
  inventoryAmount: "Nilai persediaan",
  grniAmount: "Barang diterima belum ditagih",
  cancelReason: "Alasan dibatalkan",
  lines: "Baris",
  materials: "Pemakaian bahan",
}

type DetailProps = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail Penerimaan Barang: posting (stok Karantina + jurnal), pembatalan, Riwayat. */
export function ReceiptDetailPage({ id, tab, onTabChange }: DetailProps) {
  const receipt = useQuery(receiptQuery(id))
  return (
    <LoadingArea loading={receipt.isPending}>
      {receipt.data ? (
        <ReceiptDetail
          receipt={receipt.data}
          tab={tab}
          onTabChange={onTabChange}
        />
      ) : null}
    </LoadingArea>
  )
}

function ReceiptDetail({
  receipt,
  tab,
  onTabChange,
}: {
  receipt: GoodsReceipt
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const [posting, setPosting] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const can = (a: GoodsReceipt["allowedActions"][number]) =>
    receipt.allowedActions.includes(a)
  const toll =
    receipt.orderType === "toll_manufacturing" ||
    receipt.orderType === "reprocess"

  const mutation = useIdempotentMutation({
    mutationFn: (v: { kind: "post" | "cancel"; reason?: string }, key) =>
      receiptAction(v.kind, receipt.id, receipt.version, key, v.reason),
    onSuccess: async (_, v) => {
      setPosting(false)
      setCancelling(false)
      toast.success(
        v.kind === "post" ? "Penerimaan diposting" : "Penerimaan dibatalkan"
      )
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
    },
    onError: (err) => {
      setPosting(false)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {receipt.number ?? "Draft Penerimaan"}
            </h1>
            <StatusBadge status={receipt.status} />
            <Badge variant="secondary">
              {ORDER_TYPE_LABELS[receipt.orderType]}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            Penerimaan Barang · {receipt.purchaseOrderNumber} ·{" "}
            {receipt.supplierName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/goods-receipts/$id/edit"
                  params={{ id: receipt.id }}
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
          {can("post") ? (
            <Button onClick={() => setPosting(true)}>Posting Penerimaan</Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian penerimaan"
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
                  params={{ id: receipt.purchaseOrderId }}
                  search={{}}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  {receipt.purchaseOrderNumber}
                </Link>
              }
            />
            <Info label="Pemasok" value={receipt.supplierName} />
            <Info label="Divisi" value={receipt.divisionName} />
            <Info
              label="Tanggal Terima"
              value={formatDate(receipt.businessDate)}
            />
            <Info
              label="Lokasi Inbound"
              value={`${receipt.location.code} · ${receipt.location.name}`}
            />
            <Info label="Surat Jalan" value={receipt.deliveryNote || "—"} />
            {receipt.tollLocation ? (
              <Info
                label="Lokasi Maklon"
                value={`${receipt.tollLocation.code} · ${receipt.tollLocation.name}`}
              />
            ) : null}
            <Info
              label="Nilai Persediaan"
              value={formatRupiah(receipt.inventoryAmount)}
            />
            {toll ? (
              <Info
                label="Packaging/Bahan Terpakai"
                value={formatRupiah(receipt.materialAmount)}
              />
            ) : null}
            <Info
              label="Barang Diterima Belum Ditagih (2102)"
              value={formatRupiah(receipt.grniAmount)}
            />
            <Info
              label="Diposting"
              value={receipt.postedAt ? formatDateTime(receipt.postedAt) : "—"}
            />
            {receipt.journalEntryId ? (
              <Info
                label={toll ? "Jurnal A-03" : "Jurnal A-01"}
                value={
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: receipt.journalEntryId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                }
              />
            ) : null}
            {receipt.reversalJournalEntryId ? (
              <Info
                label="Jurnal Balik"
                value={
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: receipt.reversalJournalEntryId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal balik
                  </Link>
                }
              />
            ) : null}
            <Info label="Dibuat oleh" value={receipt.createdByName || "—"} />
            <Info label="Catatan" value={receipt.note || "—"} />
            {receipt.cancelReason ? (
              <Info label="Alasan Dibatalkan" value={receipt.cancelReason} />
            ) : null}
          </dl>
          <section className="flex flex-col gap-2">
            <h2 className="text-sm font-semibold">Baris</h2>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[64rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Produk</TableHead>
                    <TableHead className="text-right">Baik</TableHead>
                    <TableHead className="text-right">Reject</TableHead>
                    <TableHead>Batch</TableHead>
                    <TableHead className="text-right">
                      {toll ? "Harga Jasa" : "Harga PO"}
                    </TableHead>
                    <TableHead className="text-right">Nilai Batch</TableHead>
                    <TableHead className="text-right">HPP / Unit</TableHead>
                    <TableHead className="text-right">Ditagih</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {receipt.lines.map((l) => (
                    <TableRow key={l.id}>
                      <TableCell>
                        <div className="font-medium">{l.product.sku}</div>
                        <div className="text-xs text-muted-foreground">
                          {l.product.name}
                        </div>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.goodQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.rejectQty)}
                        {Number(l.rejectQty) > 0 ? (
                          <div className="text-xs text-muted-foreground">
                            {REJECT_DISPOSITION_LABELS[l.rejectDisposition]}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        {l.batchNumber ? (
                          <div className="flex flex-col">
                            <span>{l.batchNumber}</span>
                            <span className="text-xs text-muted-foreground">
                              {l.productionDate
                                ? formatDate(l.productionDate)
                                : "—"}{" "}
                              – {l.expiryDate ? formatDate(l.expiryDate) : "—"}
                            </span>
                          </div>
                        ) : (
                          "—"
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatRupiah(l.unitPrice)}
                      </TableCell>
                      <TableCell className="text-right">
                        {formatRupiah(l.inventoryAmount)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {receipt.status === "draft"
                          ? "—"
                          : formatRupiah(l.unitCost)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.billedQty)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </section>
          {receipt.materials.length > 0 ? (
            <section className="flex flex-col gap-2">
              <h2 className="text-sm font-semibold">
                Pemakaian Bahan di Maklon
              </h2>
              <div className="overflow-x-auto rounded-lg border">
                <Table className="min-w-[40rem]">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Bahan</TableHead>
                      <TableHead className="text-right">Baik</TableHead>
                      <TableHead className="text-right">Reject</TableHead>
                      <TableHead className="text-right">HPP Terpakai</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {receipt.materials.map((m) => (
                      <TableRow key={m.id}>
                        <TableCell>
                          {m.product.sku} · {m.product.name}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatQuantity(m.usedGoodQty)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatQuantity(m.usedRejectQty)}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatRupiah(m.costAmount)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </section>
          ) : null}
        </>
      ) : (
        <EntityHistory
          entityType="goods_receipt"
          entityId={receipt.id}
          fieldLabels={RECEIPT_FIELD_LABELS}
        />
      )}
      <ConfirmDialog
        open={posting}
        onOpenChange={setPosting}
        title="Posting penerimaan barang?"
        description={
          toll
            ? "Nomor GR diberikan, packaging/bahan terpakai keluar dari lokasi maklon, FG baik masuk Inbound berstatus Karantina, dan jurnal A-03 dibuat."
            : "Nomor GR diberikan, barang baik masuk Inbound berstatus Karantina, dan jurnal A-01 (persediaan / Barang Diterima Belum Ditagih) dibuat."
        }
        confirmLabel="Posting Penerimaan"
        loading={mutation.isPending}
        onConfirm={() => mutation.mutate({ kind: "post" })}
      />
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan penerimaan?"
        description={
          receipt.status === "posted"
            ? "Stok Karantina dikeluarkan lagi, packaging maklon dikembalikan, jurnal dibalik, dan qty diterima PO berkurang. Hanya penerimaan yang belum ditagih, diretur, atau di-QC."
            : "Draft penerimaan dibatalkan."
        }
        confirmLabel="Batalkan Penerimaan"
        required={receipt.status === "posted"}
        destructive
        loading={mutation.isPending}
        onConfirm={(reason) => mutation.mutate({ kind: "cancel", reason })}
      />
    </div>
  )
}
