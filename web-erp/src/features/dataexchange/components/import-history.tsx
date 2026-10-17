import { useQuery } from "@tanstack/react-query"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { Button } from "@/components/ui/button"
import { formatDateTime } from "@/lib/format/date"

import { importJobsQuery, type ImportJob } from "../api/dataexchange-api"
import { JobStatusBadge } from "./job-status-badge"

type ImportHistoryProps = {
  page: number
  onPageChange: (page: number) => void
  onOpen: (job: ImportJob) => void
}

/** Riwayat impor untuk jenis data yang boleh diimpor user. */
export function ImportHistory({
  page,
  onPageChange,
  onOpen,
}: ImportHistoryProps) {
  const jobs = useQuery(importJobsQuery(page))
  const columns: DataTableColumn<ImportJob>[] = [
    {
      id: "createdAt",
      header: "Waktu",
      cell: (j) => formatDateTime(j.createdAt),
    },
    { id: "kind", header: "Jenis data", cell: (j) => j.kindLabel },
    {
      id: "file",
      header: "File",
      cell: (j) => <span className="break-all">{j.fileName}</span>,
    },
    {
      id: "status",
      header: "Status",
      cell: (j) => <JobStatusBadge status={j.status} kind="import" />,
    },
    {
      id: "rows",
      header: "Hasil",
      cell: (j) =>
        j.status === "completed"
          ? `${j.committedRows} tersimpan`
          : j.totalRows > 0
            ? `${j.validRows} valid · ${j.errorRows} error`
            : "-",
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (j) => (
        <Button
          variant="outline"
          size="sm"
          className="min-h-11 sm:min-h-8"
          onClick={() => onOpen(j)}
        >
          Buka
        </Button>
      ),
    },
  ]
  return (
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
      onPageChange={onPageChange}
      emptyMessage="Belum ada impor."
    />
  )
}
