import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { PrinterIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  orderAction,
  orderQuery,
  type OrderAction,
  type PurchaseOrder,
} from "../api/procurement-api"
import { AdvanceFormDialog } from "./advance-form-dialog"
import type { DetailTab } from "./request-detail-page"
import { ApprovalPanel, Info, ReasonDialog } from "./shared"

const ORDER_FIELD_LABELS: Record<string, string> = {
  orderType: "Jenis PO",
  supplierId: "Pemasok",
  supplierName: "Nama pemasok",
  divisionId: "Divisi",
  businessDate: "Tanggal",
  expectedDate: "Tanggal kirim diharapkan",
  paymentTermDays: "Termin (hari)",
  note: "Catatan",
  materialSourceLocationId: "Lokasi asal bahan",
  totalAmount: "Total",
  status: "Status",
  closeReason: "Alasan ditutup",
  cancelReason: "Alasan dibatalkan",
  lines: "Baris",
  materials: "Bahan maklon",
  approvalStatus: "Status persetujuan",
  approvalLevel: "Level persetujuan",
}

type Props = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail Purchase Order: baris, bahan maklon, uang muka, panel Persetujuan, dan Riwayat (S-202 – S-205). */
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
  order: PurchaseOrder
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<"submit" | "revise" | null>(null)
  const [reason, setReason] = useState<"cancel" | "close" | null>(null)
  const [advancing, setAdvancing] = useState(false)
  const can = (a: PurchaseOrder["allowedActions"][number]) =>
    order.allowedActions.includes(a)
  const { can: canDo } = usePermissions()
  const toll =
    order.orderType === "toll_manufacturing" || order.orderType === "reprocess"

  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ["procurement"] })
    await queryClient.invalidateQueries({ queryKey: ["approval"] })
  }

  const action = useIdempotentMutation({
    mutationFn: (v: { action: OrderAction; reason?: string }, key) =>
      orderAction(v.action, order.id, order.version, key, v.reason),
    onSuccess: async (_, v) => {
      setConfirm(null)
      setReason(null)
      const messages: Record<OrderAction, string> = {
        submit: "Purchase Order diajukan",
        revise: "Purchase Order kembali Draft",
        cancel: "Purchase Order dibatalkan",
        close: "Purchase Order ditutup",
      }
      toast.success(messages[v.action])
      await refresh()
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
              {order.number ?? "Draft Purchase Order"}
            </h1>
            <StatusBadge status={order.status} />
            <Badge variant="secondary">{order.orderTypeLabel}</Badge>
          </div>
          <p className="text-sm text-muted-foreground">
            {order.supplierName} · {order.divisionName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/purchase-orders/$id/edit"
                  params={{ id: order.id }}
                />
              }
            >
              Ubah
            </Button>
          ) : null}
          {can("cancel") ? (
            <Button variant="outline" onClick={() => setReason("cancel")}>
              Batalkan
            </Button>
          ) : null}
          {can("revise") ? (
            <Button variant="outline" onClick={() => setConfirm("revise")}>
              Revisi
            </Button>
          ) : null}
          {can("close") ? (
            <Button variant="outline" onClick={() => setReason("close")}>
              Tutup Purchase Order
            </Button>
          ) : null}
          {can("print") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/purchase-orders/$id/print"
                  params={{ id: order.id }}
                />
              }
            >
              <PrinterIcon /> Cetak PO
            </Button>
          ) : null}
          {(order.status === "approved" ||
            order.status === "partially_received") &&
          canDo("procurement.goods_receipt.write") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/goods-receipts/new"
                  search={{ purchaseOrderId: order.id }}
                />
              }
            >
              Terima Barang
            </Button>
          ) : null}
          {can("create_advance") ? (
            <Button variant="outline" onClick={() => setAdvancing(true)}>
              Catat Uang Muka
            </Button>
          ) : null}
          {can("submit") ? (
            <Button onClick={() => setConfirm("submit")}>
              Ajukan Purchase Order
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian Purchase Order"
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
                label={toll ? "Maklon" : "Pemasok"}
                value={`${order.supplierName} (${order.supplierCode})`}
              />
              <Info label="Divisi" value={order.divisionName || "—"} />
              <Info label="Tanggal PO" value={formatDate(order.businessDate)} />
              <Info
                label="Tanggal Kirim Diharapkan"
                value={formatDate(order.expectedDate)}
              />
              <Info
                label="Termin Pembayaran"
                value={`${order.paymentTermDays} hari`}
              />
              <Info label="Dibuat oleh" value={order.createdByName || "—"} />
              {order.tollLocation ? (
                <Info
                  label="Lokasi Maklon"
                  value={`${order.tollLocation.code} · ${order.tollLocation.name}`}
                />
              ) : null}
              {toll ? (
                <Info
                  label="Lokasi Asal Bahan"
                  value={
                    order.materialSourceLocation
                      ? `${order.materialSourceLocation.code} · ${order.materialSourceLocation.name}`
                      : "Sudah di lokasi maklon"
                  }
                />
              ) : null}
              {order.approvedAt ? (
                <Info
                  label="Disetujui"
                  value={formatDateTime(order.approvedAt)}
                />
              ) : null}
              <Info label="Catatan" value={order.note || "—"} />
              {order.closeReason ? (
                <Info label="Alasan Ditutup" value={order.closeReason} />
              ) : null}
              {order.cancelReason ? (
                <Info label="Alasan Dibatalkan" value={order.cancelReason} />
              ) : null}
            </dl>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[56rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>
                      {toll ? "Produk Jadi (hasil)" : "Produk"}
                    </TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Diterima</TableHead>
                    <TableHead className="text-right">Sisa</TableHead>
                    <TableHead className="text-right">
                      {toll ? "Harga Jasa" : "Harga"}
                    </TableHead>
                    <TableHead className="text-right">Jumlah</TableHead>
                    <TableHead>Purchase Request</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {order.lines.map((l) => (
                    <TableRow key={l.id}>
                      <TableCell>
                        {l.product
                          ? `${l.product.sku} · ${l.product.name}`
                          : l.description}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.orderedQty)} {l.product?.uom}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.receivedQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.outstandingQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.unitPrice)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.amount)}
                      </TableCell>
                      <TableCell>
                        {l.purchaseRequestId ? (
                          <Link
                            to="/procurement/purchase-requests/$id"
                            params={{ id: l.purchaseRequestId }}
                            search={{}}
                            className="whitespace-nowrap text-primary underline-offset-4 hover:underline"
                          >
                            {l.purchaseRequestNumber}
                          </Link>
                        ) : (
                          "—"
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell colSpan={5} className="font-medium">
                      Total (tanpa PPN — NON-PKP)
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(order.totalAmount)}
                    </TableCell>
                    <TableCell />
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
            {order.materials.length > 0 ? (
              <section className="flex flex-col gap-2">
                <h2 className="text-sm font-semibold">
                  {order.orderType === "reprocess"
                    ? "Bahan GM yang Dikirim ke Maklon"
                    : "Packaging yang Akan Dipakai Maklon"}
                </h2>
                <div className="overflow-x-auto rounded-lg border">
                  <Table className="min-w-[36rem]">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Bahan</TableHead>
                        <TableHead className="text-right">
                          Qty Rencana
                        </TableHead>
                        <TableHead>Catatan</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {order.materials.map((m) => (
                        <TableRow key={m.id}>
                          <TableCell>
                            {m.product.sku} · {m.product.name}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatQuantity(m.plannedQty)} {m.product.uom}
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {m.note || "—"}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </section>
            ) : null}
            <section className="flex flex-col gap-2">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h2 className="text-sm font-semibold">Uang Muka Pembelian</h2>
                <span className="text-sm text-muted-foreground">
                  Dibayar {formatRupiah(order.advancePaidAmount)} · sisa
                  dipotong tagihan{" "}
                  {formatRupiah(order.advanceOutstandingAmount)}
                </span>
              </div>
              {order.advances.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  Belum ada uang muka.
                </p>
              ) : (
                <div className="overflow-x-auto rounded-lg border">
                  <Table className="min-w-[40rem]">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Nomor</TableHead>
                        <TableHead>Tanggal Bayar</TableHead>
                        <TableHead className="text-right">Nominal</TableHead>
                        <TableHead className="text-right">Sisa</TableHead>
                        <TableHead>Status</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {order.advances.map((a) => (
                        <TableRow key={a.id}>
                          <TableCell>
                            <Link
                              to="/procurement/purchase-advances/$id"
                              params={{ id: a.id }}
                              search={{}}
                              className="text-primary underline-offset-4 hover:underline"
                            >
                              {a.number ?? "Draft"}
                            </Link>
                          </TableCell>
                          <TableCell>{formatDate(a.paymentDate)}</TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatRupiah(a.amount)}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatRupiah(a.outstandingAmount)}
                          </TableCell>
                          <TableCell>
                            <StatusBadge status={a.status} />
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </section>
          </div>
          {order.number ? (
            <ApprovalPanel subjectType="purchase_order" subjectId={order.id} />
          ) : null}
        </div>
      ) : (
        <EntityHistory
          entityType="purchase_order"
          entityId={order.id}
          fieldLabels={ORDER_FIELD_LABELS}
        />
      )}
      {confirm ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirm(null)}
          title={
            confirm === "submit"
              ? "Ajukan Purchase Order?"
              : "Revisi Purchase Order?"
          }
          description={
            confirm === "submit"
              ? `Nomor PO diberikan dan PO diajukan untuk persetujuan berjenjang sesuai total ${formatRupiah(order.totalAmount)}.`
              : "PO kembali Draft dengan nomor yang sama agar bisa diperbaiki dan diajukan ulang."
          }
          confirmLabel={
            confirm === "submit"
              ? "Ajukan Purchase Order"
              : "Revisi Purchase Order"
          }
          loading={action.isPending}
          onConfirm={() => action.mutate({ action: confirm })}
        />
      ) : null}
      <ReasonDialog
        open={reason !== null}
        onOpenChange={(o) => !o && setReason(null)}
        title={
          reason === "close"
            ? "Tutup Purchase Order?"
            : "Batalkan Purchase Order?"
        }
        description={
          reason === "close"
            ? "Sisa qty yang belum diterima tidak ditunggu lagi. Qty Purchase Request terkait dihitung ulang."
            : "PO dibatalkan dan qty Purchase Request asal dikembalikan."
        }
        confirmLabel={
          reason === "close"
            ? "Tutup Purchase Order"
            : "Batalkan Purchase Order"
        }
        required={reason === "close" || order.status !== "draft"}
        destructive={reason === "cancel"}
        loading={action.isPending}
        onConfirm={(r) =>
          reason && action.mutate({ action: reason, reason: r })
        }
      />
      <AdvanceFormDialog
        open={advancing}
        onOpenChange={setAdvancing}
        order={{
          id: order.id,
          number: order.number ?? "",
          supplierName: order.supplierName,
        }}
      />
    </div>
  )
}
