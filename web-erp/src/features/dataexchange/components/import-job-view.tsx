import { useQuery, useQueryClient } from "@tanstack/react-query"
import {
  AlertCircleIcon,
  CheckCircle2Icon,
  FileDownIcon,
  RotateCcwIcon,
} from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { ProgressDialog } from "@/components/common/progress-dialog"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  cancelImportJob,
  commitImportJob,
  downloadImportErrors,
  importJobQuery,
  importRowsQuery,
  isImportRunning,
  type DataExchangeKind,
  type ImportJob,
  type ImportJobRow,
} from "../api/dataexchange-api"
import { ImportSteps } from "./import-steps"
import { JobStatusBadge } from "./job-status-badge"

const number = new Intl.NumberFormat("id-ID")

function stepOf(job: ImportJob): number {
  switch (job.status) {
    case "uploaded":
    case "validating":
      return 2
    case "validated":
    case "committing":
      return 3
    case "completed":
      return 4
  }
  return 2
}

function progressTitle(job: ImportJob) {
  return job.status === "committing" ? "Menyimpan data" : "Memeriksa file"
}

function progressMessage(job: ImportJob) {
  if (job.status === "uploaded") {
    return "Menunggu giliran pemeriksaan…"
  }
  if (job.status === "committing") {
    return `Menyimpan baris ${number.format(job.committedRows)} dari ${number.format(job.validRows)}`
  }
  if (job.totalRows === 0) {
    return "Membaca file…"
  }
  return `Memeriksa baris ${number.format(job.processedRows)} dari ${number.format(job.totalRows)}`
}

type ImportJobViewProps = {
  id: string
  kind?: DataExchangeKind
  onReset: () => void
}

