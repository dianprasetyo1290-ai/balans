import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
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
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApprovalSteps } from "@/features/approval/components/approval-steps"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { createDeliveryOrder } from "@/features/logistics/api/logistics-api"
import { openFile } from "@/features/party/api/party-api"
import { subjectApprovalsQuery } from "@/features/procurement/api/procurement-api"
import type { DetailTab } from "@/features/procurement/components/request-detail-page"
import { Info, ReasonDialog } from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  attachPaymentProof,
  cancelOrder,
  confirmOrder,
  MAX_PROOF_FILE_BYTES,
  ORDER_KINDS,
  ORDER_TYPE_LABELS,
  orderKindOf,
  orderQuery,
  PRICE_RULE_LABELS,
  requestProofUpload,
  type SalesOrder,
} from "../api/sales-documents-api"

const ORDER_FIELD_LABELS: Record<string, string> = {
  orderType: "Jenis pesanan",
  channelId: "Channel",
  subChannelId: "Sub-channel",
  customerId: "Pelanggan",
  partnerId: "Mitra",
  externalOrderId: "Nomor order marketplace",
  clientPoNumber: "Nomor PO klien",
  businessDate: "Tanggal",
  sourceLocationId: "Lokasi asal stok",
  paymentMode: "Cara bayar",
  paymentTermDays: "Termin (hari)",
  shippingAmount: "Ongkos kirim",
  discountAmount: "Diskon",
  totalAmount: "Total",
  status: "Status",
  cancelReason: "Alasan dibatalkan",
  note: "Catatan",
  lines: "Baris",
}

type Props = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/**
 * Detail Sales Order: harga & aturan per baris, alokasi batch FEFO, Proforma/Invoice terkait, pengiriman, panel
 * Persetujuan diskon, dan Riwayat (S-301, S-302, S-306 – S-308; Desain UI/UX 3.3).
 */
export function OrderDetailPage({ id, tab, onTabChange }: Props) {
  const order = useQuery(orderQuery(id))
  return (
    <LoadingArea loading={order.isPending}>
      {order.data ? (
        <OrderDetail order={order.data} tab={tab} onTabChange={onTabChange} />
      ) : null}
    </LoadingArea>
  )
}

