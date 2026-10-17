import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, PrinterIcon, SearchIcon, TagIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { PageTabs } from "@/components/common/page-tabs"
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
import { UserPicker } from "@/features/approval/components/user-picker"
import type { ApprovalUserOption } from "@/features/approval/api/approval-api"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  batchText,
  cancelDeliveryOrder,
  couriersQuery,
  createDeliveryOrder,
  DELIVERY_TYPE_LABELS,
  deliverDeliveryOrder,
  deliveryOrderQuery,
  deliveryOrdersQuery,
  releaseDeliveryOrder,
  requestProofUpload,
  SHIPMENT_STATUS_LABELS,
  shipDeliveryOrder,
  shippableOrdersQuery,
  type DeliveryOrder,
  type DeliveryOrderStatus,
  type DeliveryOrderSummary,
} from "../api/logistics-api"

const listRoute = getRouteApi("/_app/logistics/delivery-orders/")
const ALL = "all"

const STATUS_OPTIONS: [DeliveryOrderStatus, string][] = [
  ["draft", "Draft"],
  ["in_progress", "Diproses"],
  ["in_transit", "Dalam Pengiriman"],
  ["received", "Diterima"],
  ["cancelled", "Dibatalkan"],
]

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

/** Logistik › Delivery Order (S-309). */
export function DeliveryOrderListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const list = useQuery(deliveryOrdersQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const [creating, setCreating] = useState(false)
  const columns: DataTableColumn<DeliveryOrderSummary>[] = [
    {
      id: "number",
      header: "Nomor DO",
      cell: (d) => (
        <Link
          to="/logistics/delivery-orders/$id"
          params={{ id: d.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {d.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (d) => (
        <span className="whitespace-nowrap">{formatDate(d.businessDate)}</span>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (d) => DELIVERY_TYPE_LABELS[d.deliveryType],
    },
    { id: "source", header: "Dokumen Asal", cell: (d) => d.sourceLabel },
    {
      id: "recipient",
      header: "Penerima",
      cell: (d) => d.recipientName || "—",
    },
    {
      id: "courier",
      header: "Kurir & Resi",
      cell: (d) =>
        d.courierName ? (
          <div className="flex flex-col">
            <span>{d.courierName}</span>
            <span className="text-xs text-muted-foreground">
              {d.trackingNumber || d.marketplaceAwb || "—"}
            </span>
          </div>
        ) : (
          "—"
        ),
    },
    {
      id: "status",
      header: "Status",
      cell: (d) => <StatusBadge status={d.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Delivery Order
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            DO dibuat dari Sales Order Diproses, di-release Logistik Leader ke
            gudang (Outbound: picking, packing, QC), lalu diserahkan ke kurir.
          </p>
        </div>
        <Button className="min-h-11" onClick={() => setCreating(true)}>
          <PlusIcon /> Buat Delivery Order
        </Button>
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
            aria-label="Cari Delivery Order"
            placeholder="Cari DO, SO, penerima, resi, AWB"
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
                status: v === ALL ? undefined : (v as DeliveryOrderStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                STATUS_OPTIONS.find(([k]) => k === v)?.[1] ?? "Semua status"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {STATUS_OPTIONS.map(([k, l]) => (
              <SelectItem key={k} value={k}>
                {l}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        caption="Daftar Delivery Order"
        columns={columns}
        rows={list.data?.items}
        getRowId={(d) => d.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada Delivery Order."
      />
      {creating ? <CreateDialog onClose={() => setCreating(false)} /> : null}
    </div>
  )
}

function CreateDialog({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const [q, setQ] = useState("")
  const orders = useQuery(shippableOrdersQuery(q))
  const create = useIdempotentMutation({
    mutationFn: (salesOrderId: string, key) =>
      createDeliveryOrder({ salesOrderId }, key),
    onSuccess: (d) => {
      toast.success("Delivery Order dibuat")
      onClose()
      void navigate({
        to: "/logistics/delivery-orders/$id",
        params: { id: d.id },
      })
    },
    onError: onProblem(),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Buat Delivery Order</DialogTitle>
          <DialogDescription>
            Pilih Sales Order berstatus Diproses. Barang dan batch diambil dari
            alokasi stok FEFO pesanan.
          </DialogDescription>
        </DialogHeader>
        <Input
          aria-label="Cari Sales Order"
          placeholder="Cari nomor SO atau pelanggan"
          className="min-h-11"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <LoadingArea loading={orders.isPending}>
          <ul className="flex flex-col gap-2">
            {(orders.data?.items ?? []).map((o) => (
              <li
                key={o.id}
                className="flex items-center justify-between gap-2 rounded-md border p-3 text-sm"
              >
                <div>
                  <div className="font-medium">{o.number}</div>
                  <div className="text-xs text-muted-foreground">
                    {o.partyName} · {formatDate(o.businessDate)} ·{" "}
                    {formatRupiah(o.totalAmount)}
                  </div>
                </div>
                <LoadingButton
                  size="sm"
                  className="min-h-11"
                  loading={create.isPending}
                  onClick={() => create.mutate(o.id)}
                >
                  Pilih
                </LoadingButton>
              </li>
            ))}
            {orders.data && orders.data.items.length === 0 ? (
              <li className="text-sm text-muted-foreground">
                Tidak ada Sales Order yang siap dikirim.
              </li>
            ) : null}
          </ul>
        </LoadingArea>
      </DialogContent>
    </Dialog>
  )
}

type DetailTab = "lines" | "shipment" | "history"

/** Detail DO: barang & batch, Outbound, pengiriman, pelacakan; aksi release, kirim, terima, batal. */
export function DeliveryOrderDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const query = useQuery(deliveryOrderQuery(id))
  const queryClient = useQueryClient()
  const [shipping, setShipping] = useState(false)
  const [delivering, setDelivering] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const release = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      releaseDeliveryOrder(id, query.data?.version ?? 0, key),
    onSuccess: async () => {
      toast.success("DO di-release; Outbound dibuat")
      await queryClient.invalidateQueries({ queryKey: ["logistics"] })
    },
    onError: onProblem(),
  })
  const d = query.data
  if (query.isError) {
    return (
      <p className="text-sm text-destructive">
        Delivery Order tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }
  return (
    <LoadingArea loading={query.isPending}>
      {d ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">
                {d.number ?? "Draft Delivery Order"}
              </h1>
              <StatusBadge status={d.status} />
            </div>
            <div className="flex flex-wrap gap-2">
              {d.allowedActions.includes("release") ? (
                <LoadingButton
                  className="min-h-11"
                  loading={release.isPending}
                  onClick={() => release.mutate()}
                >
                  Release ke Gudang
                </LoadingButton>
              ) : null}
              {d.allowedActions.includes("ship") ? (
                <Button className="min-h-11" onClick={() => setShipping(true)}>
                  Serahkan ke Kurir
                </Button>
              ) : null}
              {d.allowedActions.includes("deliver") ? (
                <Button
                  className="min-h-11"
                  onClick={() => setDelivering(true)}
                >
                  Konfirmasi Diterima
                </Button>
              ) : null}
              {d.number ? (
                <Button
                  variant="outline"
                  className="min-h-11"
                  render={
                    <Link
                      to="/logistics/delivery-orders/$id/print"
                      params={{ id: d.id }}
                    />
                  }
                >
                  <PrinterIcon /> Cetak DO
                </Button>
              ) : null}
              {d.packages.length > 0 || d.productLabels.length > 0 ? (
                <Button
                  variant="outline"
                  className="min-h-11"
                  render={
                    <Link
                      to="/logistics/package-labels"
                      search={{ deliveryOrderId: d.id }}
                    />
                  }
                >
                  <TagIcon /> Label Koli & Produk
                </Button>
              ) : null}
              {d.allowedActions.includes("cancel") ? (
                <Button
                  variant="ghost"
                  className="min-h-11"
                  onClick={() => setCancelling(true)}
                >
                  Batalkan
                </Button>
              ) : null}
            </div>
          </div>
          {d.status === "in_progress" && !d.outboundCompletedAt ? (
            <p className="rounded-md border bg-muted/40 p-3 text-sm">
              Menunggu gudang menyelesaikan Outbound (picking, packing, QC)
              {d.outbound ? (
                <>
                  {" "}
                  —{" "}
                  <Link
                    to="/warehouse/outbounds/$id"
                    params={{ id: d.outbound.id }}
                    className="text-primary underline"
                  >
                    {d.outbound.number}
                  </Link>
                </>
              ) : null}
              {d.outbound?.needsAttention
                ? ". Ada baris ditahan karena stok pengganti kurang."
                : "."}
            </p>
          ) : null}
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info label="Jenis" value={DELIVERY_TYPE_LABELS[d.deliveryType]} />
            <Info label="Dokumen Asal" value={d.sourceLabel} />
            <Info label="Tanggal" value={formatDate(d.businessDate)} />
            <Info
              label="Lokasi Asal"
              value={`${d.location.code} · ${d.location.name}`}
            />
            <Info label="Penerima" value={d.recipientName} />
            <Info label="Telepon" value={d.recipientPhone} />
            <Info label="Alamat" value={d.shipToAddress} />
            <Info
              label="Release"
              value={
                d.releasedAt
                  ? `${d.releasedByName} · ${formatDateTime(d.releasedAt)}`
                  : ""
              }
            />
          </dl>
          <PageTabs
            label="Bagian Delivery Order"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "lines", label: "Barang" },
              { id: "shipment", label: "Pengiriman" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "lines" ? (
            <LinesView d={d} />
          ) : tab === "shipment" ? (
            <ShipmentView d={d} />
          ) : (
            <EntityHistory entityType="delivery_order" entityId={d.id} />
          )}
          {shipping ? (
            <ShipDialog d={d} onClose={() => setShipping(false)} />
          ) : null}
          {delivering ? (
            <DeliverDialog d={d} onClose={() => setDelivering(false)} />
          ) : null}
          {cancelling ? (
            <CancelDialog d={d} onClose={() => setCancelling(false)} />
          ) : null}
        </div>
      ) : null}
    </LoadingArea>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value || "—"}</dd>
    </div>
  )
}

function LinesView({ d }: { d: DeliveryOrder }) {
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[40rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Produk</TableHead>
            <TableHead>Batch</TableHead>
            <TableHead>Kedaluwarsa</TableHead>
            <TableHead className="text-right">Qty</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {d.printLines.map((l, i) => (
            <TableRow key={i}>
              <TableCell>
                <span className="font-medium">{l.product.sku}</span>
                <span className="block text-xs text-muted-foreground">
                  {l.product.name}
                </span>
              </TableCell>
              <TableCell>{batchText(l.batch)}</TableCell>
              <TableCell>
                {l.batch?.expiryDate ? formatDate(l.batch.expiryDate) : "—"}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {Number(l.qty)} {l.product.uom}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function ShipmentView({ d }: { d: DeliveryOrder }) {
  const s = d.shipment
  if (!s) {
    return (
      <p className="text-sm text-muted-foreground">
        Barang belum diserahkan ke kurir.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <Info label="Kurir" value={s.courier.name} />
        <Info
          label="Pengantar (kurir internal)"
          value={s.internalCourierName}
        />
        <Info label="Layanan" value={s.serviceCode} />
        <Info label="Nomor Resi" value={s.trackingNumber} />
        <Info label="AWB Marketplace" value={s.marketplaceAwb} />
        <Info label="Ongkos Kirim" value={formatRupiah(s.shippingCost)} />
        <Info label="Koli" value={String(s.packageCount)} />
        <Info label="Status" value={SHIPMENT_STATUS_LABELS[s.status]} />
        <Info label="Diterima oleh" value={s.receivedByName} />
        <Info
          label="Dikonfirmasi"
          value={
            s.deliveredAt
              ? `${s.confirmedByName} · ${formatDateTime(s.deliveredAt)}`
              : ""
          }
        />
        <Info
          label="Foto/Tanda terima"
          value={s.proofFileIds.length ? `${s.proofFileIds.length} file` : ""}
        />
      </dl>
      <section>
        <h2 className="mb-2 font-medium">Pelacakan</h2>
        <ol className="flex flex-col gap-2 border-l pl-4 text-sm">
          {d.events.map((e) => (
            <li key={e.id}>
              <span className="font-medium">
                {SHIPMENT_STATUS_LABELS[e.status]}
              </span>{" "}
              · {formatDateTime(e.occurredAt)} · {e.createdByName}
              {e.note ? (
                <span className="block text-muted-foreground">{e.note}</span>
              ) : null}
            </li>
          ))}
        </ol>
      </section>
    </div>
  )
}

function ShipDialog({ d, onClose }: { d: DeliveryOrder; onClose: () => void }) {
  const queryClient = useQueryClient()
  const couriers = useQuery(couriersQuery(true))
  const [courierId, setCourierId] = useState("")
  const [merch, setMerch] = useState<ApprovalUserOption | null>(null)
  const [tracking, setTracking] = useState("")
  const [awb, setAwb] = useState("")
  const [service, setService] = useState("")
  const [cost, setCost] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const courier = couriers.data?.items.find((c) => c.id === courierId)
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      shipDeliveryOrder(
        d.id,
        {
          version: d.version,
          courierId,
          internalCourierUserId: courier?.isInternal ? merch?.id : undefined,
          trackingNumber: tracking || undefined,
          marketplaceAwb: awb || undefined,
          serviceCode: service || undefined,
          shippingCost: cost || undefined,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Barang diserahkan ke kurir")
      await queryClient.invalidateQueries({ queryKey: ["logistics"] })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Serahkan ke Kurir</DialogTitle>
          <DialogDescription>
            Hanya barang yang lulus QC Outbound yang dikirim. Stok keluar dan
            jurnal dibuat saat disimpan.
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
          <Field>
            <FieldLabel htmlFor="ship-courier">Kurir</FieldLabel>
            <Select
              value={courierId}
              onValueChange={(v) => setCourierId(String(v))}
            >
              <SelectTrigger id="ship-courier" className="min-h-11 w-full">
                <SelectValue placeholder="Pilih kurir">
                  {(v: string) =>
                    couriers.data?.items.find((c) => c.id === v)?.name ??
                    "Pilih kurir"
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(couriers.data?.items ?? []).map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {errors.courierId ? (
              <FieldError>{errors.courierId}</FieldError>
            ) : null}
          </Field>
          {courier?.isInternal ? (
            <Field>
              <FieldLabel htmlFor="ship-merch">
                Merchandiser pengantar
              </FieldLabel>
              <UserPicker
                id="ship-merch"
                value={merch}
                onValueChange={setMerch}
                placeholder="Pilih Merchandiser"
              />
              {errors.internalCourierUserId ? (
                <FieldError>{errors.internalCourierUserId}</FieldError>
              ) : null}
            </Field>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="ship-tracking">Nomor resi</FieldLabel>
                <Input
                  id="ship-tracking"
                  className="min-h-11"
                  value={tracking}
                  onChange={(e) => setTracking(e.target.value)}
                />
                {errors.trackingNumber ? (
                  <FieldError>{errors.trackingNumber}</FieldError>
                ) : null}
              </Field>
              <Field>
                <FieldLabel htmlFor="ship-service">Layanan</FieldLabel>
                <Input
                  id="ship-service"
                  className="min-h-11"
                  placeholder="REG, YES, Instant"
                  value={service}
                  onChange={(e) => setService(e.target.value)}
                />
              </Field>
            </div>
          )}
          {d.deliveryType === "marketplace" ? (
            <Field>
              <FieldLabel htmlFor="ship-awb">
                AWB / barcode marketplace
              </FieldLabel>
              <Input
                id="ship-awb"
                className="min-h-11"
                value={awb}
                onChange={(e) => setAwb(e.target.value)}
              />
              {errors.marketplaceAwb ? (
                <FieldError>{errors.marketplaceAwb}</FieldError>
              ) : null}
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="ship-cost">Ongkos kirim (opsional)</FieldLabel>
            <MoneyInput id="ship-cost" value={cost} onValueChange={setCost} />
            {errors.shippingCost ? (
              <FieldError>{errors.shippingCost}</FieldError>
            ) : null}
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              disabled={!courierId}
              className="min-h-11"
            >
              Serahkan ke Kurir
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Konfirmasi diterima — dipakai Merchandiser dari HP (foto/tanda terima). */
export function DeliverDialog({
  d,
  onClose,
}: {
  d: DeliveryOrder
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [receivedBy, setReceivedBy] = useState("")
  const [note, setNote] = useState("")
  const [proofs, setProofs] = useState<string[]>([])
  const [errors, setErrors] = useState<Record<string, string>>({})
  const internal = !!d.shipment?.internalCourierUserId
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      deliverDeliveryOrder(
        d.id,
        {
          receivedByName: receivedBy || undefined,
          note: note || undefined,
          proofFileIds: proofs,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Barang diterima")
      await queryClient.invalidateQueries({ queryKey: ["logistics"] })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Konfirmasi Diterima {d.number}</DialogTitle>
          <DialogDescription>
            {d.recipientName} · {d.shipToAddress}
            {internal
              ? ". Kurir internal wajib foto barang diterima atau tanda terima."
              : ""}
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
          <Field>
            <FieldLabel htmlFor="dlv-by">Diterima oleh</FieldLabel>
            <Input
              id="dlv-by"
              className="min-h-11"
              value={receivedBy}
              onChange={(e) => setReceivedBy(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>Foto / tanda terima ({proofs.length}/5)</FieldLabel>
            <FileUpload
              label="Ambil Foto"
              accept="image/*,application/pdf"
              maxSizeBytes={5 * 1024 * 1024}
              disabled={proofs.length >= 5}
              requestUpload={requestProofUpload}
              onUploaded={(fileId) => setProofs((p) => [...p, fileId])}
              onError={(m) => toast.error(m)}
            />
            {errors.proofFileIds ? (
              <FieldError>{errors.proofFileIds}</FieldError>
            ) : null}
          </Field>
          <Field>
            <FieldLabel htmlFor="dlv-note">Catatan</FieldLabel>
            <Textarea
              id="dlv-note"
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
              className="min-h-11 w-full sm:w-auto"
            >
              Barang Diterima
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CancelDialog({
  d,
  onClose,
}: {
  d: DeliveryOrder
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      cancelDeliveryOrder(d.id, d.version, reason, key),
    onSuccess: async () => {
      toast.success("Delivery Order dibatalkan")
      await queryClient.invalidateQueries({ queryKey: ["logistics"] })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Batalkan Delivery Order?</DialogTitle>
          <DialogDescription>
            Outbound ikut dibatalkan; alokasi stok tetap milik Sales Order
            sehingga DO baru bisa dibuat.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="cancel-reason">Alasan</FieldLabel>
          <Textarea
            id="cancel-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
          {errors.reason ? <FieldError>{errors.reason}</FieldError> : null}
        </Field>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Kembali
          </Button>
          <LoadingButton
            variant="destructive"
            className="min-h-11"
            loading={save.isPending}
            onClick={() => save.mutate()}
          >
            Batalkan DO
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
