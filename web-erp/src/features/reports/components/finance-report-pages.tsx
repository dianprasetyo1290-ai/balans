import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { DateInput } from "@/components/common/date-input"
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
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { cn } from "@/lib/utils"

import {
  dashboardQuery,
  dimensionQuery,
  salesReportQuery,
  statementQuery,
  taxQuery,
  type DimensionKind,
  type ProfitLossSummary,
  type StatementKind,
  type StatementRow,
} from "../api/reports-api"
import {
  changePercent,
  DIMENSION_LABELS,
  monthLabel,
  resolvePeriod,
  RM,
  toNumber,
  type DimensionSearch,
  type PeriodSearch,
  type SearchUpdater,
  type StatementSearch,
  type TaxSearch,
} from "../lib/reports-lib"
import { BarList, EmptyRow, PeriodFilter, ReportHeader } from "./report-layout"

const STATEMENT_TABS: { id: StatementKind; label: string }[] = [
  { id: "balance_sheet", label: "Neraca" },
  { id: "income_statement", label: "Laba Rugi" },
  { id: "cash_flow", label: "Arus Kas" },
]

function StatementTable({ rows }: { rows: StatementRow[] }) {
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[36rem]">
        <TableHeader>
          <TableRow>
            <TableHead className="w-24">Kode</TableHead>
            <TableHead>Uraian</TableHead>
            <TableHead className="text-right">Nilai</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((r, i) => {
            const heading = r.kind === "section" || r.kind === "group"
            return (
              <TableRow
                key={`${i}-${r.label}`}
                className={cn(
                  r.kind === "section" && "bg-muted/50",
                  (r.kind === "total" || r.kind === "subtotal") &&
                    "font-medium",
                  r.kind === "total" && "border-t-2"
                )}
              >
                <TableCell className="text-muted-foreground tabular-nums">
                  {r.kind === "account" ? (r.code ?? "") : ""}
                </TableCell>
                <TableCell
                  className={cn(
                    r.kind === "section" && "font-semibold",
                    r.level === 1 && "pl-6",
                    r.level === 2 && "pl-10"
                  )}
                >
                  {r.label}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {heading ? "" : formatRupiah(r.amount)}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}

/** Keuangan & Akuntansi › Laporan Keuangan: Neraca, Laba Rugi, Arus Kas (S-409). */
export function FinancialStatementsPage({
  search,
  onSearchChange,
}: {
  search: StatementSearch
  onSearchChange: SearchUpdater<StatementSearch>
}) {
  const kind = search.tab ?? "balance_sheet"
  const period = resolvePeriod(search)
  const q = useQuery(statementQuery(kind, period))
  const data = q.data
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Keuangan"
        description="Neraca per tanggal posisi, Laba Rugi dan Arus Kas (metode tidak langsung) per periode dari jurnal yang sudah diposting, dikelompokkan menurut Chart of Accounts."
        exportKind="financial_statement"
        exportPermission="accounting.report.read"
        exportParams={{ kind, ...period }}
        readModels={[...RM.gl]}
        actions={
          <Link
            to="/finance/reports"
            search={{
              year: Number(period.dateTo.slice(0, 4)),
              month: Number(period.dateTo.slice(5, 7)),
            }}
            className="self-center text-sm underline-offset-2 hover:underline"
          >
            Neraca Saldo
          </Link>
        }
      />
      <PageTabs
        label="Jenis laporan keuangan"
        tabs={STATEMENT_TABS}
        value={kind}
        onValueChange={(tab) => onSearchChange((s) => ({ ...s, tab }))}
      />
      {kind === "balance_sheet" ? (
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">Posisi per</span>
          <div className="w-44">
            <DateInput
              value={period.dateTo}
              onValueChange={(v) =>
                onSearchChange((s) => ({ ...s, dateTo: v || undefined }))
              }
            />
          </div>
        </div>
      ) : (
        <PeriodFilter
          value={period}
          onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
        />
      )}
      <LoadingArea loading={q.isPending}>
        {data ? (
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-medium">
                {data.title}{" "}
                {kind === "balance_sheet"
                  ? `per ${period.dateTo}`
                  : `${data.dateFrom} s.d. ${data.dateTo}`}
              </span>
              {kind !== "income_statement" ? (
                data.balanced ? (
                  <Badge variant="secondary">Seimbang</Badge>
                ) : (
                  <Badge variant="destructive">
                    Tidak seimbang — periksa jurnal
                  </Badge>
                )
              ) : null}
            </div>
            <StatementTable rows={data.rows} />
          </div>
        ) : null}
      </LoadingArea>
    </div>
  )
}

const PL_LINES: {
  key: keyof ProfitLossSummary
  label: string
  strong?: boolean
}[] = [
  { key: "revenue", label: "Pendapatan Bersih" },
  { key: "cogs", label: "HPP" },
  { key: "grossProfit", label: "Laba Kotor", strong: true },
  { key: "grossMarginPercent", label: "Margin Kotor %" },
  { key: "operatingExpense", label: "Beban Operasional" },
  { key: "operatingProfit", label: "Laba Operasional", strong: true },
  { key: "otherNet", label: "Pendapatan & Beban Lain-lain" },
  { key: "netProfit", label: "Laba (Rugi) Bersih", strong: true },
]

function plValue(s: ProfitLossSummary, key: keyof ProfitLossSummary) {
  return key.endsWith("Percent") ? `${s[key]}%` : formatRupiah(s[key])
}

/** Laporan › Laporan Laba Rugi per Dimensi: per channel/sub-channel/divisi/kampanye dan margin produk (S-417). */
export function DimensionProfitLossPage({
  search,
  onSearchChange,
}: {
  search: DimensionSearch
  onSearchChange: SearchUpdater<DimensionSearch>
}) {
  const tab = search.tab ?? "dimension"
  const dimension: DimensionKind = search.dimension ?? "channel"
  const groupBy = search.groupBy ?? "product"
  const period = resolvePeriod(search)
  const dim = useQuery({
    ...dimensionQuery(dimension, period),
    enabled: tab === "dimension",
  })
  const margin = useQuery({
    ...salesReportQuery(groupBy, period),
    enabled: tab === "margin",
  })
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Laba Rugi per Dimensi"
        description="Laba rugi per channel, sub-channel, divisi, atau kampanye dari dimensi jurnal (biaya bersama tampil di kolom tanpa dimensi), dan margin kotor per produk/kategori dari penjualan & HPP."
        exportKind={tab === "dimension" ? "profit_loss_dimension" : "sales"}
        exportPermission={
          tab === "dimension"
            ? "accounting.report.read"
            : "sales.sales_order.read"
        }
        exportParams={
          tab === "dimension"
            ? { dimension, ...period }
            : { groupBy, ...period }
        }
        readModels={tab === "dimension" ? [...RM.gl] : [...RM.sales]}
      />
      <PageTabs
        label="Jenis laporan"
        tabs={[
          { id: "dimension", label: "Laba Rugi per Dimensi" },
          { id: "margin", label: "Margin per Produk/Kategori" },
        ]}
        value={tab}
        onValueChange={(t) => onSearchChange((s) => ({ ...s, tab: t }))}
      />
      <div className="flex flex-wrap items-center gap-3">
        <PeriodFilter
          value={period}
          onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
        />
        {tab === "dimension" ? (
          <Select
            value={dimension}
            onValueChange={(v) =>
              onSearchChange((s) => ({
                ...s,
                dimension: (v ?? "channel") as DimensionKind,
              }))
            }
          >
            <SelectTrigger className="w-44" aria-label="Dimensi">
              <SelectValue>
                {(v: string) => DIMENSION_LABELS[v] ?? v}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {Object.entries(DIMENSION_LABELS).map(([k, label]) => (
                <SelectItem key={k} value={k}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          <Select
            value={groupBy}
            onValueChange={(v) =>
              onSearchChange((s) => ({
                ...s,
                groupBy: (v ?? "product") as "product" | "category",
              }))
            }
          >
            <SelectTrigger className="w-44" aria-label="Kelompokkan per">
              <SelectValue>
                {(v: string) =>
                  v === "category" ? "Kategori" : "Produk (SKU)"
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="product">Produk (SKU)</SelectItem>
              <SelectItem value="category">Kategori</SelectItem>
            </SelectContent>
          </Select>
        )}
      </div>
      {tab === "dimension" ? (
        <LoadingArea loading={dim.isPending}>
          {dim.data ? (
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[40rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Uraian</TableHead>
                    {dim.data.columns.map((c) => (
                      <TableHead key={c.id ?? "none"} className="text-right">
                        {c.name}
                      </TableHead>
                    ))}
                    <TableHead className="text-right">Total</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {dim.data.columns.length === 0 ? (
                    <EmptyRow colSpan={2} />
                  ) : null}
                  {dim.data.columns.length > 0
                    ? PL_LINES.map((line) => (
                        <TableRow
                          key={line.key}
                          className={cn(line.strong && "font-medium")}
                        >
                          <TableCell>{line.label}</TableCell>
                          {dim.data.columns.map((c) => (
                            <TableCell
                              key={c.id ?? "none"}
                              className="text-right tabular-nums"
                            >
                              {plValue(c.summary, line.key)}
                            </TableCell>
                          ))}
                          <TableCell className="text-right tabular-nums">
                            {plValue(dim.data.total, line.key)}
                          </TableCell>
                        </TableRow>
                      ))
                    : null}
                  {dim.data.accounts.length > 0 ? (
                    <TableRow className="bg-muted/50">
                      <TableCell
                        colSpan={dim.data.columns.length + 2}
                        className="font-semibold"
                      >
                        Rincian per akun
                      </TableCell>
                    </TableRow>
                  ) : null}
                  {dim.data.accounts.map((a) => (
                    <TableRow key={a.accountCode}>
                      <TableCell>
                        <span className="text-muted-foreground tabular-nums">
                          {a.accountCode}
                        </span>{" "}
                        {a.accountName}
                      </TableCell>
                      {a.amounts.map((v, i) => (
                        <TableCell key={i} className="text-right tabular-nums">
                          {formatRupiah(v)}
                        </TableCell>
                      ))}
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(a.total)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : null}
        </LoadingArea>
      ) : (
        <LoadingArea loading={margin.isPending}>
          {margin.data ? (
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[48rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>
                      {groupBy === "category" ? "Kategori" : "Produk"}
                    </TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Penjualan Neto</TableHead>
                    <TableHead className="text-right">HPP</TableHead>
                    <TableHead className="text-right">Laba Kotor</TableHead>
                    <TableHead className="text-right">Margin</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {margin.data.lines.length === 0 ? (
                    <EmptyRow colSpan={6} />
                  ) : null}
                  {margin.data.lines.map((l) => (
                    <TableRow key={l.key}>
                      <TableCell>
                        {l.code ? (
                          <span className="text-muted-foreground tabular-nums">
                            {l.code}{" "}
                          </span>
                        ) : null}
                        {l.name}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {toNumber(l.qty).toLocaleString("id-ID")}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.net)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.cogs)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.grossProfit)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {l.marginPercent}%
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell className="font-medium">Total</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {toNumber(margin.data.total.qty).toLocaleString("id-ID")}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(margin.data.total.net)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(margin.data.total.cogs)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(margin.data.total.grossProfit)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {margin.data.total.marginPercent}%
                    </TableCell>
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
          ) : null}
        </LoadingArea>
      )}
    </div>
  )
}

const MONTHS = [
  "Januari",
  "Februari",
  "Maret",
  "April",
  "Mei",
  "Juni",
  "Juli",
  "Agustus",
  "September",
  "Oktober",
  "November",
  "Desember",
]

/** Laporan › Laporan Pajak: rekap PPh dipotong, PPh Final, pemantauan omzet terhadap Rp4,8 M (RPT-18). */
export function TaxReportPage({
  search,
  onSearchChange,
}: {
  search: TaxSearch
  onSearchChange: SearchUpdater<TaxSearch>
}) {
  const year = search.year ?? Number(todayBusinessDate().slice(0, 4))
  const q = useQuery(taxQuery(year, search.month))
  const data = q.data
  const years = Array.from(
    { length: 5 },
    (_, i) => Number(todayBusinessDate().slice(0, 4)) - i
  )
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Pajak"
        description="Rekap PPh 21/23/4(2) yang dipotong dan disetor per masa, PPh Final, serta omzet bruto kumulatif tahun berjalan terhadap batas pengusaha kecil Rp4,8 miliar (Balans NON-PKP). Rekap bukti potong per pemasok untuk Coretax ada di Pajak & Bukti Potong."
        exportKind="tax"
        exportPermission="accounting.report.read"
        exportParams={{ year: String(year) }}
        readModels={[...RM.gl]}
      />
      <div className="flex flex-wrap items-center gap-3">
        <Select
          value={String(year)}
          onValueChange={(v) =>
            onSearchChange((s) => ({ ...s, year: Number(v), month: undefined }))
          }
        >
          <SelectTrigger className="w-32" aria-label="Tahun">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {years.map((y) => (
              <SelectItem key={y} value={String(y)}>
                {y}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <LoadingArea loading={q.isPending}>
        {data ? (
          <div className="flex flex-col gap-4">
            <Card>
              <CardHeader>
                <CardTitle>Omzet bruto {data.year}</CardTitle>
              </CardHeader>
              <CardContent className="flex flex-col gap-2">
                <div className="flex flex-wrap items-baseline gap-2">
                  <span className="text-2xl font-semibold tabular-nums">
                    {formatRupiah(data.turnover)}
                  </span>
                  <span className="text-sm text-muted-foreground">
                    dari batas {formatRupiah(data.limit)} ({data.usedPercent}%)
                  </span>
                  {data.limitExceeded ? (
                    <Badge variant="destructive">Melewati batas</Badge>
                  ) : data.warning ? (
                    <Badge variant="destructive">Mendekati batas</Badge>
                  ) : null}
                </div>
                <div
                  className="h-3 w-full overflow-hidden rounded bg-muted"
                  role="progressbar"
                  aria-label="Omzet terhadap batas pengusaha kecil"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.min(100, toNumber(data.usedPercent))}
                >
                  <div
                    className={cn(
                      "h-full",
                      data.warning ? "bg-destructive" : "bg-primary"
                    )}
                    style={{
                      width: `${Math.min(100, toNumber(data.usedPercent))}%`,
                    }}
                  />
                </div>
              </CardContent>
            </Card>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[60rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Masa</TableHead>
                    <TableHead className="text-right">PPh 21</TableHead>
                    <TableHead className="text-right">PPh 23</TableHead>
                    <TableHead className="text-right">PPh 4(2)</TableHead>
                    <TableHead className="text-right">PPh Disetor</TableHead>
                    <TableHead className="text-right">PPh Final</TableHead>
                    <TableHead className="text-right">Omzet Bruto</TableHead>
                    <TableHead className="text-right">Kumulatif</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.months.map((m, i) => (
                    <TableRow
                      key={m.month}
                      className={cn(
                        "cursor-pointer",
                        search.month === i + 1 && "bg-muted/50"
                      )}
                      onClick={() =>
                        onSearchChange((s) => ({ ...s, month: i + 1 }))
                      }
                    >
                      <TableCell>
                        <button
                          type="button"
                          className="min-h-11 text-left underline-offset-2 hover:underline sm:min-h-0"
                        >
                          {MONTHS[i]}
                        </button>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.pph21Withheld)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.pph23Withheld)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.pph42Withheld)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.withholdingPaid)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.finalTax)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.turnover)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(m.cumulativeTurnover)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {search.month ? (
              <div className="flex flex-col gap-2">
                <h2 className="text-lg font-semibold">
                  Rincian jurnal PPh {MONTHS[search.month - 1]} {data.year}
                </h2>
                <div className="overflow-x-auto rounded-lg border">
                  <Table className="min-w-[48rem]">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Tanggal</TableHead>
                        <TableHead>Nomor Jurnal</TableHead>
                        <TableHead>Dokumen</TableHead>
                        <TableHead>Akun</TableHead>
                        <TableHead className="text-right">Dipotong</TableHead>
                        <TableHead className="text-right">Disetor</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {data.lines.length === 0 ? (
                        <EmptyRow colSpan={6}>
                          Tidak ada jurnal PPh pada masa ini.
                        </EmptyRow>
                      ) : null}
                      {data.lines.map((l, i) => (
                        <TableRow key={`${l.entryId}-${i}`}>
                          <TableCell>{l.businessDate}</TableCell>
                          <TableCell>{l.number}</TableCell>
                          <TableCell>
                            {l.sourceLabel || l.description}
                          </TableCell>
                          <TableCell>
                            {l.accountCode} {l.accountName}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatRupiah(l.credit)}
                          </TableCell>
                          <TableCell className="text-right tabular-nums">
                            {formatRupiah(l.debit)}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </div>
            ) : null}
          </div>
        ) : null}
      </LoadingArea>
    </div>
  )
}

function Kpi({
  title,
  value,
  hint,
}: {
  title: string
  value: string
  hint?: string
}) {
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="text-sm font-normal text-muted-foreground">
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-xl font-semibold tabular-nums sm:text-2xl">
          {value}
        </div>
        {hint ? (
          <div className="text-xs text-muted-foreground">{hint}</div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function changeHint(current: string, previous: string) {
  const c = changePercent(current, previous)
  if (c === null) {
    return "Periode sebelumnya: -"
  }
  return `${c >= 0 ? "▲" : "▼"} ${Math.abs(c).toFixed(1)}% dibanding periode sebelumnya`
}

/** Ringkasan › Dashboard Eksekutif: penjualan, margin, stok, kas (S-418, RPT-14). */
export function ExecutiveDashboardPage({
  search,
  onSearchChange,
}: {
  search: PeriodSearch
  onSearchChange: SearchUpdater<PeriodSearch>
}) {
  const period = resolvePeriod(search)
  const q = useQuery(dashboardQuery(period))
  const d = q.data
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Dashboard Eksekutif"
        description="Penjualan, margin, posisi stok, kas, piutang, dan utang dari buku besar (materialized view yang diperbarui job tiap 15 menit)."
        readModels={[...RM.gl]}
      />
      <PeriodFilter
        value={period}
        onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
      />
      <LoadingArea loading={q.isPending}>
        {d ? (
          <div className="flex flex-col gap-4">
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <Kpi
                title="Penjualan Bersih"
                value={formatRupiah(d.period.revenue)}
                hint={changeHint(d.period.revenue, d.previous.revenue)}
              />
              <Kpi
                title="Laba Kotor"
                value={formatRupiah(d.period.grossProfit)}
                hint={`Margin ${d.period.grossMarginPercent}% · ${changeHint(d.period.grossProfit, d.previous.grossProfit)}`}
              />
              <Kpi
                title="Laba Bersih"
                value={formatRupiah(d.period.netProfit)}
                hint={`Margin bersih ${d.period.netMarginPercent}%`}
              />
              <Kpi
                title="Kas & Bank"
                value={formatRupiah(d.cash)}
                hint={`Per ${d.dateTo}`}
              />
              <Kpi
                title="Nilai Persediaan"
                value={formatRupiah(d.inventory)}
                hint="Saldo akun persediaan 13xx"
              />
              <Kpi
                title="Piutang"
                value={formatRupiah(d.receivables)}
                hint="Piutang usaha, marketplace, karyawan"
              />
              <Kpi
                title="Utang Usaha"
                value={formatRupiah(d.payables)}
                hint="Termasuk Barang Diterima Belum Ditagih"
              />
              <Kpi
                title="Beban Operasional"
                value={formatRupiah(d.period.operatingExpense)}
              />
            </div>
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>Tren penjualan & laba kotor 12 bulan</CardTitle>
                </CardHeader>
                <CardContent>
                  <TrendChart points={d.trend} />
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>Penjualan per channel</CardTitle>
                </CardHeader>
                <CardContent>
                  {d.channels.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      Belum ada penjualan pada periode ini.
                    </p>
                  ) : (
                    <BarList
                      label="Penjualan bersih per channel"
                      items={d.channels.map((c) => ({
                        key: c.id ?? "none",
                        label: c.name,
                        value: toNumber(c.revenue),
                        display: `${formatRupiah(c.revenue)} · margin ${c.marginPercent}%`,
                      }))}
                    />
                  )}
                </CardContent>
              </Card>
            </div>
          </div>
        ) : null}
      </LoadingArea>
    </div>
  )
}

/** Grafik batang SVG tren bulanan; nilai lengkap juga ada di tabel tersembunyi untuk pembaca layar. */
function TrendChart({
  points,
}: {
  points: { month: string; revenue: string; grossProfit: string }[]
}) {
  const max = Math.max(
    1,
    ...points.map((p) => Math.max(toNumber(p.revenue), toNumber(p.grossProfit)))
  )
  const w = 24
  const gap = 8
  const h = 120
  return (
    <div>
      <svg
        viewBox={`0 0 ${points.length * (w + gap)} ${h + 20}`}
        className="h-48 w-full"
        role="img"
        aria-label="Tren penjualan bersih dan laba kotor per bulan"
      >
        {points.map((p, i) => {
          const rev = (Math.max(0, toNumber(p.revenue)) / max) * h
          const gp = (Math.max(0, toNumber(p.grossProfit)) / max) * h
          const x = i * (w + gap)
          return (
            <g key={p.month}>
              <rect
                x={x}
                y={h - rev}
                width={w / 2}
                height={rev}
                className="fill-primary/70"
              />
              <rect
                x={x + w / 2}
                y={h - gp}
                width={w / 2}
                height={gp}
                className="fill-chart-2"
              />
              <text
                x={x + w / 2}
                y={h + 14}
                textAnchor="middle"
                className="fill-muted-foreground text-[8px]"
              >
                {monthLabel(p.month)}
              </text>
            </g>
          )
        })}
      </svg>
      <div className="flex gap-4 text-xs text-muted-foreground">
        <span className="flex items-center gap-1">
          <span
            aria-hidden="true"
            className="inline-block size-3 rounded bg-primary/70"
          />{" "}
          Penjualan bersih
        </span>
        <span className="flex items-center gap-1">
          <span
            aria-hidden="true"
            className="inline-block size-3 rounded bg-chart-2"
          />{" "}
          Laba kotor
        </span>
      </div>
      <table className="sr-only">
        <caption>Tren bulanan</caption>
        <tbody>
          {points.map((p) => (
            <tr key={p.month}>
              <th scope="row">{monthLabel(p.month)}</th>
              <td>{formatRupiah(p.revenue)}</td>
              <td>{formatRupiah(p.grossProfit)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
