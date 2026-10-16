import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { Badge } from "@/components/ui/badge"
import { formatDateTime } from "@/lib/format/date"

import { auditLogsQuery, type AuditLog } from "../api/identity-api"

const route = getRouteApi("/_app/settings/audit-logs")

export const AUDIT_ACTION_LABELS: Record<AuditLog["action"], string> = {
  create: "Dibuat",
  update: "Diubah",
  delete: "Dihapus",
  restore: "Dipulihkan",
  status_change: "Status diubah",
  approve: "Disetujui",
  reject: "Ditolak",
  submit: "Diajukan",
  post: "Diposting",
  cancel: "Dibatalkan",
  login: "Masuk",
  login_failed: "Gagal masuk",
  logout: "Keluar",
  import: "Diimpor",
  export: "Diekspor",
}

const ENTITY_LABELS: Record<string, string> = { user: "Pengguna", role: "Role" }

function formatValue(v: unknown): string {
  if (v === null || v === undefined) {
    return "—"
  }
  if (typeof v === "boolean") {
    return v ? "Ya" : "Tidak"
  }
  if (Array.isArray(v)) {
    return v.length ? v.join(", ") : "—"
  }
  return typeof v === "object" ? JSON.stringify(v) : String(v)
}

/** Daftar perubahan per field; dipakai juga tab Riwayat di halaman detail dokumen. */
export function AuditChanges({ log }: { log: AuditLog }) {
  const entries = Object.entries(log.changes)
  if (entries.length === 0) {
    return log.note ? (
      <span className="text-muted-foreground">{log.note}</span>
    ) : (
      <span className="text-muted-foreground">—</span>
    )
  }
  return (
    <ul className="flex flex-col gap-0.5 text-xs">
      {entries.map(([field, c]) => (
        <li key={field}>
          <span className="font-medium">{field}</span>: {formatValue(c.from)} →{" "}
          {formatValue(c.to)}
        </li>
      ))}
      {log.note ? <li className="text-muted-foreground">{log.note}</li> : null}
    </ul>
  )
}

/** Pengaturan › Riwayat Perubahan (S-105). */
export function AuditLogsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const logs = useQuery(auditLogsQuery(search))

  const columns: DataTableColumn<AuditLog>[] = [
    {
      id: "time",
      header: "Waktu",
      cell: (l) => (
        <span className="whitespace-nowrap">{formatDateTime(l.createdAt)}</span>
      ),
    },
    { id: "actor", header: "Oleh", cell: (l) => l.actorName },
    {
      id: "action",
      header: "Aksi",
      cell: (l) => (
        <Badge variant="secondary">{AUDIT_ACTION_LABELS[l.action]}</Badge>
      ),
    },
    {
      id: "entity",
      header: "Data",
      cell: (l) => (
        <div className="flex flex-col">
          <span>{l.entityLabel || l.entityId}</span>
          <span className="text-xs text-muted-foreground">
            {ENTITY_LABELS[l.entityType] ?? l.entityType}
          </span>
        </div>
      ),
    },
    {
      id: "changes",
      header: "Perubahan",
      cell: (l) => <AuditChanges log={l} />,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Riwayat Perubahan
        </h1>
        <p className="text-sm text-muted-foreground">
          Siapa mengubah apa dan kapan. Data sensitif disamarkan.
        </p>
      </div>
      <DataTable
        columns={columns}
        rows={logs.data?.items}
        getRowId={(l) => l.id}
        loading={logs.isPending}
        fetching={logs.isFetching}
        page={search.page}
        totalPages={logs.data?.totalPages ?? 0}
        totalItems={logs.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada riwayat perubahan."
      />
    </div>
  )
}
