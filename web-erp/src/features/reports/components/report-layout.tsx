import { useQuery, useQueryClient } from "@tanstack/react-query"
import { DownloadIcon, RefreshCwIcon } from "lucide-react"
import type { ReactNode } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
import { TableCell, TableRow } from "@/components/ui/table"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  exportReport,
  readModelsQuery,
  refreshReadModels,
  type Period,
} from "../api/reports-api"
import { READ_MODEL_LABELS } from "../lib/reports-lib"

type ReportHeaderProps = {
  title: string
  description: string
  /** Jenis ekspor tanpa awalan `report.`, mis. `sales`. Kosong = tanpa tombol ekspor. */
  exportKind?: string
  exportParams?: Record<string, string | undefined>
  /** Permission ekspor (sama dengan permission baca laporan). */
  exportPermission?: string
  /** Read model sumber laporan; waktu refresh-nya ditampilkan. */
  readModels?: string[]
  actions?: ReactNode
}

/** Judul laporan + tombol Ekspor Excel (job) dan Perbarui Data (refresh read model). */
export function ReportHeader({
  title,
  description,
  exportKind,
  exportParams = {},
  exportPermission,
  readModels,
  actions,
}: ReportHeaderProps) {
  const { can } = usePermissions()
  const queryClient = useQueryClient()
  const status = useQuery({ ...readModelsQuery, enabled: !!readModels })
  const exporting = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      exportReport(exportKind ?? "", exportParams, key),
    onSuccess: () =>
      toast.success(
        "Ekspor sedang diproses. Tautan unduhan dikirim lewat notifikasi dan tersedia di Import/Export Excel › Ekspor."
      ),
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const refreshing = useIdempotentMutation({
    mutationFn: (_: void, key) => refreshReadModels(key),
    onSuccess: async (data) => {
      queryClient.setQueryData(readModelsQuery.queryKey, data)
      await queryClient.invalidateQueries({ queryKey: ["reports"] })
      toast.success("Data laporan diperbarui")
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const refreshed = (status.data?.items ?? []).filter((r) =>
    readModels?.includes(r.name)
  )
  const oldest = refreshed
    .map((r) => r.refreshedAt)
    .sort()
    .at(0)
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">{description}</p>
        {readModels ? (
          <p className="mt-1 text-xs text-muted-foreground">
            {oldest
              ? `Data per ${formatDateTime(oldest)} (${readModels
                  .map((n) => READ_MODEL_LABELS[n] ?? n)
                  .join(", ")}; diperbarui otomatis tiap 15 menit)`
              : "Data laporan belum pernah diperbarui."}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {actions}
        {readModels ? (
          <LoadingButton
            variant="outline"
            className="min-h-11 sm:min-h-9"
            loading={refreshing.isPending}
            onClick={() => refreshing.mutate()}
          >
            <RefreshCwIcon /> Perbarui Data
          </LoadingButton>
        ) : null}
        {exportKind && (!exportPermission || can(exportPermission)) ? (
          <LoadingButton
            variant="outline"
            className="min-h-11 sm:min-h-9"
            loading={exporting.isPending}
            onClick={() => exporting.mutate()}
          >
            <DownloadIcon /> Ekspor Excel
          </LoadingButton>
        ) : null}
      </div>
    </div>
  )
}

/** Filter periode tanggal awal–akhir. */
export function PeriodFilter({
  value,
  onChange,
}: {
  value: Period
  onChange: (p: Period) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="w-44">
        <DateInput
          value={value.dateFrom}
          onValueChange={(v) => onChange({ ...value, dateFrom: v })}
          placeholder="Tanggal awal"
        />
      </div>
      <span className="text-sm text-muted-foreground">s.d.</span>
      <div className="w-44">
        <DateInput
          value={value.dateTo}
          onValueChange={(v) => onChange({ ...value, dateTo: v })}
          placeholder="Tanggal akhir"
        />
      </div>
    </div>
  )
}

/** Baris kosong tabel laporan. */
export function EmptyRow({
  colSpan,
  children = "Tidak ada data pada filter ini.",
}: {
  colSpan: number
  children?: ReactNode
}) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan} className="text-muted-foreground">
        {children}
      </TableCell>
    </TableRow>
  )
}

/** Diagram batang horizontal sederhana (CSS) dengan label teks agar tetap terbaca pembaca layar. */
export function BarList({
  items,
  label,
  format,
}: {
  items: { key: string; label: string; value: number; display: string }[]
  label: string
  format?: (v: number) => string
}) {
  const max = Math.max(1, ...items.map((i) => Math.abs(i.value)))
  return (
    <ul className="flex flex-col gap-2" aria-label={label}>
      {items.map((i) => (
        <li
          key={i.key}
          className="grid grid-cols-[8rem_1fr] items-center gap-2 text-sm sm:grid-cols-[12rem_1fr]"
        >
          <span className="truncate" title={i.label}>
            {i.label}
          </span>
          <span className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className={
                i.value < 0
                  ? "h-3 rounded bg-destructive/70"
                  : "h-3 rounded bg-primary/70"
              }
              style={{
                width: `${Math.max(2, (Math.abs(i.value) / max) * 100)}%`,
              }}
            />
            <span className="shrink-0 text-muted-foreground tabular-nums">
              {format ? format(i.value) : i.display}
            </span>
          </span>
        </li>
      ))}
    </ul>
  )
}
