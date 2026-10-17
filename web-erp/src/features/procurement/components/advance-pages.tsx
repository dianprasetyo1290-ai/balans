import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { StatusBadge } from "@/components/common/status-badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  ADVANCE_STATUSES,
  advanceQuery,
  advancesQuery,
  cancelAdvance,
  postAdvance,
  type PurchaseAdvance,
  type PurchaseAdvanceStatus,
} from "../api/procurement-api"
import { AdvanceFormDialog } from "./advance-form-dialog"
import { DateRange } from "./request-list-page"
import type { DetailTab } from "./request-detail-page"
import { Info, ReasonDialog } from "./shared"

const route = getRouteApi("/_app/procurement/purchase-advances/")

const ALL = "all"

/** Pengadaan › Uang Muka Pembelian: daftar uang muka per PO (S-205). */
export function AdvanceListPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const advances = useQuery(advancesQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const [creating, setCreating] = useState(false)

  const columns: DataTableColumn<PurchaseAdvance>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (a) => (
        <Link
          to="/procurement/purchase-advances/$id"
          params={{ id: a.id }}
          search={{}}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {a.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal Bayar",
      cell: (a) => (
        <span className="whitespace-nowrap">{formatDate(a.paymentDate)}</span>
      ),
    },
    {
      id: "po",
      header: "Purchase Order",
      cell: (a) => (
        <div className="flex flex-col">
          <Link
            to="/procurement/purchase-orders/$id"
            params={{ id: a.purchaseOrderId }}
            search={{}}
            className="whitespace-nowrap text-primary underline-offset-4 hover:underline"
          >
            {a.purchaseOrderNumber}
          </Link>
          <span className="text-xs text-muted-foreground">
            {a.supplierName}
          </span>
        </div>
      ),
    },
    {
      id: "amount",
      header: "Nominal",
      align: "right",
      cell: (a) => formatRupiah(a.amount),
    },
    {
      id: "outstanding",
      header: "Sisa",
      align: "right",
      cell: (a) => formatRupiah(a.outstandingAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (a) => <StatusBadge status={a.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Uang Muka Pembelian
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Pembayaran di muka ke pemasok atau maklon per Purchase Order yang
            disetujui. Posting membuat jurnal A-19; sisa uang muka dipotong
            otomatis dari tagihan pemasok.
          </p>
        </div>
        {can("procurement.purchase_advance.write") ? (
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Catat Uang Muka
          </Button>
        ) : null}
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
            aria-label="Cari uang muka"
            placeholder="Cari nomor, PO, atau pemasok"
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
                status:
                  v === ALL || !v ? undefined : (v as PurchaseAdvanceStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === ALL ? "Semua status" : statusLabel(v).label
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {ADVANCE_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {statusLabel(s).label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <DateRange
          from={search.dateFrom}
          to={search.dateTo}
          onChange={(dateFrom, dateTo) =>
            void navigate({
              search: (s) => ({ ...s, dateFrom, dateTo, page: 1 }),
            })
          }
        />
      </div>
      <DataTable
        columns={columns}
        rows={advances.data?.items}
        getRowId={(a) => a.id}
        loading={advances.isPending}
        fetching={advances.isFetching}
        page={search.page}
        totalPages={advances.data?.totalPages ?? 0}
        totalItems={advances.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada uang muka pembelian."
      />
      <AdvanceFormDialog open={creating} onOpenChange={setCreating} />
    </div>
  )
}

const ADVANCE_FIELD_LABELS: Record<string, string> = {
  purchaseOrderId: "Purchase Order",
  amount: "Nominal",
  paymentDate: "Tanggal bayar",
  bankAccountCode: "Akun Kas & Bank",
  note: "Catatan",
  status: "Status",
  appliedAmount: "Dipotong tagihan",
  cancelReason: "Alasan dibatalkan",
}

type DetailProps = {
  id: string
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}

/** Detail uang muka: posting jurnal A-19, pembatalan (jurnal balik), dan Riwayat. */
export function AdvanceDetailPage({ id, tab, onTabChange }: DetailProps) {
  const advance = useQuery(advanceQuery(id))
  return (
    <LoadingArea loading={advance.isPending}>
      {advance.data ? (
        <AdvanceDetail
          advance={advance.data}
          tab={tab}
          onTabChange={onTabChange}
        />
      ) : null}
    </LoadingArea>
  )
}

function AdvanceDetail({
  advance,
  tab,
  onTabChange,
}: {
  advance: PurchaseAdvance
  tab: DetailTab
  onTabChange: (tab: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const [posting, setPosting] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [editing, setEditing] = useState(false)
  const can = (a: PurchaseAdvance["allowedActions"][number]) =>
    advance.allowedActions.includes(a)

  const mutation = useIdempotentMutation({
    mutationFn: (v: { kind: "post" | "cancel"; reason?: string }, key) =>
      v.kind === "post"
        ? postAdvance(advance.id, advance.version, key)
        : cancelAdvance(advance.id, advance.version, v.reason ?? "", key),
    onSuccess: async (_, v) => {
      setPosting(false)
      setCancelling(false)
      toast.success(
        v.kind === "post" ? "Uang muka diposting" : "Uang muka dibatalkan"
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
              {advance.number ?? "Draft Uang Muka"}
            </h1>
            <StatusBadge status={advance.status} />
          </div>
          <p className="text-sm text-muted-foreground">
            Uang Muka Pembelian · {advance.purchaseOrderNumber} ·{" "}
            {advance.supplierName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button variant="outline" onClick={() => setEditing(true)}>
              Ubah
            </Button>
          ) : null}
          {can("cancel") ? (
            <Button variant="outline" onClick={() => setCancelling(true)}>
              Batalkan
            </Button>
          ) : null}
          {can("post") ? (
            <Button onClick={() => setPosting(true)}>Posting Uang Muka</Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian uang muka"
        value={tab}
        onValueChange={onTabChange}
        tabs={[
          { id: "summary", label: "Ringkasan" },
          { id: "history", label: "Riwayat" },
        ]}
      />
      {tab === "summary" ? (
        <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
          <Info
            label="Purchase Order"
            value={
              <Link
                to="/procurement/purchase-orders/$id"
                params={{ id: advance.purchaseOrderId }}
                search={{}}
                className="text-primary underline-offset-4 hover:underline"
              >
                {advance.purchaseOrderNumber}
              </Link>
            }
          />
          <Info label="Pemasok" value={advance.supplierName} />
          <Info label="Tanggal Bayar" value={formatDate(advance.paymentDate)} />
          <Info label="Nominal" value={formatRupiah(advance.amount)} />
          <Info
            label="Dipotong Tagihan"
            value={formatRupiah(advance.appliedAmount)}
          />
          <Info
            label="Sisa Uang Muka"
            value={formatRupiah(advance.outstandingAmount)}
          />
          <Info label="Dibayar dari Akun" value={advance.bankAccountCode} />
          <Info
            label="Diposting"
            value={advance.postedAt ? formatDateTime(advance.postedAt) : "—"}
          />
          {advance.journalEntryId ? (
            <Info
              label="Jurnal A-19"
              value={
                <Link
                  to="/finance/journals/$id"
                  params={{ id: advance.journalEntryId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat jurnal
                </Link>
              }
            />
          ) : null}
          {advance.reversalJournalEntryId ? (
            <Info
              label="Jurnal Balik"
              value={
                <Link
                  to="/finance/journals/$id"
                  params={{ id: advance.reversalJournalEntryId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat jurnal balik
                </Link>
              }
            />
          ) : null}
          <Info label="Catatan" value={advance.note || "—"} />
          {advance.cancelReason ? (
            <Info label="Alasan Dibatalkan" value={advance.cancelReason} />
          ) : null}
        </dl>
      ) : (
        <EntityHistory
          entityType="purchase_advance"
          entityId={advance.id}
          fieldLabels={ADVANCE_FIELD_LABELS}
        />
      )}
      <ConfirmDialog
        open={posting}
        onOpenChange={setPosting}
        title="Posting uang muka?"
        description={`Nomor BYR diberikan dan jurnal A-19 dibuat: debit 1401 Uang Muka Pembelian, kredit akun ${advance.bankAccountCode} sebesar ${formatRupiah(advance.amount)}.`}
        confirmLabel="Posting Uang Muka"
        loading={mutation.isPending}
        onConfirm={() => mutation.mutate({ kind: "post" })}
      />
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan uang muka?"
        description={
          advance.status === "posted"
            ? "Jurnal A-19 dibalik di tanggal hari ini. Hanya uang muka yang belum dipotong tagihan yang bisa dibatalkan."
            : "Draft uang muka dibatalkan."
        }
        confirmLabel="Batalkan Uang Muka"
        required={advance.status === "posted"}
        destructive
        loading={mutation.isPending}
        onConfirm={(reason) => mutation.mutate({ kind: "cancel", reason })}
      />
      <AdvanceFormDialog
        open={editing}
        onOpenChange={setEditing}
        advance={advance}
      />
    </div>
  )
}