function OrderDetail({
  order,
  tab,
  onTabChange,
}: {
  order: SalesOrder
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const [confirming, setConfirming] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [uploading, setUploading] = useState(false)
  const can = (a: SalesOrder["allowedActions"][number]) =>
    order.allowedActions.includes(a)
  const kind = ORDER_KINDS[orderKindOf(order.orderType)]
  const marketplace = order.orderType === "marketplace"
  const navigate = useNavigate()

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ["sales"] })
    await queryClient.invalidateQueries({ queryKey: ["approval"] })
  }

  const confirm = useIdempotentMutation({
    mutationFn: (_: void, key) => confirmOrder(order.id, order.version, key),
    onSuccess: async (o) => {
      setConfirming(false)
      toast.success(
        o.status === "pending_approval"
          ? "Diskon melebihi batas, Sales Order menunggu persetujuan Head Sales"
          : o.status === "awaiting_payment"
            ? "Sales Order dikonfirmasi, Proforma Invoice terbit"
            : "Sales Order dikonfirmasi"
      )
      await refresh()
    },
    onError: (err) => {
      setConfirming(false)
      showErrorDialog(parseProblem(err))
    },
  })

  const cancel = useIdempotentMutation({
    mutationFn: (reason: string, key) =>
      cancelOrder(order.id, order.version, reason, key),
    onSuccess: async () => {
      setCancelling(false)
      toast.success("Sales Order dibatalkan, alokasi stok dilepas")
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  // Buat DO (S-309): Logistik membuat Delivery Order dari pesanan Diproses dengan alokasi belum dikirim.
  const createDO = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createDeliveryOrder({ salesOrderId: order.id }, key),
    onSuccess: async (d) => {
      toast.success("Delivery Order dibuat")
      await queryClient.invalidateQueries({ queryKey: ["logistics"] })
      await refresh()
      void navigate({
        to: "/logistics/delivery-orders/$id",
        params: { id: d.id },
      })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const lineName = (lineId: string) => {
    const l = order.lines.find((x) => x.id === lineId)
    return l?.isBundle ? `${l.product.sku} (bundel)` : ""
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {order.number ?? "Draft Sales Order"}
            </h1>
            <StatusBadge status={order.status} />
            <Badge variant="secondary">
              {ORDER_TYPE_LABELS[order.orderType]}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            {order.partyName || "—"} · {order.subChannelName} ·{" "}
            <Link
              to={kind.path}
              search={{ page: 1 }}
              className="underline-offset-4 hover:underline"
            >
              {kind.title}
            </Link>
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link to="/sales/orders/$id/edit" params={{ id: order.id }} />
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
          {can("upload_payment_proof") ? (
            <Button variant="outline" onClick={() => setUploading(true)}>
              Unggah Bukti Bayar
            </Button>
          ) : null}
          {order.proformas
            .filter((p) => p.status === "awaiting_payment")
            .map((p) => (
              <Button
                key={p.id}
                variant="outline"
                render={
                  <Link
                    to="/sales/proforma-invoices/$id"
                    params={{ id: p.id }}
                  />
                }
              >
                Buka Proforma {p.number}
              </Button>
            ))}
          {can("create_delivery_order") ? (
            <LoadingButton
              className="min-h-11"
              loading={createDO.isPending}
              onClick={() => createDO.mutate()}
            >
              Buat DO
            </LoadingButton>
          ) : null}
          {can("confirm") ? (
            <Button onClick={() => setConfirming(true)}>
              Konfirmasi Sales Order
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian Sales Order"
        value={tab}
        onValueChange={onTabChange}
        tabs={[
          { id: "summary", label: "Ringkasan" },
          { id: "history", label: "Riwayat" },
        ]}
      />
      {tab === "summary" ? (
        <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
          <div className="flex min-w-0 flex-col gap-4">
            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-3">
              <Info
                label={marketplace ? "Pembeli" : "Pelanggan / Mitra"}
                value={
                  [order.partyName, order.partyCode, order.partyPhone]
                    .filter(Boolean)
                    .join(" · ") || "—"
                }
              />
              <Info
                label="Channel"
                value={`${order.channelName} › ${order.subChannelName}`}
              />
              <Info label="Tanggal" value={formatDate(order.businessDate)} />
              {marketplace ? (
                <Info
                  label="Nomor Order Marketplace"
                  value={order.externalOrderId}
                />
              ) : null}
              {order.clientPoNumber ? (
                <Info label="PO Klien" value={order.clientPoNumber} />
              ) : null}
              <Info
                label="Lokasi Asal Stok"
                value={`${order.sourceLocation.code} · ${order.sourceLocation.name}`}
              />
              {!marketplace ? (
                <Info
                  label="Cara Bayar"
                  value={
                    order.paymentMode === "credit"
                      ? `Tempo ${order.paymentTermDays} hari`
                      : "Bayar di muka (Proforma Invoice)"
                  }
                />
              ) : null}
              {!marketplace ? (
                <Info
                  label="Kirim ke"
                  value={
                    [order.shipToName, order.shipToPhone, order.shipToAddress]
                      .filter(Boolean)
                      .join(" · ") || "—"
                  }
                />
              ) : null}
              <Info label="Dibuat oleh" value={order.createdByName || "—"} />
              {order.confirmedAt ? (
                <Info
                  label="Dikonfirmasi"
                  value={formatDateTime(order.confirmedAt)}
                />
              ) : null}
              {order.paymentProofFileId ? (
                <Info
                  label="Bukti Bayar"
                  value={
                    <button
                      type="button"
                      className="text-primary underline-offset-4 hover:underline"
                      onClick={() =>
                        void openFile(order.paymentProofFileId ?? "")
                      }
                    >
                      {order.paymentProofFileName ?? "Lihat file"}
                    </button>
                  }
                />
              ) : null}
              <Info label="Catatan" value={order.note || "—"} />
              {order.cancelReason ? (
                <Info label="Alasan Dibatalkan" value={order.cancelReason} />
              ) : null}
            </dl>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[56rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Produk</TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Harga</TableHead>
                    <TableHead>Aturan Harga</TableHead>
                    <TableHead className="text-right">Diskon</TableHead>
                    <TableHead className="text-right">Jumlah</TableHead>
                    <TableHead className="text-right">Dikirim</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {order.lines.map((l) => (
                    <TableRow key={l.id}>
                      <TableCell>
                        {l.product.sku} · {l.product.name}
                        {l.isBundle ? (
                          <Badge variant="outline" className="ml-2">
                            Bundel
                          </Badge>
                        ) : null}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.qty)} {l.product.uom}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.unitPrice)}
                      </TableCell>
                      <TableCell>
                        <span title={l.priceDescription}>
                          {PRICE_RULE_LABELS[l.priceRule] ?? l.priceRule}
                        </span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {Number(l.discountAmount) > 0
                          ? `−${formatRupiah(l.discountAmount)}`
                          : "—"}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.lineTotalAmount)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.shippedQty)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TotalRow label="Bruto" value={order.grossAmount} />
                  {Number(order.lineDiscountAmount) +
                    Number(order.documentDiscountAmount) >
                  0 ? (
                    <TotalRow
                      label={
                        marketplace ? "Diskon & voucher penjual" : "Diskon"
                      }
                      value={`−${formatRupiah(
                        (
                          Number(order.lineDiscountAmount) +
                          Number(order.documentDiscountAmount)
                        ).toFixed(2)
                      )}`}
                      raw
                    />
                  ) : null}
                  {Number(order.shippingAmount) > 0 ? (
                    <TotalRow
                      label="Ongkos kirim"
                      value={order.shippingAmount}
                    />
                  ) : null}
                  <TotalRow
                    label="Total (tanpa PPN — NON-PKP)"
                    value={order.totalAmount}
                  />
                </TableFooter>
              </Table>
            </div>
            {order.allocations.length > 0 ? (
              <section className="flex flex-col gap-2">
                <h2 className="text-sm font-semibold">
                  Alokasi Stok (FEFO, bundel dipecah ke komponen)
                </h2>
                <div className="overflow-x-auto rounded-lg border">
                  <Table className="min-w-[48rem]">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Komponen</TableHead>
                        <TableHead>Lokasi</TableHead>
                        <TableHead>Batch · Kedaluwarsa</TableHead>
                        <TableHead className="text-right">Qty</TableHead>
                        <TableHead className="text-right">Dikirim</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {order.allocations.map((a) => (
                        <TableRow key={a.id}>
                          <TableCell>
                            {a.product.sku} · {a.product.name}
                            {lineName(a.lineId) ? (
                              <span className="block text-xs text-muted-foreground">
                                dari {lineName(a.lineId)}
                              </span>
                            ) : null}
                          </TableCell>
                          <TableCell>{a.location.code}</TableCell>
                          <TableCell>
                            {a.released ? (
                              <span className="text-muted-foreground">
                                Dilepas
                              </span>
                            ) : (
                              <ul className="flex flex-col">
                                {a.batches.map((b, i) => (
                                  <li key={b.batchId ?? i}>
                                    {b.batchNumber ?? "Tanpa batch"}
                                    {b.expiryDate
                                      ? ` · ED ${formatDate(b.expiryDate)}`
                                      : ""}{" "}
                                    <span className="text-muted-foreground">
                                      ({formatQuantity(b.qty)})
                                    </span>
                                  </li>
                                ))}
                              </ul>
                            )}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatQuantity(a.qty)} {a.product.uom}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatQuantity(a.shippedQty)}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </section>
            ) : null}
            {order.deliveryOrders && order.deliveryOrders.length > 0 ? (
              <section className="flex flex-col gap-2">
                <h2 className="text-sm font-semibold">Delivery Order</h2>
                <ul className="flex flex-col gap-1 text-sm">
                  {order.deliveryOrders.map((d) => (
                    <li key={d.id}>
                      <Link
                        to="/logistics/delivery-orders/$id"
                        params={{ id: d.id }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {d.number ?? "Draft DO"}
                      </Link>{" "}
                      · {statusLabel(d.status).label}
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
            {order.shipments.length > 0 ? (
              <section className="flex flex-col gap-2">
                <h2 className="text-sm font-semibold">Pengiriman</h2>
                <ul className="flex flex-col gap-1 text-sm">
                  {order.shipments.map((s) => (
                    <li key={s.id}>
                      {s.sourceLabel} · {formatDate(s.businessDate)} · HPP{" "}
                      {formatRupiah(s.costAmount)}
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
          </div>
          <div className="flex flex-col gap-4">
            <section className="flex flex-col gap-2 rounded-lg border p-4 text-sm">
              <h2 className="font-semibold">Dokumen Tagihan</h2>
              {order.proformas.length + order.invoices.length === 0 ? (
                <p className="text-muted-foreground">
                  {marketplace
                    ? "Order marketplace ditagih lewat settlement marketplace."
                    : "Proforma Invoice atau invoice terbit saat Sales Order dikonfirmasi."}
                </p>
              ) : null}
              {order.proformas.map((p) => (
                <div
                  key={p.id}
                  className="flex items-center justify-between gap-2"
                >
                  <Link
                    to="/sales/proforma-invoices/$id"
                    params={{ id: p.id }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    {p.number}
                  </Link>
                  <StatusBadge status={p.status} />
                </div>
              ))}
              {order.invoices.map((inv) => (
                <div
                  key={inv.id}
                  className="flex items-center justify-between gap-2"
                >
                  <Link
                    to="/sales/invoices/$id"
                    params={{ id: inv.id }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    {inv.number}
                  </Link>
                  <StatusBadge status={inv.status} />
                </div>
              ))}
            </section>
            {order.discountApprovalRequired ? (
              <DiscountApprovalPanel orderId={order.id} />
            ) : null}
          </div>
        </div>
      ) : (
        <EntityHistory
          entityType={marketplace ? "marketplace_order" : "sales_order"}
          entityId={order.id}
          fieldLabels={ORDER_FIELD_LABELS}
        />
      )}
      {confirming ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirming(false)}
          title="Konfirmasi Sales Order?"
          description={
            order.paymentMode === "credit"
              ? `Harga dihitung ulang, stok dialokasikan FEFO, dan invoice tempo ${order.paymentTermDays} hari terbit sebesar total pesanan.`
              : "Harga dihitung ulang, stok dialokasikan FEFO, dan Proforma Invoice berlaku 14 hari terbit. Diskon di atas batas role diajukan ke Head Sales."
          }
          confirmLabel="Konfirmasi Sales Order"
          loading={confirm.isPending}
          onConfirm={() => confirm.mutate()}
        />
      ) : null}
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan Sales Order?"
        description="Alokasi stok dilepas, Proforma dibatalkan, dan invoice tempo tanpa pembayaran dibatalkan dengan jurnal balik."
        confirmLabel="Batalkan Sales Order"
        required={order.status !== "draft"}
        destructive
        loading={cancel.isPending}
        onConfirm={(r) => cancel.mutate(r)}
      />
      <ProofDialog
        open={uploading}
        onOpenChange={setUploading}
        order={order}
        onDone={refresh}
      />
    </div>
  )
}

function TotalRow({
  label,
  value,
  raw = false,
}: {
  label: string
  value: string
  raw?: boolean
}) {
  return (
    <TableRow>
      <TableCell colSpan={5} className="font-medium">
        {label}
      </TableCell>
      <TableCell className="text-right tabular-nums">
        {raw ? value : formatRupiah(value)}
      </TableCell>
      <TableCell />
    </TableRow>
  )
}

/** Panel persetujuan diskon di atas batas role (`sales_order_discount`). */
function DiscountApprovalPanel({ orderId }: { orderId: string }) {
  const approvals = useQuery(
    subjectApprovalsQuery("sales_order_discount", orderId)
  )
  const requests = approvals.data?.items ?? []
  return (
    <section className="flex flex-col gap-3 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">Persetujuan Diskon</h2>
      <LoadingArea loading={approvals.isPending} label="Memuat persetujuan">
        {requests.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum ada pengajuan persetujuan.
          </p>
        ) : (
          <ol className="flex flex-col gap-4">
            {requests.map((r, i) => (
              <li key={r.id} className="flex flex-col gap-2">
                {requests.length > 1 ? (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span>Pengajuan {requests.length - i}</span>
                    <StatusBadge status={r.status} />
                  </div>
                ) : null}
                <ApprovalSteps steps={r.steps} />
              </li>
            ))}
          </ol>
        )}
      </LoadingArea>
    </section>
  )
}

/** Unggah bukti bayar pelanggan (foto/scan transfer). */
function ProofDialog({
  open,
  onOpenChange,
  order,
  onDone,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: SalesOrder
  onDone: () => Promise<void>
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ProofForm
          order={order}
          onClose={() => onOpenChange(false)}
          onDone={onDone}
        />
      ) : null}
    </Dialog>
  )
}

function ProofForm({
  order,
  onClose,
  onDone,
}: {
  order: SalesOrder
  onClose: () => void
  onDone: () => Promise<void>
}) {
  const [fileId, setFileId] = useState<string | null>(null)
  const [error, setError] = useState<string>()
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      attachPaymentProof(order.id, order.version, fileId ?? "", key),
    onSuccess: async () => {
      toast.success(
        "Bukti bayar tersimpan; F&A mengonfirmasi di Proforma Invoice"
      )
      onClose()
      await onDone()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Unggah Bukti Bayar</DialogTitle>
        <DialogDescription>
          Foto atau PDF bukti transfer dari pelanggan untuk {order.number}.
        </DialogDescription>
      </DialogHeader>
      <FileUpload
        label="Pilih file bukti bayar"
        accept="image/jpeg,image/png,image/webp,application/pdf"
        maxSizeBytes={MAX_PROOF_FILE_BYTES}
        requestUpload={(f) => requestProofUpload(f, "sales.payment_proof")}
        onUploaded={(id) => {
          setFileId(id)
          setError(undefined)
        }}
        onRemoved={() => setFileId(null)}
        onError={setError}
      />
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton
          loading={save.isPending}
          loadingText="Menyimpan…"
          disabled={!fileId}
          onClick={() => save.mutate()}
        >
          Simpan Bukti Bayar
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
