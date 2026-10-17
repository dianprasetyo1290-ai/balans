import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { PlayIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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
import { formatDateTime, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  depreciationRunQuery,
  depreciationRunsQuery,
  periodLabel,
  periodOptions,
  previousPeriod,
  runDepreciation,
  type DepreciationRun,
} from "../api/close-api"

const route = getRouteApi("/_app/finance/depreciation")

/**
 * Keuangan & Akuntansi › Penyusutan: hasil job `accounting.depreciate` per periode (A-31, S-408) dan jalankan
 * penyusutan manual untuk periode terbuka. Idempotent: aset yang sudah disusutkan pada periode dilewati.
 */
export function DepreciationPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const runs = useQuery(depreciationRunsQuery(search.page))
  const [running, setRunning] = useState(false)

  const columns: DataTableColumn<DepreciationRun>[] = [
    {
      id: "period",
      header: "Periode",
      cell: (r) => (
        <Link
          to="/finance/depreciation"
          search={{ page: search.page, run: r.id }}
          className="font-medium text-primary underline-offset-4 hover:underline"
        >
          {r.periodLabel}
        </Link>
      ),
    },
    {
      id: "assets",
      header: "Jumlah Aset",
      align: "right",
      cell: (r) => r.assetCount,
    },
    {
      id: "total",
      header: "Total Penyusutan",
      align: "right",
      cell: (r) => formatRupiah(r.total),
    },
    {
      id: "last",
      header: "Terakhir Dijalankan",
      cell: (r) => (
        <span className="whitespace-nowrap">
          {formatDateTime(r.lastRunAt)} ({r.runCount}×)
        </span>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Penyusutan</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Penyusutan garis lurus dijalankan otomatis setiap tanggal 1 pukul
            01.00 WIB untuk bulan sebelumnya (Dr 6305, Kr akumulasi kelas aset).
            Menjalankan ulang aman: aset yang sudah disusutkan dilewati.
          </p>
        </div>
        {can("accounting.fixed_asset.post") ? (
          <Button onClick={() => setRunning(true)}>
            <PlayIcon /> Jalankan Penyusutan
          </Button>
        ) : null}
      </div>
      <DataTable
        columns={columns}
        rows={runs.data?.items}
        getRowId={(r) => r.id}
        loading={runs.isPending}
        fetching={runs.isFetching}
        page={search.page}
        totalPages={runs.data?.totalPages ?? 0}
        totalItems={runs.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Penyusutan belum pernah dijalankan."
      />
      {search.run ? <RunDetail id={search.run} /> : null}
      <RunDialog
        open={running}
        onOpenChange={setRunning}
        onDone={(id) => void navigate({ search: (s) => ({ ...s, run: id }) })}
      />
    </div>
  )
}

function RunDetail({ id }: { id: string }) {
  const run = useQuery(depreciationRunQuery(id))
  return (
    <LoadingArea loading={run.isPending}>
      {run.data ? (
        <section className="flex flex-col gap-2">
          <h2 className="text-lg font-semibold">
            Penyusutan {run.data.run.periodLabel}
          </h2>
          <div className="overflow-x-auto rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Aset</TableHead>
                  <TableHead className="text-right">Penyusutan</TableHead>
                  <TableHead className="text-right">Akumulasi</TableHead>
                  <TableHead className="text-right">Nilai Buku</TableHead>
                  <TableHead>Jurnal</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {run.data.lines.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell>
                      <Link
                        to="/finance/fixed-assets/$id"
                        params={{ id: l.assetId }}
                        search={{ tab: "detail" }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {l.assetCode}
                      </Link>{" "}
                      {l.assetName}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.amount)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.accumulated)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.bookValue)}
                    </TableCell>
                    <TableCell>
                      <Link
                        to="/finance/journals/$id"
                        params={{ id: l.journalEntryId }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {l.journalNumber}
                      </Link>
                    </TableCell>
                  </TableRow>
                ))}
                {run.data.lines.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className="text-center text-muted-foreground"
                    >
                      Tidak ada aset yang disusutkan pada periode ini.
                    </TableCell>
                  </TableRow>
                ) : null}
              </TableBody>
            </Table>
          </div>
        </section>
      ) : null}
    </LoadingArea>
  )
}

type RunDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onDone: (runId: string) => void
}

function RunDialog(props: RunDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <RunForm {...props} /> : null}
    </Dialog>
  )
}

function RunForm({ onOpenChange, onDone }: RunDialogProps) {
  const queryClient = useQueryClient()
  const today = todayBusinessDate()
  const [period, setPeriod] = useState(previousPeriod(today))
  const options = periodOptions(today, 12)
  const run = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      runDepreciation(period.year, period.month, key),
    onSuccess: async (d) => {
      toast.success(
        `Penyusutan ${d.run.periodLabel}: ${d.run.assetCount} aset, ${formatRupiah(d.run.total)}`
      )
      await queryClient.invalidateQueries({ queryKey: ["accounting"] })
      onOpenChange(false)
      onDone(d.run.id)
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Jalankan Penyusutan</DialogTitle>
        <DialogDescription>
          Periode harus masih terbuka. Aset aktif yang belum disusutkan pada
          periode ini dijurnal A-31 per aset.
        </DialogDescription>
      </DialogHeader>
      <Field>
        <FieldLabel htmlFor="dep-period">Periode</FieldLabel>
        <Select
          value={`${period.year}-${period.month}`}
          onValueChange={(v) => {
            const [year, month] = String(v).split("-").map(Number)
            setPeriod({ year, month })
          }}
        >
          <SelectTrigger id="dep-period" className="w-full">
            <SelectValue>
              {() => periodLabel(period.year, period.month)}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {options.map((p) => (
              <SelectItem
                key={`${p.year}-${p.month}`}
                value={`${p.year}-${p.month}`}
              >
                {periodLabel(p.year, p.month)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Batal
        </Button>
        <LoadingButton
          loading={run.isPending}
          loadingText="Memproses…"
          onClick={() => run.mutate()}
        >
          Jalankan Penyusutan
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
