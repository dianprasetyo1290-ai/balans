import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { UploadIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
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
import { Field, FieldLabel } from "@/components/ui/field"
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
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  cancelSettlement,
  marketplaceReceivablesQuery,
  postSettlement,
  settlementFormatsQuery,
  settlementQuery,
  settlementsQuery,
  type MarketplaceSettlementSummary,
  type SettlementStatus,
} from "../api/reports-api"
import type { SearchUpdater, SettlementSearch } from "../lib/reports-lib"

const STATUS_LABELS: Record<SettlementStatus, string> = {
  draft: "Draft",
  posted: "Diposting",
  cancelled: "Dibatalkan",
}

/** Keuangan & Akuntansi › Piutang Usaha › Settlement Marketplace (S-403, ACC-06). */
export function MarketplaceSettlementPage({
  search,
  onSearchChange,
}: {
  search: SettlementSearch
  onSearchChange: SearchUpdater<SettlementSearch>
}) {
  const tab = search.tab ?? "settlements"
  const { can } = usePermissions()
  const formats = useQuery(settlementFormatsQuery)
  const [market, setMarket] = useState("")
  const kind =
    (formats.data?.items ?? []).find((f) => f.marketplace === market)?.kind ??
    formats.data?.items[0]?.kind
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Settlement Marketplace
          </h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Impor laporan settlement (dana cair) dari seller center; setiap
            baris dicocokkan dengan order marketplace yang sudah diserahkan ke
            kurir. Posting membuat jurnal A-10s: Bank + Beban Fee Marketplace +
            Beban Promosi Marketplace terhadap Piutang Marketplace.
          </p>
        </div>
        {can("accounting.settlement.import") ? (
          <div className="flex flex-wrap gap-2">
            <Select
              value={market || formats.data?.items[0]?.marketplace || ""}
              onValueChange={(v) => setMarket(v ?? "")}
            >
              <SelectTrigger className="w-52" aria-label="Format marketplace">
                <SelectValue>
                  {(v: string) =>
                    formats.data?.items.find((f) => f.marketplace === v)
                      ?.label ?? "Pilih marketplace"
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(formats.data?.items ?? []).map((f) => (
                  <SelectItem key={f.marketplace} value={f.marketplace}>
                    {f.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {kind ? (
              <Button
                className="min-h-11 sm:min-h-9"
                nativeButton={false}
                render={
                  <Link
                    to="/integrations/data-exchange"
                    search={{ tab: "import", kind }}
                  />
                }
              >
                <UploadIcon /> Impor Laporan Settlement
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
      <PageTabs
        label="Settlement marketplace"
        tabs={[
          { id: "settlements", label: "Settlement" },
          { id: "receivables", label: "Sisa Piutang per Order" },
        ]}
        value={tab}
        onValueChange={(t) =>
          onSearchChange((s) => ({ ...s, tab: t, page: 1 }))
        }
      />
      {tab === "settlements" ? (
        <SettlementList search={search} onSearchChange={onSearchChange} />
      ) : (
        <ReceivableList search={search} onSearchChange={onSearchChange} />
      )}
      {search.id ? (
        <SettlementDialog
          id={search.id}
          onClose={() => onSearchChange((s) => ({ ...s, id: undefined }))}
        />
      ) : null}
    </div>
  )
}

function SettlementList({
  search,
  onSearchChange,
}: {
  search: SettlementSearch
  onSearchChange: SearchUpdater<SettlementSearch>
}) {
  const list = useQuery(settlementsQuery(search.page, search.status))
  const columns: DataTableColumn<MarketplaceSettlementSummary>[] = [
    {
      id: "ref",
      header: "Referensi Pencairan",
      cell: (s) => (
        <button
          type="button"
          className="min-h-11 text-left font-medium underline-offset-2 hover:underline sm:min-h-0"
          onClick={() => onSearchChange((p) => ({ ...p, id: s.id }))}
        >
          {s.payoutReference}
        </button>
      ),
    },
    { id: "market", header: "Marketplace", cell: (s) => s.marketplaceName },
    {
      id: "date",
      header: "Tanggal Dana Cair",
      cell: (s) => formatDate(s.settlementDate),
    },
    { id: "orders", header: "Order", cell: (s) => s.lineCount, align: "right" },
    {
      id: "net",
      header: "Dana Cair",
      cell: (s) => formatRupiah(s.net),
      align: "right",
    },
    {
      id: "fee",
      header: "Fee",
      cell: (s) => formatRupiah(s.fee),
      align: "right",
    },
    {
      id: "promo",
      header: "Promo",
      cell: (s) => formatRupiah(s.promo),
      align: "right",
    },
    {
      id: "settled",
      header: "Pengurang Piutang",
      cell: (s) => formatRupiah(s.settled),
      align: "right",
    },
    {
      id: "status",
      header: "Status",
      cell: (s) => <StatusBadge status={s.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-3">
      <Select
        value={search.status ?? "all"}
        onValueChange={(v) =>
          onSearchChange((s) => ({
            ...s,
            page: 1,
            status: !v || v === "all" ? undefined : (v as SettlementStatus),
          }))
        }
      >
        <SelectTrigger className="w-44" aria-label="Filter status">
          <SelectValue>
            {(v: string) =>
              v === "all"
                ? "Semua status"
                : STATUS_LABELS[v as SettlementStatus]
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">Semua status</SelectItem>
          {Object.entries(STATUS_LABELS).map(([k, label]) => (
            <SelectItem key={k} value={k}>
              {label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <DataTable
        columns={columns}
        rows={list.data?.items}
        getRowId={(s) => s.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada settlement. Impor laporan settlement dari seller center."
      />
    </div>
  )
}

function ReceivableList({
  search,
  onSearchChange,
}: {
  search: SettlementSearch
  onSearchChange: SearchUpdater<SettlementSearch>
}) {
  const openOnly = search.open ?? true
  const list = useQuery(
    marketplaceReceivablesQuery(search.page, openOnly, search.q)
  )
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <Input
          className="w-64"
          placeholder="Cari nomor order"
          aria-label="Cari nomor order"
          defaultValue={search.q}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              const q = e.currentTarget.value
              onSearchChange((s) => ({ ...s, q: q || undefined, page: 1 }))
            }
          }}
        />
        <Button
          variant={openOnly ? "default" : "outline"}
          className="min-h-11 sm:min-h-9"
          aria-pressed={openOnly}
          onClick={() =>
            onSearchChange((s) => ({ ...s, open: !openOnly, page: 1 }))
          }
        >
          Hanya yang belum lunas
        </Button>
      </div>
      {list.data ? (
        <p className="text-sm text-muted-foreground">
          Piutang {formatRupiah(list.data.totalReceivable)} · settlement{" "}
          {formatRupiah(list.data.totalSettled)} · sisa{" "}
          {formatRupiah(list.data.totalOutstanding)}
        </p>
      ) : null}
      <DataTable
        columns={[
          {
            id: "order",
            header: "Nomor Order",
            cell: (r) => r.externalOrderId,
          },
          { id: "so", header: "Sales Order", cell: (r) => r.number },
          {
            id: "market",
            header: "Marketplace",
            cell: (r) => r.marketplaceName,
          },
          {
            id: "handover",
            header: "Serah Kurir",
            cell: (r) => formatDate(r.handedOverDate),
          },
          {
            id: "receivable",
            header: "Piutang",
            cell: (r) => formatRupiah(r.receivable),
            align: "right",
          },
          {
            id: "settled",
            header: "Settlement",
            cell: (r) => formatRupiah(r.settled),
            align: "right",
          },
          {
            id: "draft",
            header: "Draft",
            cell: (r) => formatRupiah(r.draft),
            align: "right",
          },
          {
            id: "outstanding",
            header: "Sisa",
            cell: (r) => formatRupiah(r.outstanding),
            align: "right",
          },
        ]}
        rows={list.data?.items}
        getRowId={(r) => r.salesOrderId}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Tidak ada piutang marketplace."
      />
    </div>
  )
}

function SettlementDialog({
  id,
  onClose,
}: {
  id: string
  onClose: () => void
}) {
  const q = useQuery(settlementQuery(id))
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [reason, setReason] = useState("")
  const st = q.data
  const done = async (msg: string) => {
    toast.success(msg)
    await queryClient.invalidateQueries({ queryKey: ["reports"] })
  }
  const post = useIdempotentMutation({
    mutationFn: (_: void, key) => postSettlement(id, st?.version ?? 0, key),
    onSuccess: async () => {
      setConfirm(false)
      await done("Settlement diposting, jurnal A-10s dibuat")
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const cancel = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      cancelSettlement(id, st?.version ?? 0, reason, key),
    onSuccess: async () => {
      setCancelling(false)
      await done("Settlement dibatalkan")
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>
            {st
              ? `Settlement ${st.marketplaceName} ${st.payoutReference}`
              : "Settlement"}
          </DialogTitle>
          <DialogDescription>
            {st
              ? `Dana cair ${formatDate(st.settlementDate)} ke akun ${st.bankAccountCode} · ${st.lineCount} order`
              : "Memuat settlement"}
          </DialogDescription>
        </DialogHeader>
        <LoadingArea loading={q.isPending}>
          {st ? (
            <div className="flex max-h-[60vh] flex-col gap-3 overflow-y-auto">
              <div className="flex flex-wrap gap-4 text-sm">
                <StatusBadge status={st.status} />
                <span>Dana cair {formatRupiah(st.net)}</span>
                <span>Fee {formatRupiah(st.fee)}</span>
                <span>Promo {formatRupiah(st.promo)}</span>
                <span className="font-medium">
                  Pengurang piutang {formatRupiah(st.settled)}
                </span>
              </div>
              {st.cancelReason ? (
                <p className="text-sm text-muted-foreground">
                  Alasan batal: {st.cancelReason}
                </p>
              ) : null}
              <div className="overflow-x-auto rounded-lg border">
                <Table className="min-w-[44rem]">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Nomor Order</TableHead>
                      <TableHead>Sales Order</TableHead>
                      <TableHead className="text-right">Piutang</TableHead>
                      <TableHead className="text-right">Dana Cair</TableHead>
                      <TableHead className="text-right">Fee</TableHead>
                      <TableHead className="text-right">Promo</TableHead>
                      <TableHead className="text-right">Sisa Piutang</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {st.lines.map((l) => (
                      <TableRow key={l.lineNo}>
                        <TableCell>{l.externalOrderId}</TableCell>
                        <TableCell>{l.orderNumber}</TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatRupiah(l.receivable)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatRupiah(l.net)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatRupiah(l.fee)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatRupiah(l.promo)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {formatRupiah(l.outstanding)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
              {cancelling ? (
                <Field>
                  <FieldLabel htmlFor="settlement-cancel-reason">
                    Alasan pembatalan
                  </FieldLabel>
                  <Textarea
                    id="settlement-cancel-reason"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                </Field>
              ) : null}
            </div>
          ) : null}
        </LoadingArea>
        <DialogFooter>
          {st?.allowedActions.includes("cancel") ? (
            cancelling ? (
              <LoadingButton
                variant="destructive"
                className="min-h-11 sm:min-h-9"
                loading={cancel.isPending}
                disabled={!reason.trim()}
                onClick={() => cancel.mutate()}
              >
                Batalkan Settlement
              </LoadingButton>
            ) : (
              <Button
                variant="outline"
                className="min-h-11 sm:min-h-9"
                onClick={() => setCancelling(true)}
              >
                Batalkan
              </Button>
            )
          ) : null}
          {st?.allowedActions.includes("post") ? (
            <Button
              className="min-h-11 sm:min-h-9"
              onClick={() => setConfirm(true)}
            >
              Posting Settlement
            </Button>
          ) : null}
        </DialogFooter>
        <ConfirmDialog
          open={confirm}
          onOpenChange={setConfirm}
          title="Posting settlement?"
          description={
            st
              ? `Jurnal A-10s: Dr Bank ${formatRupiah(st.net)}, Dr Fee ${formatRupiah(st.fee)}, Dr Promo ${formatRupiah(st.promo)}, Cr Piutang Marketplace ${formatRupiah(st.settled)}. Jurnal tidak bisa diubah.`
              : ""
          }
          confirmLabel="Posting Settlement"
          onConfirm={() => post.mutate()}
          loading={post.isPending}
        />
      </DialogContent>
    </Dialog>
  )
}
