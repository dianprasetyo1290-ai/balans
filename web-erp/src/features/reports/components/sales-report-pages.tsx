import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { Badge } from "@/components/ui/badge"
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
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import {
  consignmentReportQuery,
  internalUsageReportQuery,
  outstandingPOQuery,
  payableAgingQuery,
  returnReportQuery,
  salesReportQuery,
  type ReportGroupBy,
} from "../api/reports-api"
import {
  DISPOSITION_LABELS,
  GROUP_BY_LABELS,
  resolvePeriod,
  RM,
  toNumber,
  USAGE_PURPOSE_LABELS,
  type PayablesSearch,
  type PeriodSearch,
  type SalesSearch,
  type SearchUpdater,
} from "../lib/reports-lib"
import { BarList, EmptyRow, PeriodFilter, ReportHeader } from "./report-layout"

const qtyText = (v: string) =>
  toNumber(v).toLocaleString("id-ID", { maximumFractionDigits: 3 })

/** Laporan › Laporan Penjualan per channel, SKU, periode (RPT-05). */
export function SalesReportPage({
  search,
  onSearchChange,
}: {
  search: SalesSearch
  onSearchChange: SearchUpdater<SalesSearch>
}) {
  const groupBy: ReportGroupBy = search.groupBy ?? "channel"
  const period = resolvePeriod(search)
  const q = useQuery(salesReportQuery(groupBy, period))
  const data = q.data
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Penjualan"
        description="Penjualan diakui (invoice; marketplace saat serah ke kurir) per channel, sub-channel, SKU, kategori, atau periode: qty, bruto, diskon, neto, HPP, dan margin. Neto sama dengan jurnal Penjualan − Potongan Penjualan."
        exportKind="sales"
        exportPermission="sales.sales_order.read"
        exportParams={{ groupBy, ...period }}
        readModels={[...RM.sales]}
      />
      <div className="flex flex-wrap items-center gap-3">
        <PeriodFilter
          value={period}
          onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
        />
        <Select
          value={groupBy}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              groupBy: (v ?? "channel") as ReportGroupBy,
            }))
          }
        >
          <SelectTrigger className="w-44" aria-label="Kelompokkan per">
            <SelectValue>
              {(v: string) => `Per ${GROUP_BY_LABELS[v] ?? v}`}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {Object.entries(GROUP_BY_LABELS).map(([k, label]) => (
              <SelectItem key={k} value={k}>
                Per {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <LoadingArea loading={q.isPending}>
        {data ? (
          <div className="flex flex-col gap-4">
            {data.lines.length > 0 && groupBy !== "day" ? (
              <BarList
                label="Penjualan neto"
                items={data.lines.slice(0, 10).map((l) => ({
                  key: l.key,
                  label: l.name,
                  value: toNumber(l.net),
                  display: formatRupiah(l.net),
                }))}
              />
            ) : null}
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[60rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>{GROUP_BY_LABELS[groupBy]}</TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Bruto</TableHead>
                    <TableHead className="text-right">Diskon</TableHead>
                    <TableHead className="text-right">Neto</TableHead>
                    <TableHead className="text-right">HPP</TableHead>
                    <TableHead className="text-right">Laba Kotor</TableHead>
                    <TableHead className="text-right">Margin</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.lines.length === 0 ? <EmptyRow colSpan={8} /> : null}
                  {data.lines.map((l) => (
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
                        {qtyText(l.qty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.gross)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(l.discount)}
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
                      {qtyText(data.total.qty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(data.total.gross)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(data.total.discount)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(data.total.net)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(data.total.cogs)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(data.total.grossProfit)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {data.total.marginPercent}%
                    </TableCell>
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
          </div>
        ) : null}
      </LoadingArea>
    </div>
  )
}

/** Laporan › Laporan Konsinyasi: rekonsiliasi per mitra per bulan (RPT-06). */
export function ConsignmentReportPage({
  search,
  onSearchChange,
}: {
  search: PeriodSearch
  onSearchChange: SearchUpdater<PeriodSearch>
}) {
  const period = resolvePeriod(search)
  const q = useQuery(consignmentReportQuery(period))
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Konsinyasi"
        description="Penjualan konsinyasi (invoice hasil rekonsiliasi) dan selisih yang ditagihkan per mitra. Stok di lokasi mitra ada di Laporan Stok dengan jenis lokasi Konsinyasi."
        actions={
          <Link
            to="/reports/stock"
            search={{ locationType: "consignment" }}
            className="self-center text-sm underline-offset-2 hover:underline"
          >
            Stok di mitra konsinyasi
          </Link>
        }
      />
      <PeriodFilter
        value={period}
        onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
      />
      <LoadingArea loading={q.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[48rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Mitra</TableHead>
                <TableHead>Jenis</TableHead>
                <TableHead className="text-right">Invoice</TableHead>
                <TableHead className="text-right">Bruto</TableHead>
                <TableHead className="text-right">Total</TableHead>
                <TableHead className="text-right">Dibayar</TableHead>
                <TableHead className="text-right">Sisa</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(q.data?.items ?? []).length === 0 ? (
                <EmptyRow colSpan={7}>
                  Belum ada rekonsiliasi konsinyasi pada periode ini.
                </EmptyRow>
              ) : null}
              {(q.data?.items ?? []).map((r) => (
                <TableRow key={`${r.partnerId}-${r.invoiceType}`}>
                  <TableCell>
                    <span className="text-muted-foreground tabular-nums">
                      {r.partnerCode}
                    </span>{" "}
                    {r.partnerName}
                  </TableCell>
                  <TableCell>
                    {r.invoiceType === "variance" ? "Selisih" : "Penjualan"}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {r.invoiceCount}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.gross)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.total)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.paid)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.outstanding)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
    </div>
  )
}

/** Laporan › Laporan Retur: barang retur, alasan, dan tindak lanjut (RPT-09). */
export function ReturnReportPage({
  search,
  onSearchChange,
}: {
  search: PeriodSearch
  onSearchChange: SearchUpdater<PeriodSearch>
}) {
  const period = resolvePeriod(search)
  const q = useQuery(returnReportQuery(period))
  const items = q.data?.items ?? []
  const byDisposition = new Map<string, number>()
  for (const r of items) {
    byDisposition.set(
      r.disposition,
      (byDisposition.get(r.disposition) ?? 0) + toNumber(r.cost)
    )
  }
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Retur"
        description="Barang retur yang sudah didisposisi: alasan retur dan tindak lanjutnya (restock, repacking, reprocess, sampel, pemusnahan). Retur pembelian ada di Pengadaan › Retur Pembelian."
      />
      <PeriodFilter
        value={period}
        onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
      />
      <LoadingArea loading={q.isPending}>
        <div className="flex flex-col gap-4">
          {byDisposition.size > 0 ? (
            <BarList
              label="Nilai retur per tindak lanjut"
              items={[...byDisposition.entries()].map(([k, v]) => ({
                key: k,
                label: DISPOSITION_LABELS[k] ?? k,
                value: v,
                display: formatRupiah(String(v)),
              }))}
            />
          ) : null}
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[56rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Nomor</TableHead>
                  <TableHead>Tanggal</TableHead>
                  <TableHead>Produk</TableHead>
                  <TableHead>Alasan</TableHead>
                  <TableHead>Tindak Lanjut</TableHead>
                  <TableHead className="text-right">Qty</TableHead>
                  <TableHead className="text-right">Nilai</TableHead>
                  <TableHead>Catatan QC</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.length === 0 ? <EmptyRow colSpan={8} /> : null}
                {items.map((r, i) => (
                  <TableRow key={`${r.dispositionId}-${i}`}>
                    <TableCell>{r.number}</TableCell>
                    <TableCell>{formatDate(r.businessDate)}</TableCell>
                    <TableCell>
                      <span className="text-muted-foreground tabular-nums">
                        {r.product.sku}
                      </span>{" "}
                      {r.product.name}
                    </TableCell>
                    <TableCell>{r.reasonName || "-"}</TableCell>
                    <TableCell>
                      {DISPOSITION_LABELS[r.disposition] ?? r.disposition}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(r.qty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(r.cost)}
                    </TableCell>
                    <TableCell>{r.qcNote || "-"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      </LoadingArea>
    </div>
  )
}

/** Laporan › Laporan Pemakaian Internal per kampanye & sampel karyawan (RPT-08). */
export function InternalUsageReportPage({
  search,
  onSearchChange,
}: {
  search: PeriodSearch
  onSearchChange: SearchUpdater<PeriodSearch>
}) {
  const period = resolvePeriod(search)
  const q = useQuery(internalUsageReportQuery(period))
  const items = q.data?.items ?? []
  const byCampaign = new Map<string, number>()
  for (const r of items) {
    const key = r.campaignName || USAGE_PURPOSE_LABELS[r.purpose] || r.purpose
    byCampaign.set(key, (byCampaign.get(key) ?? 0) + toNumber(r.cost))
  }
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Pemakaian Internal"
        description="Pemakaian internal yang sudah diposting per kampanye, tujuan (marketing, event, sampel karyawan, sampling sales), dan divisi, beserta nilai HPP-nya."
      />
      <PeriodFilter
        value={period}
        onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
      />
      <LoadingArea loading={q.isPending}>
        <div className="flex flex-col gap-4">
          {byCampaign.size > 0 ? (
            <BarList
              label="Nilai pemakaian per kampanye/tujuan"
              items={[...byCampaign.entries()].map(([k, v]) => ({
                key: k,
                label: k,
                value: v,
                display: formatRupiah(String(v)),
              }))}
            />
          ) : null}
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[60rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Nomor</TableHead>
                  <TableHead>Tanggal</TableHead>
                  <TableHead>Tujuan</TableHead>
                  <TableHead>Kampanye</TableHead>
                  <TableHead>Divisi</TableHead>
                  <TableHead>Produk</TableHead>
                  <TableHead className="text-right">Terpakai</TableHead>
                  <TableHead className="text-right">Kembali</TableHead>
                  <TableHead className="text-right">Nilai</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.length === 0 ? <EmptyRow colSpan={9} /> : null}
                {items.map((r, i) => (
                  <TableRow key={`${r.usageId}-${i}`}>
                    <TableCell>{r.number}</TableCell>
                    <TableCell>{formatDate(r.businessDate)}</TableCell>
                    <TableCell>
                      {USAGE_PURPOSE_LABELS[r.purpose] ?? r.purpose}
                    </TableCell>
                    <TableCell>{r.campaignCode || "-"}</TableCell>
                    <TableCell>{r.divisionName || "-"}</TableCell>
                    <TableCell>
                      <span className="text-muted-foreground tabular-nums">
                        {r.product.sku}
                      </span>{" "}
                      {r.product.name}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(
                        toNumber(r.consumedQty) === 0 ? r.qty : r.consumedQty
                      )}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(r.returnedQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(r.cost)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell colSpan={8} className="font-medium">
                    Total
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(q.data?.totalCost)}
                  </TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          </div>
        </div>
      </LoadingArea>
    </div>
  )
}

const AGING_COLS = [
  ["current", "Belum Jatuh Tempo"],
  ["d1To30", "1–30 hari"],
  ["d31To60", "31–60 hari"],
  ["d61To90", "61–90 hari"],
  ["over90", "> 90 hari"],
  ["total", "Total"],
] as const

/** Laporan › Laporan Utang: umur utang (RPT-11) dan Outstanding PO (RPT-10). */
export function PayablesReportPage({
  search,
  onSearchChange,
}: {
  search: PayablesSearch
  onSearchChange: SearchUpdater<PayablesSearch>
}) {
  const tab = search.tab ?? "aging"
  const asOf = search.asOf ?? todayBusinessDate()
  const aging = useQuery({
    ...payableAgingQuery(asOf),
    enabled: tab === "aging",
  })
  const po = useQuery({
    ...outstandingPOQuery,
    enabled: tab === "outstanding_po",
  })
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Utang"
        description="Umur utang pemasok per tanggal posisi (dicocokkan dengan saldo akun Utang Usaha) dan Outstanding PO: barang belum diterima serta diterima belum ditagih."
        exportKind={tab === "aging" ? "payable_aging" : "outstanding_po"}
        exportPermission={
          tab === "aging"
            ? "procurement.supplier_bill.read"
            : "procurement.purchase_order.read"
        }
        exportParams={tab === "aging" ? { asOf } : {}}
        actions={
          <Link
            to="/finance/goods-received-not-invoiced"
            className="self-center text-sm underline-offset-2 hover:underline"
          >
            Barang Diterima Belum Ditagih
          </Link>
        }
      />
      <PageTabs
        label="Jenis laporan utang"
        tabs={[
          { id: "aging", label: "Umur Utang" },
          { id: "outstanding_po", label: "Outstanding PO" },
        ]}
        value={tab}
        onValueChange={(t) => onSearchChange((s) => ({ ...s, tab: t }))}
      />
      {tab === "aging" ? (
        <>
          <div className="w-44">
            <DateInput
              value={asOf}
              onValueChange={(v) =>
                onSearchChange((s) => ({ ...s, asOf: v || undefined }))
              }
            />
          </div>
          <LoadingArea loading={aging.isPending}>
            {aging.data ? (
              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <span>
                    Saldo akun 2101: {formatRupiah(aging.data.ledgerBalance)}
                  </span>
                  {toNumber(aging.data.difference) === 0 ? (
                    <Badge variant="secondary">Cocok dengan buku besar</Badge>
                  ) : (
                    <Badge variant="destructive">
                      Selisih {formatRupiah(aging.data.difference)}
                    </Badge>
                  )}
                </div>
                <div className="overflow-x-auto rounded-lg border">
                  <Table className="min-w-[56rem]">
                    <TableHeader>
                      <TableRow>
                        <TableHead>Pemasok</TableHead>
                        {AGING_COLS.map(([k, label]) => (
                          <TableHead key={k} className="text-right">
                            {label}
                          </TableHead>
                        ))}
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {aging.data.items.length === 0 ? (
                        <EmptyRow colSpan={7}>
                          Tidak ada utang terbuka.
                        </EmptyRow>
                      ) : null}
                      {aging.data.items.map((r) => (
                        <TableRow key={r.supplierId}>
                          <TableCell>{r.supplierName}</TableCell>
                          {AGING_COLS.map(([k]) => (
                            <TableCell
                              key={k}
                              className="text-right tabular-nums"
                            >
                              {formatRupiah(r.amounts[k])}
                            </TableCell>
                          ))}
                        </TableRow>
                      ))}
                    </TableBody>
                    <TableFooter>
                      <TableRow>
                        <TableCell className="font-medium">Total</TableCell>
                        {AGING_COLS.map(([k]) => (
                          <TableCell
                            key={k}
                            className="text-right tabular-nums"
                          >
                            {formatRupiah(aging.data.totals[k])}
                          </TableCell>
                        ))}
                      </TableRow>
                    </TableFooter>
                  </Table>
                </div>
              </div>
            ) : null}
          </LoadingArea>
        </>
      ) : (
        <LoadingArea loading={po.isPending}>
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[64rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>PO</TableHead>
                  <TableHead>Pemasok</TableHead>
                  <TableHead>Diharapkan</TableHead>
                  <TableHead>Barang</TableHead>
                  <TableHead className="text-right">Qty PO</TableHead>
                  <TableHead className="text-right">Diterima</TableHead>
                  <TableHead className="text-right">Belum Diterima</TableHead>
                  <TableHead className="text-right">Belum Ditagih</TableHead>
                  <TableHead className="text-right">
                    Nilai Belum Diterima
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(po.data?.items ?? []).length === 0 ? (
                  <EmptyRow colSpan={9}>Tidak ada PO terbuka.</EmptyRow>
                ) : null}
                {(po.data?.items ?? []).map((l) => (
                  <TableRow key={`${l.purchaseOrderId}-${l.lineNo}`}>
                    <TableCell>{l.number}</TableCell>
                    <TableCell>{l.supplierName}</TableCell>
                    <TableCell>{formatDate(l.expectedDate)}</TableCell>
                    <TableCell>{l.item}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(l.orderedQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(l.receivedQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(l.openQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(l.unbilledQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.openValue)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
              <TableFooter>
                <TableRow>
                  <TableCell colSpan={8} className="font-medium">
                    Total
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(po.data?.totalOpenValue)}
                  </TableCell>
                </TableRow>
              </TableFooter>
            </Table>
          </div>
        </LoadingArea>
      )}
    </div>
  )
}
