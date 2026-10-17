import { useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { StatusBadge } from "@/components/common/status-badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { ensureFiscalPeriods, fiscalPeriodsQuery } from "../api/accounting-api"

/** Keuangan & Akuntansi › Periode Akuntansi: periode bulanan Terbuka/Ditutup (ACC-08, S-133). */
export function FiscalPeriodsPage() {
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const periods = useQuery(fiscalPeriodsQuery())
  const ensure = useIdempotentMutation({
    mutationFn: (_: void, key) => ensureFiscalPeriods(key),
    onSuccess: async (r) => {
      toast.success(
        r.created > 0
          ? `${r.created} periode dibuat`
          : "Periode bulan ini dan bulan depan sudah ada"
      )
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "fiscal-periods"],
      })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Periode Akuntansi
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Periode dibuat otomatis setiap bulan. Transaksi di periode yang
            sudah ditutup ditolak. Tutup buku dan buka kembali periode tersedia
            di Tutup Buku.
          </p>
        </div>
        {can("accounting.fiscal_period.write") ? (
          <LoadingButton
            loading={ensure.isPending}
            loadingText="Membuat…"
            onClick={() => ensure.mutate()}
          >
            <PlusIcon /> Buat Periode
          </LoadingButton>
        ) : null}
      </div>
      <LoadingArea loading={periods.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Periode</TableHead>
                <TableHead>Tanggal</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Ditutup</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(periods.data ?? []).map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium">{p.label}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {formatDate(p.startDate)} – {formatDate(p.endDate)}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={p.status} />
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {p.closedAt ? formatDateTime(p.closedAt) : "—"}
                  </TableCell>
                </TableRow>
              ))}
              {periods.data?.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={4}
                    className="text-center text-muted-foreground"
                  >
                    Belum ada periode.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
    </div>
  )
}
