import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { useState } from "react"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { PageTabs } from "@/components/common/page-tabs"
import {
  approvalInboxQuery,
  approvalRequestsQuery,
  type ApprovalRequest,
  type ApprovalTask,
} from "@/features/approval/api/approval-api"
import { ApprovalDetailSheet } from "@/features/approval/components/approval-detail-sheet"
import {
  DecisionDialog,
  type Decision,
} from "@/features/approval/components/decision-dialog"
import { formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

const route = getRouteApi("/_app/procurement/approvals")

const SUBJECT_TYPES = ["purchase_request", "purchase_order"] as const

export type ProcurementApprovalTab = "inbox" | "requested" | "decided"

function documentCell(r: ApprovalRequest) {
  return (
    <div className="flex flex-col">
      {r.subjectLink ? (
        <Link
          to={r.subjectLink}
          className="font-medium text-primary underline-offset-4 hover:underline"
        >
          {r.subjectLabel}
        </Link>
      ) : (
        <span className="font-medium">{r.subjectLabel}</span>
      )}
      <span className="text-xs text-muted-foreground">
        {r.subjectTypeLabel}
        {r.summary ? ` · ${r.summary}` : ""}
      </span>
    </div>
  )
}

/**
 * Pengadaan › Persetujuan Pengadaan: inbox persetujuan khusus Purchase Request dan Purchase Order
 * (S-201, S-202). Inbox lintas dokumen tetap di Persetujuan Saya.
 */
export function ProcurementApprovalsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const prInbox = useQuery({
    ...approvalInboxQuery("purchase_request"),
    enabled: search.tab === "inbox",
  })
  const poInbox = useQuery({
    ...approvalInboxQuery("purchase_order"),
    enabled: search.tab === "inbox",
  })
  const historyType = search.subjectType ?? "purchase_order"
  const history = useQuery({
    ...approvalRequestsQuery({
      page: search.page,
      scope: search.tab === "decided" ? "decided" : "requested",
      subjectType: historyType,
    }),
    enabled: search.tab !== "inbox",
  })
  const [detail, setDetail] = useState<ApprovalRequest | null>(null)
  const [deciding, setDeciding] = useState<{
    request: ApprovalRequest
    decision: Decision
  } | null>(null)

  const tasks = [
    ...(prInbox.data?.items ?? []),
    ...(poInbox.data?.items ?? []),
  ].sort((a, b) => a.request.createdAt.localeCompare(b.request.createdAt))

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
              Atas nama {t.delegatedFromName}
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
      cell: (t) => (
        <span className="tabular-nums">{formatRupiah(t.request.amount)}</span>
      ),
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
            onClick={() => setDetail(t.request)}
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
      header: "Diajukan",
      cell: (r) => (
        <div className="flex flex-col">
          <span>{r.requestedByName}</span>
          <span className="text-xs text-muted-foreground">
            {formatDateTime(r.createdAt)}
          </span>
        </div>
      ),
    },
    {
      id: "amount",
      header: "Nilai",
      align: "right",
      cell: (r) => (
        <span className="tabular-nums">{formatRupiah(r.amount)}</span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (r) => <StatusBadge status={r.status} />,
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (r) => (
        <Button variant="ghost" size="sm" onClick={() => setDetail(r)}>
          Lihat
        </Button>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Persetujuan Pengadaan
          {search.tab === "inbox" ? ` (${tasks.length})` : ""}
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Purchase Request dan Purchase Order yang menunggu keputusan Anda.
          Tolak wajib catatan; dokumen lain ada di Persetujuan Saya.
        </p>
      </div>
      <PageTabs
        label="Bagian persetujuan pengadaan"
        value={search.tab}
        onValueChange={(tab) =>
          void navigate({ search: (s) => ({ ...s, tab, page: 1 }) })
        }
        tabs={[
          { id: "inbox", label: "Menunggu Saya" },
          { id: "requested", label: "Diajukan Saya" },
          { id: "decided", label: "Riwayat Keputusan" },
        ]}
      />
      {search.tab === "inbox" ? (
        <DataTable
          columns={inboxColumns}
          rows={prInbox.isPending || poInbox.isPending ? undefined : tasks}
          getRowId={(t) => t.request.id}
          loading={prInbox.isPending || poInbox.isPending}
          fetching={prInbox.isFetching || poInbox.isFetching}
          page={1}
          totalPages={1}
          totalItems={tasks.length}
          onSortChange={() => undefined}
          onPageChange={() => undefined}
          emptyMessage="Tidak ada Purchase Request atau Purchase Order yang menunggu persetujuan Anda."
        />
      ) : (
        <>
          <div
            className="flex gap-2"
            role="radiogroup"
            aria-label="Jenis dokumen"
          >
            {SUBJECT_TYPES.map((t) => (
              <Button
                key={t}
                size="sm"
                variant={historyType === t ? "default" : "outline"}
                role="radio"
                aria-checked={historyType === t}
                onClick={() =>
                  void navigate({
                    search: (s) => ({ ...s, subjectType: t, page: 1 }),
                  })
                }
              >
                {t === "purchase_order" ? "Purchase Order" : "Purchase Request"}
              </Button>
            ))}
          </div>
          <DataTable
            columns={historyColumns}
            rows={history.data?.items}
            getRowId={(r) => r.id}
            loading={history.isPending}
            fetching={history.isFetching}
            page={search.page}
            totalPages={history.data?.totalPages ?? 0}
            totalItems={history.data?.totalItems ?? 0}
            onSortChange={() => undefined}
            onPageChange={(page) =>
              void navigate({ search: (s) => ({ ...s, page }) })
            }
            emptyMessage="Belum ada riwayat."
          />
        </>
      )}
      <ApprovalDetailSheet
        request={detail}
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