/** Langkah 3–4 wizard impor: progress, hasil pemeriksaan, baris error, dan konfirmasi simpan. */
export function ImportJobView({ id, kind, onReset }: ImportJobViewProps) {
  const queryClient = useQueryClient()
  const query = useQuery(importJobQuery(id))
  const [background, setBackground] = useState(false)
  const [downloading, setDownloading] = useState(false)
  const job = query.data

  const refresh = async () => {
    await queryClient.invalidateQueries({
      queryKey: ["dataexchange", "import-jobs"],
    })
  }
  const commit = useIdempotentMutation({
    mutationFn: (_: void, key) => commitImportJob(id, key),
    onSuccess: async () => {
      setBackground(false)
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const cancel = useIdempotentMutation({
    mutationFn: (_: void, key) => cancelImportJob(id, key),
    onSuccess: async () => {
      toast.success("Impor dibatalkan")
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  if (!job) {
    return <LoadingArea loading={query.isPending}>{null}</LoadingArea>
  }
  const running = isImportRunning(job.status)
  const canCommit = job.allowedActions.includes("commit")
  const canCancel = job.allowedActions.includes("cancel")
  const hasErrors = job.allowedActions.includes("download_errors")

  return (
    <div className="flex flex-col gap-4">
      <ImportSteps current={stepOf(job)} />
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            Impor {job.kindLabel}
            <JobStatusBadge status={job.status} kind="import" />
          </CardTitle>
          <CardDescription>
            {job.fileName} · diunggah {formatDateTime(job.createdAt)}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {job.totalRows > 0 ? (
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-sm">
                <span className="font-medium">
                  {number.format(job.totalRows)} baris
                </span>{" "}
                ·{" "}
                <span className="text-emerald-700 dark:text-emerald-300">
                  {number.format(job.validRows)} valid
                </span>{" "}
                ·{" "}
                <span
                  className={job.errorRows > 0 ? "text-destructive" : undefined}
                >
                  {number.format(job.errorRows)} error
                </span>
              </p>
              {hasErrors ? (
                <LoadingButton
                  variant="outline"
                  className="min-h-11 sm:min-h-9"
                  loading={downloading}
                  onClick={() => {
                    setDownloading(true)
                    downloadImportErrors(id)
                      .catch((err: unknown) =>
                        showErrorDialog(parseProblem(err))
                      )
                      .finally(() => setDownloading(false))
                  }}
                >
                  <FileDownIcon /> Unduh baris error (.xlsx)
                </LoadingButton>
              ) : null}
            </div>
          ) : null}
          {job.status === "completed" ? (
            <p className="flex items-center gap-2 text-sm text-emerald-700 dark:text-emerald-300">
              <CheckCircle2Icon aria-hidden="true" className="size-4" />
              {number.format(job.committedRows)} baris {job.kindLabel}{" "}
              tersimpan.
            </p>
          ) : null}
          {job.status === "failed" ? (
            <p
              role="alert"
              className="flex items-start gap-2 text-sm text-destructive"
            >
              <AlertCircleIcon
                aria-hidden="true"
                className="mt-0.5 size-4 shrink-0"
              />
              {job.message || "Impor gagal."}
            </p>
          ) : null}
          {job.status === "cancelled" ? (
            <p className="text-sm text-muted-foreground">
              Impor dibatalkan. File yang sama boleh diunggah ulang.
            </p>
          ) : null}
        </CardContent>
      </Card>

      {job.status === "validated" ||
      (job.status === "failed" && job.totalRows > 0) ? (
        <ImportRows job={job} kind={kind} />
      ) : null}

      <div className="flex flex-wrap justify-end gap-2">
        {canCancel ? (
          <LoadingButton
            variant="outline"
            className="min-h-11 sm:min-h-9"
            loading={cancel.isPending}
            disabled={commit.isPending}
            onClick={() => cancel.mutate()}
          >
            Batal
          </LoadingButton>
        ) : null}
        {canCommit ? (
          <LoadingButton
            className="min-h-11 sm:min-h-9"
            loading={commit.isPending}
            loadingText="Menyimpan…"
            disabled={cancel.isPending}
            onClick={() => commit.mutate()}
          >
            Simpan {number.format(job.validRows)} baris valid
          </LoadingButton>
        ) : null}
        {!running && !canCommit ? (
          <Button
            variant="outline"
            className="min-h-11 sm:min-h-9"
            onClick={onReset}
          >
            <RotateCcwIcon /> Impor file lain
          </Button>
        ) : null}
      </div>

      <ProgressDialog
        open={running && !background}
        title={progressTitle(job)}
        progress={job.progress}
        message={progressMessage(job)}
        onRunInBackground={() => {
          setBackground(true)
          toast.info(
            "Impor berjalan di latar belakang. Notifikasi dikirim saat selesai."
          )
        }}
      />
    </div>
  )
}

type RowsTab = "errors" | "valid"

function ImportRows({
  job,
  kind,
}: {
  job: ImportJob
  kind?: DataExchangeKind
}) {
  const [tab, setTab] = useState<RowsTab>(
    job.errorRows > 0 ? "errors" : "valid"
  )
  const [page, setPage] = useState(1)
  const rows = useQuery(importRowsQuery(job.id, tab === "valid", page))
  const columns = kind?.columns ?? []
  const header = (key?: string) =>
    key ? (columns.find((c) => c.key === key)?.header ?? key) : "-"

  const errorColumns: DataTableColumn<ImportJobRow>[] = [
    { id: "row", header: "Baris", cell: (r) => `Baris ${r.rowNumber}` },
    {
      id: "column",
      header: "Kolom",
      cell: (r) => (
        <ul className="flex flex-col gap-1">
          {r.errors.map((e, i) => (
            <li key={i}>{header(e.column)}</li>
          ))}
        </ul>
      ),
    },
    {
      id: "message",
      header: "Keterangan",
      cell: (r) => (
        <ul className="flex flex-col gap-1 text-destructive">
          {r.errors.map((e, i) => (
            <li key={i}>{e.message}</li>
          ))}
        </ul>
      ),
    },
  ]
  const previewColumns: DataTableColumn<ImportJobRow>[] = [
    { id: "row", header: "Baris", cell: (r) => r.rowNumber },
    ...columns.slice(0, 6).map((c) => ({
      id: c.key,
      header: c.header,
      cell: (r: ImportJobRow) => r.values[c.key] ?? "",
    })),
  ]

  return (
    <div className="flex flex-col gap-3">
      <PageTabs
        label="Hasil pemeriksaan"
        value={tab}
        onValueChange={(t) => {
          setTab(t)
          setPage(1)
        }}
        tabs={[
          {
            id: "errors",
            label: `Baris error (${number.format(job.errorRows)})`,
          },
          {
            id: "valid",
            label: `Baris valid (${number.format(job.validRows)})`,
          },
        ]}
      />
      <DataTable
        columns={tab === "errors" ? errorColumns : previewColumns}
        rows={rows.data?.items}
        getRowId={(r) => String(r.rowNumber)}
        loading={rows.isPending}
        fetching={rows.isFetching}
        page={page}
        totalPages={rows.data?.pagination.totalPages ?? 0}
        totalItems={rows.data?.pagination.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={setPage}
        emptyMessage={
          tab === "errors" ? "Tidak ada baris error." : "Tidak ada baris valid."
        }
      />
    </div>
  )
}
