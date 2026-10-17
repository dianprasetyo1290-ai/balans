import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { PlusIcon, UserCheckIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"
import { useQueryClient } from "@tanstack/react-query"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  approvalInboxQuery,
  approvalRequestsQuery,
  cancelDelegation,
  delegationsQuery,
  subjectTypesQuery,
  type ApprovalDelegation,
  type ApprovalRequest,
  type ApprovalTask,
} from "../api/approval-api"
import { ApprovalDetailSheet } from "./approval-detail-sheet"
import { stepApprover } from "./approval-steps"
import { DecisionDialog, type Decision } from "./decision-dialog"
import { DelegationDialog } from "./delegation-dialog"

const route = getRouteApi("/_app/approvals")

export type ApprovalTab = "inbox" | "requested" | "decided" | "delegations"

const TABS: { id: ApprovalTab; label: string }[] = [
  { id: "inbox", label: "Menunggu Saya" },
  { id: "requested", label: "Diajukan Saya" },
  { id: "decided", label: "Riwayat Keputusan" },
  { id: "delegations", label: "Delegasi" },
]

const ALL = "all"

function amountCell(amount: string) {
  return Number(amount) > 0 ? (
    <span className="tabular-nums">{formatRupiah(amount)}</span>
  ) : (
    "—"
  )
}

function documentCell(r: ApprovalRequest) {
  return (
    <div className="flex flex-col">
      <span className="font-medium">{r.subjectLabel}</span>
      <span className="text-xs text-muted-foreground">
        {r.subjectTypeLabel}
        {r.summary ? ` · ${r.summary}` : ""}
      </span>
    </div>
  )
}

