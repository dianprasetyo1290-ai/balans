import { useQuery, useQueryClient } from "@tanstack/react-query"
import { DownloadIcon, FileSpreadsheetIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { Button } from "@/components/ui/button"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createExportJob,
  downloadExport,
  exportJobsQuery,
  type DataExchangeKind,
  type ExportJob,
} from "../api/dataexchange-api"
import { JobStatusBadge } from "./job-status-badge"

type ExportPanelProps = {
  kinds: DataExchangeKind[]
  loading: boolean
  /** Jenis data yang dibuka dari tombol Ekspor halaman master. */
  highlight?: string
}

/** Ekspor master ke xlsx sebagai job; file siap diunduh dari riwayat dan notifikasi. */
export function ExportPanel({ kinds, loading, highlight }: ExportPanelProps) {
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const jobs = useQuery(exportJobsQuery(page))
  const [pending, setPending] = useState<string | null>(null)

  const start = useIdempotentMutation({
    mutationFn: (kind: string, key) => createExportJob(kind, key),
    onMutate: (kind) => setPending(kind),
    onSettled: () => setPending(null),
    onSuccess: async (job) => {
      toast.success(
        `Ekspor ${job.kindLabel} diproses. Notifikasi dikirim saat file siap.`
      )
      setPage(1)
      await queryClient.invalidateQueries({
        queryKey: ["dataexchange", "export-jobs"],
      })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const columns: DataTableColumn<ExportJob>[] = [
    {
      id: "createdAt",
      header: "Waktu",
      cell: (j) => formatDateTime(j.createdAt),
    },
    { id: "kind", header: "Jenis data", cell: (j) => j.kindLabel },
    {
      id: "status",
      header: "Status",
      cell: (j) => <JobStatusBadge status={j.status} kind="export" />,
    },
    {
      id: "rows",
      header: "Baris",
      align: "right",
      cell: (j) => (j.status === "completed" ? j.totalRows : "-"),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (j) =>
        j.status === "completed" ? (
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 sm:min-h-8"
            onClick={() =>
              void downloadExport(j.id).catch((err: unknown) =>
                showErrorDialog(parseProblem(err))
              )
            }
          >
            <DownloadIcon /> Unduh
          </Button>
        ) : j.status === "failed" ? (
          <span className="text-xs text-destructive">{j.message}</span>
        ) : null,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <LoadingArea loading={loading}>
        {kinds.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Anda belum memiliki akses ekspor data.
          </p>
        ) : (
          <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {kinds.map((k) => (
              <li
                key={k.kind}
                className={cn(
                  "flex items-center justify-between gap-3 rounded-lg border p-3",
                  highlight === k.kind && "border-primary ring-1 ring-primary"
                )}
              >
                <span className="flex items-center gap-2 text-sm font-medium">
                  <FileSpreadsheetIcon
                    aria-hidden="true"
                    className="size-4 text-muted-foreground"
                  />
                  {k.label}
                </span>
                <LoadingButton
                  variant={highlight === k.kind ? "default" : "outline"}
                  size="sm"
                  className="min-h-11 sm:min-h-8"
                  loading={pending === k.kind}
                  disabled={pending !== null && pending !== k.kind}
                  onClick={() => start.mutate(k.kind)}
                >
                  Ekspor {k.label}
                </LoadingButton>
              </li>
            ))}
          </ul>
        )}
      </LoadingArea>
      <div className="flex flex-col gap-2">
        <h2 className="text-lg font-semibold">Riwayat Ekspor</h2>
        <DataTable
          columns={columns}
          rows={jobs.data?.items}
          getRowId={(j) => j.id}
          loading={jobs.isPending}
          fetching={jobs.isFetching}
          page={page}
          totalPages={jobs.data?.pagination.totalPages ?? 0}
          totalItems={jobs.data?.pagination.totalItems ?? 0}
          onSortChange={() => undefined}
          onPageChange={setPage}
          emptyMessage="Belum ada ekspor."
        />
      </div>
    </div>
  )
}
