import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { auditLogsQuery } from "@/features/identity/api/identity-api"
import {
  AUDIT_ACTION_LABELS,
  AuditChanges,
} from "@/features/identity/components/audit-logs-page"
import { formatDateTime } from "@/lib/format/date"

type EntityHistoryProps = {
  /** Jenis entitas audit trail, mis. `product`, `warehouse` (didaftarkan backend lewat RegisterEntity). */
  entityType: string
  entityId: string
  /** Label Bahasa Indonesia untuk nama field API. */
  fieldLabels?: Record<string, string>
  /** Label nilai enum per field. */
  valueLabels?: Record<string, Record<string, string>>
}

/**
 * Tab Riwayat di halaman detail (S-105): siapa mengubah apa dan kapan untuk satu entitas, terbaru lebih dulu.
 * Cukup permission baca entitas tersebut; data sensitif sudah disamarkan backend.
 */
export function EntityHistory({
  entityType,
  entityId,
  fieldLabels,
  valueLabels,
}: EntityHistoryProps) {
  const [page, setPage] = useState(1)
  const logs = useQuery(auditLogsQuery({ page, entityType, entityId }))
  const items = logs.data?.items ?? []
  const totalPages = logs.data?.totalPages ?? 0

  return (
    <LoadingArea loading={logs.isPending} label="Memuat riwayat">
      {logs.isError ? (
        <p className="text-sm text-destructive">
          Riwayat tidak bisa dimuat. Coba muat ulang halaman.
        </p>
      ) : items.length === 0 ? (
        <p className="text-sm text-muted-foreground">Belum ada riwayat.</p>
      ) : (
        <div className="flex flex-col gap-3">
          <ol className="flex flex-col divide-y rounded-lg border">
            {items.map((l) => (
              <li
                key={l.id}
                className="flex flex-col gap-1 p-3 sm:flex-row sm:gap-4"
              >
                <div className="flex shrink-0 flex-col text-sm sm:w-48">
                  <span className="font-medium">{l.actorName}</span>
                  <span className="text-xs whitespace-nowrap text-muted-foreground">
                    {formatDateTime(l.createdAt)}
                  </span>
                </div>
                <div className="flex min-w-0 flex-col gap-1">
                  <div>
                    <Badge variant="secondary">
                      {AUDIT_ACTION_LABELS[l.action]}
                    </Badge>
                  </div>
                  <AuditChanges
                    log={l}
                    fieldLabels={fieldLabels}
                    valueLabels={valueLabels}
                  />
                </div>
              </li>
            ))}
          </ol>
          {totalPages > 1 ? (
            <div className="flex items-center justify-between gap-2 text-sm text-muted-foreground">
              <span>
                Halaman {page} dari {totalPages}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  Sebelumnya
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  Berikutnya
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      )}
    </LoadingArea>
  )
}