/** Persetujuan Saya: inbox lintas jenis dokumen, riwayat, dan delegasi (S-110, S-111; Desain UI/UX 3.5). */
export function ApprovalInboxPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { user } = usePermissions()
  const subjectTypes = useQuery(subjectTypesQuery)
  const inbox = useQuery({
    ...approvalInboxQuery(search.subjectType),
    enabled: search.tab === "inbox",
  })
  const history = useQuery({
    ...approvalRequestsQuery({
      page: search.page,
      scope: search.tab === "decided" ? "decided" : "requested",
      subjectType: search.subjectType,
    }),
    enabled: search.tab === "requested" || search.tab === "decided",
  })
  const [detail, setDetail] = useState<{
    request: ApprovalRequest
    delegatedFromName?: string | null
  } | null>(null)
  const [deciding, setDeciding] = useState<{
    request: ApprovalRequest
    decision: Decision
  } | null>(null)

  const decide = (request: ApprovalRequest, decision: Decision) => {
    setDetail(null)
    setDeciding({ request, decision })
  }

  const inboxColumns: DataTableColumn<ApprovalTask>[] = [
    {
      id: "document",
      header: "Dokumen",
      cell: (t) => (
        <div className="flex flex-col gap-1">
          {documentCell(t.request)}
          {t.delegatedFromName ? (
            <Badge variant="outline" className="w-fit">
              <UserCheckIcon aria-hidden="true" /> Atas nama{" "}
              {t.delegatedFromName}
            </Badge>
          ) : null}
        </div>
      ),
    },
    {
      id: "requestedBy",
      header: "Diajukan oleh",
      cell: (t) => (
        <div className="flex flex-col">
          <span>{t.request.requestedByName}</span>
          <span className="text-xs text-muted-foreground">
            {formatDateTime(t.request.createdAt)}
          </span>
        </div>
      ),
    },
    {
      id: "amount",
      header: "Nilai",
      align: "right",
      cell: (t) => amountCell(t.request.amount),
    },
    {
      id: "level",
      header: "Level",
      cell: (t) => `${t.request.currentLevel} dari ${t.request.totalLevels}`,
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (t) => (
        <div className="flex justify-end gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={() =>
              setDetail({
                request: t.request,
                delegatedFromName: t.delegatedFromName,
              })
            }
          >
            Lihat
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => decide(t.request, "reject")}
          >
            Tolak
          </Button>
          <Button size="sm" onClick={() => decide(t.request, "approve")}>
            Setujui
          </Button>
        </div>
      ),
    },
  ]

  const historyColumns: DataTableColumn<ApprovalRequest>[] = [
    { id: "document", header: "Dokumen", cell: documentCell },
    {
      id: "requestedBy",
      header: search.tab === "decided" ? "Diajukan oleh" : "Diajukan",
      cell: (r) => (
        <div className="flex flex-col">
          {search.tab === "decided" ? <span>{r.requestedByName}</span> : null}
          <span
            className={cn(
              search.tab === "decided" && "text-xs text-muted-foreground"
            )}
          >
            {formatDateTime(r.createdAt)}
          </span>
        </div>
      ),
    },
    {
      id: "amount",
      header: "Nilai",
      align: "right",
      cell: (r) => amountCell(r.amount),
    },
    {
      id: "status",
      header: "Status",
      cell: (r) => {
        const pending = r.steps.find((s) => s.status === "pending")
        return (
          <div className="flex flex-col gap-1">
            <StatusBadge status={r.status} />
            {pending ? (
              <span className="text-xs text-muted-foreground">
                Level {pending.level}: {stepApprover(pending)}
              </span>
            ) : null}
          </div>
        )
      },
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (r) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setDetail({ request: r })}
        >
          Lihat
        </Button>
      ),
    },
  ]

  const typeFilter =
    search.tab !== "delegations" ? (
      <Select
        value={search.subjectType ?? ALL}
        onValueChange={(v) =>
          void navigate({
            search: (s) => ({
              ...s,
              subjectType: v === ALL || !v ? undefined : String(v),
              page: 1,
            }),
          })
        }
      >
        <SelectTrigger className="w-full sm:w-64" aria-label="Jenis dokumen">
          <SelectValue>
            {(v: string) =>
              v === ALL
                ? "Semua jenis"
                : (subjectTypes.data?.find((t) => t.type === v)?.label ?? v)
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Semua jenis</SelectItem>
          {(subjectTypes.data ?? []).map((t) => (
            <SelectItem key={t.type} value={t.type}>
              {t.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    ) : null

  const inboxCount = inbox.data?.totalItems

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Persetujuan Saya
          {search.tab === "inbox" && inboxCount !== undefined
            ? ` (${inboxCount})`
            : ""}
        </h1>
        <p className="text-sm text-muted-foreground">
          Dokumen yang menunggu keputusan Anda, riwayat pengajuan, dan delegasi
          saat cuti.
        </p>
      </div>
      <div
        role="tablist"
        aria-label="Bagian persetujuan"
        className="flex gap-1 overflow-x-auto border-b"
      >
        {TABS.map((tab) => {
          const active = search.tab === tab.id
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              aria-selected={active}
              className={cn(
                "-mb-px min-h-11 shrink-0 border-b-2 px-3 text-sm font-medium transition-colors",
                active
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              )}
              onClick={() =>
                void navigate({
                  search: (s) => ({ ...s, tab: tab.id, page: 1 }),
                })
              }
            >
              {tab.label}
            </button>
          )
        })}
      </div>
      {typeFilter}
      {search.tab === "inbox" ? (
        <InboxCards
          tasks={inbox.data?.items}
          loading={inbox.isPending}
          onView={(t) =>
            setDetail({
              request: t.request,
              delegatedFromName: t.delegatedFromName,
            })
          }
          onDecide={decide}
        />
      ) : null}
      {search.tab === "inbox" ? (
        <div className="hidden md:block">
          <DataTable
            columns={inboxColumns}
            rows={inbox.data?.items}
            getRowId={(t) => t.request.id}
            loading={inbox.isPending}
            fetching={inbox.isFetching}
            page={1}
            totalPages={1}
            totalItems={inbox.data?.totalItems ?? 0}
            onSortChange={() => {}}
            onPageChange={() => {}}
            emptyMessage="Tidak ada dokumen yang menunggu persetujuan Anda."
          />
        </div>
      ) : null}
      {search.tab === "requested" || search.tab === "decided" ? (
        <DataTable
          columns={historyColumns}
          rows={history.data?.items}
          getRowId={(r) => r.id}
          loading={history.isPending}
          fetching={history.isFetching}
          page={search.page}
          totalPages={history.data?.totalPages ?? 0}
          totalItems={history.data?.totalItems ?? 0}
          onSortChange={() => {}}
          onPageChange={(page) =>
            void navigate({ search: (s) => ({ ...s, page }) })
          }
          emptyMessage={
            search.tab === "requested"
              ? "Belum ada dokumen yang Anda ajukan."
              : "Belum ada dokumen yang Anda putuskan."
          }
        />
      ) : null}
      {search.tab === "delegations" && user ? (
        <DelegationsSection currentUserId={user.id} all={false} />
      ) : null}
      <ApprovalDetailSheet
        request={detail?.request ?? null}
        delegatedFromName={detail?.delegatedFromName}
        onOpenChange={(open) => !open && setDetail(null)}
        onDecide={decide}
      />
      <DecisionDialog
        request={deciding?.request ?? null}
        decision={deciding?.decision ?? "approve"}
        onOpenChange={(open) => !open && setDeciding(null)}
      />
    </div>
  )
}

type InboxCardsProps = {
  tasks: ApprovalTask[] | undefined
  loading: boolean
  onView: (t: ApprovalTask) => void
  onDecide: (r: ApprovalRequest, d: Decision) => void
}

/** Inbox di HP: kartu dengan tombol 44 px agar Setujui/Tolak tidak perlu digeser (Technical Guideline 10.10). */
function InboxCards({ tasks, loading, onView, onDecide }: InboxCardsProps) {
  if (loading || !tasks) {
    return null
  }
  return (
    <ul
      className="flex flex-col gap-3 md:hidden"
      aria-label="Dokumen menunggu persetujuan"
    >
      {tasks.length === 0 ? (
        <li className="rounded-lg border p-4 text-center text-sm text-muted-foreground">
          Tidak ada dokumen yang menunggu persetujuan Anda.
        </li>
      ) : null}
      {tasks.map((t) => (
        <li
          key={t.request.id}
          className="flex flex-col gap-3 rounded-lg border p-4"
        >
          <button
            type="button"
            className="flex flex-col gap-1 text-left"
            onClick={() => onView(t)}
          >
            {documentCell(t.request)}
            <span className="text-sm">
              {t.request.requestedByName} ·{" "}
              {formatDateTime(t.request.createdAt)}
            </span>
            <span className="flex flex-wrap items-center gap-x-3 text-sm">
              {Number(t.request.amount) > 0 ? (
                <span className="font-medium tabular-nums">
                  {formatRupiah(t.request.amount)}
                </span>
              ) : null}
              <span className="text-muted-foreground">
                Level {t.request.currentLevel} dari {t.request.totalLevels}
              </span>
            </span>
            {t.delegatedFromName ? (
              <Badge variant="outline" className="w-fit">
                <UserCheckIcon aria-hidden="true" /> Atas nama{" "}
                {t.delegatedFromName}
              </Badge>
            ) : null}
          </button>
          <div className="grid grid-cols-2 gap-2">
            <Button
              variant="outline"
              className="h-11"
              onClick={() => onDecide(t.request, "reject")}
            >
              Tolak
            </Button>
            <Button
              className="h-11"
              onClick={() => onDecide(t.request, "approve")}
            >
              Setujui
            </Button>
          </div>
        </li>
      ))}
    </ul>
  )
}

type DelegationsSectionProps = {
  currentUserId: string
  /** true = semua user (Pengaturan › Matriks Persetujuan). */
  all: boolean
  canManageOthers?: boolean
}

/** Daftar delegasi dengan aksi buat dan hentikan. */
export function DelegationsSection({
  currentUserId,
  all,
  canManageOthers = false,
}: DelegationsSectionProps) {
  const queryClient = useQueryClient()
  const [includePast, setIncludePast] = useState(false)
  const delegations = useQuery(delegationsQuery(all, includePast))
  const [open, setOpen] = useState(false)
  const [stopping, setStopping] = useState<ApprovalDelegation | null>(null)

  const stop = useIdempotentMutation({
    mutationFn: (d: ApprovalDelegation, key) =>
      cancelDelegation(d.id, d.version, key),
    onSuccess: async () => {
      toast.success("Delegasi dihentikan")
      setStopping(null)
      await queryClient.invalidateQueries({
        queryKey: ["approval", "delegations"],
      })
    },
    onError: (err) => {
      setStopping(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const STATUS: Record<ApprovalDelegation["status"], string> = {
    scheduled: "Terjadwal",
    active: "Berlaku",
    ended: "Berakhir",
    cancelled: "Dihentikan",
  }

  const columns: DataTableColumn<ApprovalDelegation>[] = [
    {
      id: "from",
      header: "Dari",
      cell: (d) => (d.fromUserId === currentUserId ? "Saya" : d.fromUserName),
    },
    {
      id: "to",
      header: "Kepada",
      cell: (d) => (d.toUserId === currentUserId ? "Saya" : d.toUserName),
    },
    {
      id: "period",
      header: "Periode",
      cell: (d) => `${formatDate(d.validFrom)} – ${formatDate(d.validTo)}`,
    },
    { id: "reason", header: "Alasan", cell: (d) => d.reason || "—" },
    {
      id: "status",
      header: "Status",
      cell: (d) => (
        <Badge
          variant="outline"
          className={cn(
            d.status === "active" &&
              "border-transparent bg-emerald-500/15 text-emerald-800 dark:text-emerald-300"
          )}
        >
          {STATUS[d.status]}
        </Badge>
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (d) =>
        (d.status === "active" || d.status === "scheduled") &&
        (d.fromUserId === currentUserId || canManageOthers) ? (
          <Button variant="ghost" size="sm" onClick={() => setStopping(d)}>
            Hentikan
          </Button>
        ) : null,
    },
  ]

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-(--color-primary)"
            checked={includePast}
            onChange={(e) => setIncludePast(e.target.checked)}
          />
          Tampilkan yang sudah berakhir
        </label>
        <Button onClick={() => setOpen(true)}>
          <PlusIcon /> Delegasikan Persetujuan
        </Button>
      </div>
      <DataTable
        columns={columns}
        rows={delegations.data}
        getRowId={(d) => d.id}
        loading={delegations.isPending}
        fetching={delegations.isFetching}
        page={1}
        totalPages={1}
        totalItems={delegations.data?.length ?? 0}
        onSortChange={() => {}}
        onPageChange={() => {}}
        emptyMessage="Belum ada delegasi yang berlaku."
      />
      <DelegationDialog
        open={open}
        onOpenChange={setOpen}
        currentUserId={currentUserId}
        canChooseFrom={canManageOthers}
      />
      {stopping ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setStopping(null)}
          title="Hentikan delegasi?"
          description={`${stopping.toUserName} tidak bisa lagi memutuskan persetujuan atas nama ${stopping.fromUserName}.`}
          confirmLabel="Hentikan Delegasi"
          destructive
          loading={stop.isPending}
          onConfirm={() => stop.mutate(stopping)}
        />
      ) : null}
    </div>
  )
}
