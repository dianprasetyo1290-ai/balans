import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { StatusBadge } from "@/components/common/status-badge"
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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  requestAction,
  requestQuery,
  type PurchaseRequest,
  type RequestAction,
} from "../api/procurement-api"
import { ApprovalPanel, Info, ReasonDialog } from "./shared"

export type DetailTab = "summary" | "history"

const REQUEST_FIELD_LABELS: Record<string, string> = {
  divisionId: "Divisi",
  businessDate: "Tanggal",
  neededDate: "Tanggal dibutuhkan",
  note: "Catatan",
  estimatedTotal: "Estimasi total",
  status: "Status",
  lines: "Baris",
  approvalStatus: "Status persetujuan",
  approvalLevel: "Level persetujuan",
}

type Props = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail Purchase Request: baris, PO yang dibuat, panel Persetujuan, dan Riwayat (S-201). */
export function RequestDetailPage({ id, tab, onTabChange }: Props) {
  const request = useQuery(requestQuery(id))
  return (
    <LoadingArea loading={request.isPending}>
      {request.data ? (
        <RequestDetail
          request={request.data}
          tab={tab}
          onTabChange={onTabChange}
        />
      ) : null}
    </LoadingArea>
  )
}

const CONFIRM: Record<
  Exclude<RequestAction, "close">,
  { title: string; description: string; label: string; destructive?: boolean }
> = {
  submit: {
    title: "Ajukan Purchase Request?",
    description:
      "Nomor PR diberikan dan Head divisi peminta diberi notifikasi untuk menyetujui. PR tidak bisa diubah selama menunggu persetujuan.",
    label: "Ajukan Purchase Request",
  },
  revise: {
    title: "Revisi Purchase Request?",
    description:
      "PR kembali Draft dengan nomor yang sama agar bisa diperbaiki dan diajukan ulang.",
    label: "Revisi Purchase Request",
  },
  cancel: {
    title: "Batalkan Purchase Request?",
    description: "Draft PR dibatalkan dan tidak bisa diajukan lagi.",
    label: "Batalkan Purchase Request",
    destructive: true,
  },
}

