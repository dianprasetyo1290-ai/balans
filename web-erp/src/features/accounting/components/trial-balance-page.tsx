import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { DownloadIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { Badge } from "@/components/ui/badge"
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePermissions } from "@/features/auth/api/auth-api"
import { createExportJob } from "@/features/dataexchange/api/dataexchange-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { channelsQuery, divisionsQuery } from "../api/accounting-api"
import {
  periodLabel,
  periodOptions,
  periodRange,
  trialBalanceQuery,
} from "../api/close-api"
import { DimensionSelect } from "./dimension-select"

const route = getRouteApi("/_app/finance/reports")

const money = (v: string) => (Number(v) ? formatRupiah(v) : "")

/**
 * Keuangan & Akuntansi › Laporan Keuangan › Neraca Saldo (ACC-18, S-410): saldo awal, mutasi, dan saldo akhir per
 * akun untuk satu periode dan dimensi, dengan drill-down ke Buku Besar → jurnal → dokumen sumber.
 */
export function TrialBalancePage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const channels = useQuery(channelsQuery(true))
  const divisions = useQuery(divisionsQuery(true))
  const [dateFrom, dateTo] = periodRange(search.year, search.month)
  const tb = useQuery(
    trialBalanceQuery({
      dateFrom,
      dateTo,
      divisionId: search.divisionId,
      channelId: search.channelId,
    })
  )
  const periods = periodOptions(todayBusinessDate())
  const setSearch = (patch: Partial<typeof search>) =>
    void navigate({ search: (s) => ({ ...s, ...patch }) })

  const exportJob = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const params: Record<string, string> = {
        year: String(search.year),
        month: String(search.month),
      }
      if (search.divisionId) params.divisionId = search.divisionId
      if (search.channelId) params.channelId = search.channelId
      return createExportJob("accounting.trial_balance", key, params)
    },
    onSuccess: () =>
      toast.success(
        "Ekspor Neraca Saldo diproses. Tautan unduhan dikirim lewat notifikasi."
      ),
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const data = tb.data
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Laporan Keuangan
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Dari jurnal yang sudah diposting. Klik akun untuk membuka Buku Besar
            dengan periode dan dimensi yang sama.
          </p>
        </div>
        {can("dataexchange.transaction.export") ? (
          <LoadingButton
            variant="outline"
            loading={exportJob.isPending}
            loadingText="Memproses…"
            onClick={() => exportJob.mutate()}
          >
            <DownloadIcon /> Ekspor Neraca Saldo
          </LoadingButton>
        ) : null}
      </div>
      <PageTabs
        label="Laporan keuangan"
        tabs={[{ id: "trial-balance", label: "Neraca Saldo" }]}
        value="trial-balance"
        onValueChange={() => undefined}
      />
      <div className="grid gap-3 sm:grid-cols-3">
        <Field>
          <FieldLabel htmlFor="tb-period">Periode</FieldLabel>
          <Select
            value={`${search.year}-${search.month}`}
            onValueChange={(v) => {
              const [y, m] = String(v).split("-").map(Number)
              setSearch({ year: y, month: m })
            }}
          >
            <SelectTrigger id="tb-period" className="w-full">
              <SelectValue>
                {() => periodLabel(search.year, search.month)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {periods.map((p) => (
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
        <Field>
          <FieldLabel htmlFor="tb-division">Divisi</FieldLabel>
          <DimensionSelect
            id="tb-division"
            value={search.divisionId}
            options={divisions.data ?? []}
            placeholder="Semua divisi"
            onValueChange={(v) => setSearch({ divisionId: v ?? undefined })}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="tb-channel">Channel</FieldLabel>
          <DimensionSelect
            id="tb-channel"
            value={search.channelId}
            options={channels.data ?? []}
            placeholder="Semua channel"
            onValueChange={(v) => setSearch({ channelId: v ?? undefined })}
          />
        </Field>
      </div>
      <div className="flex items-center gap-2 text-sm">
        <span className="font-medium">
          Neraca Saldo {periodLabel(search.year, search.month)}
        </span>
        {data ? (
          data.balanced ? (
            <Badge variant="outline">Balance</Badge>
          ) : (
            <Badge variant="destructive">Tidak balance</Badge>
          )
        ) : null}
      </div>
      <LoadingArea loading={tb.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead rowSpan={2}>Akun</TableHead>
                <TableHead colSpan={2} className="text-center">
                  Saldo Awal
                </TableHead>
                <TableHead colSpan={2} className="text-center">
                  Mutasi
                </TableHead>
                <TableHead colSpan={2} className="text-center">
                  Saldo Akhir
                </TableHead>
              </TableRow>
              <TableRow>
                {["Debit", "Kredit", "Debit", "Kredit", "Debit", "Kredit"].map(
                  (h, i) => (
                    <TableHead key={i} className="text-right">
                      {h}
                    </TableHead>
                  )
                )}
              </TableRow>
            </TableHeader>
            <TableBody>
              {(data?.rows ?? []).map((r) => (
                <TableRow key={r.accountId}>
                  <TableCell>
                    <Link
                      to="/finance/general-ledger"
                      search={{
                        accountId: r.accountId,
                        dateFrom,
                        dateTo,
                        divisionId: search.divisionId,
                        channelId: search.channelId,
                        page: 1,
                      }}
                      className="text-primary underline-offset-4 hover:underline"
                    >
                      <span className="font-medium tabular-nums">
                        {r.accountCode}
                      </span>{" "}
                      {r.accountName}
                    </Link>
                  </TableCell>
                  {[
                    r.openingDebit,
                    r.openingCredit,
                    r.periodDebit,
                    r.periodCredit,
                    r.closingDebit,
                    r.closingCredit,
                  ].map((v, i) => (
                    <TableCell
                      key={i}
                      className="text-right whitespace-nowrap tabular-nums"
                    >
                      {money(v)}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
              {data && data.rows.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className="text-center text-muted-foreground"
                  >
                    Belum ada jurnal terposting sampai periode ini.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
            {data ? (
              <TableFooter>
                <TableRow>
                  <TableCell className="font-semibold">Total</TableCell>
                  {[
                    data.totals.openingDebit,
                    data.totals.openingCredit,
                    data.totals.periodDebit,
                    data.totals.periodCredit,
                    data.totals.closingDebit,
                    data.totals.closingCredit,
                  ].map((v, i) => (
                    <TableCell
                      key={i}
                      className="text-right font-semibold whitespace-nowrap tabular-nums"
                    >
                      {formatRupiah(v)}
                    </TableCell>
                  ))}
                </TableRow>
              </TableFooter>
            ) : null}
          </Table>
        </div>
      </LoadingArea>
    </div>
  )
}