function RequestDetail({
  request,
  tab,
  onTabChange,
}: {
  request: PurchaseRequest
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<Exclude<
    RequestAction,
    "close"
  > | null>(null)
  const [closing, setClosing] = useState(false)
  const can = (a: PurchaseRequest["allowedActions"][number]) =>
    request.allowedActions.includes(a)

  const action = useIdempotentMutation({
    mutationFn: (v: { action: RequestAction; reason?: string }, key) =>
      requestAction(v.action, request.id, request.version, key, v.reason),
    onSuccess: async (_, v) => {
      setConfirm(null)
      setClosing(false)
      const messages: Record<RequestAction, string> = {
        submit: "Purchase Request diajukan",
        revise: "Purchase Request kembali Draft",
        cancel: "Purchase Request dibatalkan",
        close: "Purchase Request ditutup",
      }
      toast.success(messages[v.action])
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      await queryClient.invalidateQueries({ queryKey: ["approval"] })
      await queryClient.invalidateQueries({ queryKey: ["audit"] })
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const lineOrders = (lineId: string) =>
    request.orders.filter((o) => o.purchaseRequestLineId === lineId)
  const orders = [
    ...new Map(request.orders.map((o) => [o.purchaseOrderId, o])).values(),
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {request.number ?? "Draft Purchase Request"}
            </h1>
            <StatusBadge status={request.status} />
          </div>
          <p className="text-sm text-muted-foreground">
            Purchase Request · {request.divisionName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/procurement/purchase-requests/$id/edit"
                  params={{ id: request.id }}
                />
              }
            >
              Ubah
            </Button>
          ) : null}
          {can("cancel") ? (
            <Button variant="outline" onClick={() => setConfirm("cancel")}>
              Batalkan
            </Button>
          ) : null}
          {can("revise") ? (
            <Button variant="outline" onClick={() => setConfirm("revise")}>
              Revisi
            </Button>
          ) : null}
          {can("close") ? (
            <Button variant="outline" onClick={() => setClosing(true)}>
              Tutup Purchase Request
            </Button>
          ) : null}
          {can("create_order") ? (
            <Button
              onClick={() =>
                void navigate({
                  to: "/procurement/purchase-orders/new",
                  search: { orderType: "packaging", fromRequest: request.id },
                })
              }
            >
              Buat Purchase Order
            </Button>
          ) : null}
          {can("submit") ? (
            <Button onClick={() => setConfirm("submit")}>
              Ajukan Purchase Request
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian Purchase Request"
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
                label="Divisi Peminta"
                value={request.divisionName || "—"}
              />
              <Info label="Peminta" value={request.requestedByName || "—"} />
              <Info label="Tanggal" value={formatDate(request.businessDate)} />
              <Info
                label="Tanggal Dibutuhkan"
                value={
                  request.neededDate ? formatDate(request.neededDate) : "—"
                }
              />
              <Info
                label="Diajukan"
                value={
                  request.submittedAt
                    ? formatDateTime(request.submittedAt)
                    : "—"
                }
              />
              <Info label="Catatan" value={request.note || "—"} />
              {request.closeReason ? (
                <Info label="Alasan Ditutup" value={request.closeReason} />
              ) : null}
            </dl>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[52rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Barang</TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Sudah PO</TableHead>
                    <TableHead className="text-right">Sisa</TableHead>
                    <TableHead className="text-right">Estimasi Harga</TableHead>
                    <TableHead className="text-right">Jumlah</TableHead>
                    <TableHead>Purchase Order</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {request.lines.map((l) => (
                    <TableRow key={l.id}>
                      <TableCell>
                        <div className="flex flex-col">
                          <span>
                            {l.product
                              ? `${l.product.sku} · ${l.product.name}`
                              : l.description}
                          </span>
                          {l.product && l.description ? (
                            <span className="text-xs text-muted-foreground">
                              {l.description}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.qty)} {l.product?.uom}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.fulfilledQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatQuantity(l.remainingQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.estimatedPrice)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.estimatedAmount)}
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col gap-0.5">
                          {lineOrders(l.id).map((o) => (
                            <Link
                              key={o.purchaseOrderId}
                              to="/procurement/purchase-orders/$id"
                              params={{ id: o.purchaseOrderId }}
                              search={{}}
                              className="text-sm whitespace-nowrap text-primary underline-offset-4 hover:underline"
                            >
                              {o.purchaseOrderNumber ?? "Draft PO"} ·{" "}
                              {formatQuantity(o.orderedQty)}
                            </Link>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell colSpan={5} className="font-medium">
                      Estimasi Total
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(request.estimatedTotal)}
                    </TableCell>
                    <TableCell />
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
            {orders.length > 0 ? (
              <section className="flex flex-col gap-2">
                <h2 className="text-sm font-semibold">
                  Purchase Order terkait
                </h2>
                <ul className="flex flex-wrap gap-3 text-sm">
                  {orders.map((o) => (
                    <li
                      key={o.purchaseOrderId}
                      className="flex items-center gap-2"
                    >
                      <Link
                        to="/procurement/purchase-orders/$id"
                        params={{ id: o.purchaseOrderId }}
                        search={{}}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {o.purchaseOrderNumber ?? "Draft PO"}
                      </Link>
                      <StatusBadge status={o.status} />
                    </li>
                  ))}
                </ul>
              </section>
            ) : null}
          </div>
          {request.number ? (
            <ApprovalPanel
              subjectType="purchase_request"
              subjectId={request.id}
            />
          ) : null}
        </div>
      ) : (
        <EntityHistory
          entityType="purchase_request"
          entityId={request.id}
          fieldLabels={REQUEST_FIELD_LABELS}
        />
      )}
      {confirm ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirm(null)}
          title={CONFIRM[confirm].title}
          description={CONFIRM[confirm].description}
          confirmLabel={CONFIRM[confirm].label}
          destructive={CONFIRM[confirm].destructive}
          loading={action.isPending}
          onConfirm={() => action.mutate({ action: confirm })}
        />
      ) : null}
      <ReasonDialog
        open={closing}
        onOpenChange={setClosing}
        title="Tutup Purchase Request?"
        description="Sisa qty yang belum dijadikan PO tidak akan diproses lagi."
        confirmLabel="Tutup Purchase Request"
        required
        loading={action.isPending}
        onConfirm={(reason) => action.mutate({ action: "close", reason })}
      />
    </div>
  )
}
