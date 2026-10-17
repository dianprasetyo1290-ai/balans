import { useQuery } from "@tanstack/react-query"

import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
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
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import {
  batchCostQuery,
  stockCountReportQuery,
  stockExpiryQuery,
  stockMovementQuery,
  stockPositionQuery,
} from "../api/reports-api"
import {
  EXPIRY_BUCKET_LABELS,
  LOCATION_TYPE_LABELS,
  resolvePeriod,
  RM,
  toNumber,
  type PeriodSearch,
  type SearchUpdater,
  type StockCountSearch,
  type StockMovementSearch,
  type StockSearch,
} from "../lib/reports-lib"
import { BarList, EmptyRow, PeriodFilter, ReportHeader } from "./report-layout"

const qtyText = (v: string) =>
  toNumber(v).toLocaleString("id-ID", { maximumFractionDigits: 3 })

function LocationTypeFilter({
  value,
  onChange,
}: {
  value?: string
  onChange: (v: string | undefined) => void
}) {
  return (
    <Select
      value={value ?? "all"}
      onValueChange={(v) => onChange(!v || v === "all" ? undefined : v)}
    >
      <SelectTrigger className="w-52" aria-label="Jenis lokasi">
        <SelectValue>
          {(v: string) =>
            v === "all" ? "Semua jenis lokasi" : (LOCATION_TYPE_LABELS[v] ?? v)
          }
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">Semua jenis lokasi</SelectItem>
        {Object.entries(LOCATION_TYPE_LABELS)
          .filter(([k]) => k !== "in_transit")
          .map(([k, label]) => (
            <SelectItem key={k} value={k}>
              {label}
            </SelectItem>
          ))}
      </SelectContent>
    </Select>
  )
}

/** Laporan › Laporan Stok: posisi & nilai per SKU, lokasi, batch (RPT-01). */
export function StockPositionReportPage({
  search,
  onSearchChange,
}: {
  search: StockSearch
  onSearchChange: SearchUpdater<StockSearch>
}) {
  const q = useQuery(stockPositionQuery({ locationType: search.locationType }))
  const data = q.data
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Stok"
        description="Posisi dan nilai stok (HPP FIFO) per SKU, lokasi, dan batch, termasuk lokasi konsinyasi, buffer vending, maklon, dan stok Dalam Pengiriman. Total per jenis lokasi sama dengan saldo akun persediaan."
        exportKind="stock_position"
        exportPermission="inventory.stock.read"
        exportParams={{ locationType: search.locationType }}
        readModels={[...RM.stock]}
      />
      <LocationTypeFilter
        value={search.locationType}
        onChange={(v) => onSearchChange((s) => ({ ...s, locationType: v }))}
      />
      <LoadingArea loading={q.isPending}>
        {data ? (
          <div className="flex flex-col gap-4">
            <BarList
              label="Nilai stok per jenis lokasi"
              items={data.byLocationType.map((t) => ({
                key: t.locationType,
                label: LOCATION_TYPE_LABELS[t.locationType] ?? t.locationType,
                value: toNumber(t.value),
                display: `${formatRupiah(t.value)} · ${qtyText(t.qty)}`,
              }))}
            />
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[56rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>SKU</TableHead>
                    <TableHead>Produk</TableHead>
                    <TableHead>Lokasi</TableHead>
                    <TableHead>Batch</TableHead>
                    <TableHead>Kedaluwarsa</TableHead>
                    <TableHead className="text-right">Qty</TableHead>
                    <TableHead className="text-right">Nilai</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.items.length === 0 ? <EmptyRow colSpan={7} /> : null}
                  {data.items.map((r) => (
                    <TableRow
                      key={`${r.product.id}-${r.location.id}-${r.batchId ?? ""}`}
                    >
                      <TableCell className="tabular-nums">
                        {r.product.sku}
                      </TableCell>
                      <TableCell>{r.product.name}</TableCell>
                      <TableCell>
                        {r.location.code}{" "}
                        <span className="text-muted-foreground">
                          (
                          {LOCATION_TYPE_LABELS[r.location.type] ??
                            r.location.type}
                          )
                        </span>
                      </TableCell>
                      <TableCell>{r.batchNumber || "-"}</TableCell>
                      <TableCell>{formatDate(r.expiryDate)}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {qtyText(r.qty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(r.value)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell colSpan={5} className="font-medium">
                      Total
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {qtyText(data.totalQty)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(data.totalValue)}
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

/** Laporan › Laporan Mutasi Stok (RPT-02). */
export function StockMovementReportPage({
  search,
  onSearchChange,
}: {
  search: StockMovementSearch
  onSearchChange: SearchUpdater<StockMovementSearch>
}) {
  const period = resolvePeriod(search)
  const q = useQuery(
    stockMovementQuery(period, { locationType: search.locationType })
  )
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Mutasi Stok"
        description="Saldo awal, barang masuk, keluar, pindah lokasi/status, dan saldo akhir per SKU per lokasi. Rincian per transaksi ada di Persediaan › Kartu Stok."
        exportKind="stock_movement"
        exportPermission="inventory.stock.read"
        exportParams={{ ...period, locationType: search.locationType }}
        readModels={[...RM.stock]}
      />
      <div className="flex flex-wrap items-center gap-3">
        <PeriodFilter
          value={period}
          onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
        />
        <LocationTypeFilter
          value={search.locationType}
          onChange={(v) => onSearchChange((s) => ({ ...s, locationType: v }))}
        />
      </div>
      <LoadingArea loading={q.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[64rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead>Lokasi</TableHead>
                <TableHead className="text-right">Saldo Awal</TableHead>
                <TableHead className="text-right">Masuk</TableHead>
                <TableHead className="text-right">Keluar</TableHead>
                <TableHead className="text-right">Pindah Masuk</TableHead>
                <TableHead className="text-right">Pindah Keluar</TableHead>
                <TableHead className="text-right">Saldo Akhir</TableHead>
                <TableHead className="text-right">Nilai Akhir</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(q.data?.items ?? []).length === 0 ? (
                <EmptyRow colSpan={9} />
              ) : null}
              {(q.data?.items ?? []).map((r) => (
                <TableRow key={`${r.product.id}-${r.location.id}`}>
                  <TableCell>
                    <span className="text-muted-foreground tabular-nums">
                      {r.product.sku}
                    </span>{" "}
                    {r.product.name}
                  </TableCell>
                  <TableCell>{r.location.code}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.openingQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.receiveQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.issueQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.moveInQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.moveOutQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.closingQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.closingValue)}
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

/** Laporan › Laporan Stock Count: hasil hitung & selisih (RPT-03). */
export function StockCountReportPage({
  search,
  onSearchChange,
}: {
  search: StockCountSearch
  onSearchChange: SearchUpdater<StockCountSearch>
}) {
  const period = resolvePeriod(search)
  const varianceOnly = search.varianceOnly ?? true
  const q = useQuery(stockCountReportQuery(period, varianceOnly))
  const data = q.data
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Stock Count"
        description="Hasil stock count per baris: qty sistem saat snapshot, qty hitung, selisih qty, dan nilai selisih (HPP snapshot)."
        exportKind="stock_count"
        exportPermission="warehouse.stock_count.read"
        exportParams={{ ...period, varianceOnly: String(varianceOnly) }}
      />
      <div className="flex flex-wrap items-center gap-3">
        <PeriodFilter
          value={period}
          onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
        />
        <Button
          variant={varianceOnly ? "default" : "outline"}
          className="min-h-11 sm:min-h-9"
          aria-pressed={varianceOnly}
          onClick={() =>
            onSearchChange((s) => ({ ...s, varianceOnly: !varianceOnly }))
          }
        >
          Hanya yang selisih
        </Button>
      </div>
      <LoadingArea loading={q.isPending}>
        {data ? (
          <div className="flex flex-col gap-2">
            <p className="text-sm text-muted-foreground">
              Selisih kurang {formatRupiah(data.totalShortageAmount)} · lebih{" "}
              {formatRupiah(data.totalOverageAmount)} · bersih{" "}
              {formatRupiah(data.totalVarianceAmount)}
            </p>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[64rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Nomor</TableHead>
                    <TableHead>Tanggal</TableHead>
                    <TableHead>Lokasi</TableHead>
                    <TableHead>Produk</TableHead>
                    <TableHead>Batch</TableHead>
                    <TableHead className="text-right">Sistem</TableHead>
                    <TableHead className="text-right">Hitung</TableHead>
                    <TableHead className="text-right">Selisih</TableHead>
                    <TableHead className="text-right">Nilai Selisih</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.items.length === 0 ? <EmptyRow colSpan={9} /> : null}
                  {data.items.map((r, i) => (
                    <TableRow key={`${r.stockCountId}-${i}`}>
                      <TableCell>{r.number}</TableCell>
                      <TableCell>{formatDate(r.businessDate)}</TableCell>
                      <TableCell>{r.locationCode}</TableCell>
                      <TableCell>
                        <span className="text-muted-foreground tabular-nums">
                          {r.sku}
                        </span>{" "}
                        {r.productName}
                        {r.isExtra ? (
                          <Badge variant="outline" className="ml-1">
                            Temuan
                          </Badge>
                        ) : null}
                      </TableCell>
                      <TableCell>{r.batchNumber || "-"}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {qtyText(r.systemQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {r.countedQty ? qtyText(r.countedQty) : "-"}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {qtyText(r.varianceQty)}
                      </TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(r.varianceAmount)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        ) : null}
      </LoadingArea>
    </div>
  )
}

/** Laporan › Laporan HPP: komposisi HPP per SKU per batch dengan rincian reject (RPT-04). */
export function CogsReportPage({
  search,
  onSearchChange,
}: {
  search: PeriodSearch
  onSearchChange: SearchUpdater<PeriodSearch>
}) {
  const period = resolvePeriod(search)
  const q = useQuery(batchCostQuery(period))
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan HPP"
        description="HPP per SKU per batch dari Penerimaan Barang: qty baik, reject (diretur atau diserap ke HPP), nilai barang/jasa maklon, packaging terpakai, nilai batch, dan HPP per unit. Sisa cost layer FIFO ada di Persediaan › HPP per Batch."
      />
      <PeriodFilter
        value={period}
        onChange={(p) => onSearchChange((s) => ({ ...s, ...p }))}
      />
      <LoadingArea loading={q.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[72rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Penerimaan</TableHead>
                <TableHead>Pemasok</TableHead>
                <TableHead>Produk</TableHead>
                <TableHead>Batch</TableHead>
                <TableHead className="text-right">Qty Baik</TableHead>
                <TableHead className="text-right">Reject</TableHead>
                <TableHead>Perlakuan Reject</TableHead>
                <TableHead className="text-right">Nilai Barang/Jasa</TableHead>
                <TableHead className="text-right">Packaging</TableHead>
                <TableHead className="text-right">Nilai Batch</TableHead>
                <TableHead className="text-right">HPP/Unit</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(q.data?.items ?? []).length === 0 ? (
                <EmptyRow colSpan={11} />
              ) : null}
              {(q.data?.items ?? []).map((r, i) => (
                <TableRow key={`${r.receiptId}-${i}`}>
                  <TableCell>
                    {r.receiptNumber}
                    <div className="text-xs text-muted-foreground">
                      {formatDate(r.businessDate)}
                    </div>
                  </TableCell>
                  <TableCell>{r.supplierName}</TableCell>
                  <TableCell>
                    <span className="text-muted-foreground tabular-nums">
                      {r.sku}
                    </span>{" "}
                    {r.productName}
                  </TableCell>
                  <TableCell>{r.batchNumber || "-"}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.goodQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.rejectQty)}
                  </TableCell>
                  <TableCell>
                    {toNumber(r.rejectQty) === 0
                      ? "-"
                      : r.rejectDisposition === "absorbed"
                        ? "Diserap ke HPP"
                        : "Diretur ke pemasok"}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.serviceAmount)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.materialCost)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.inventoryAmount)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {toNumber(r.unitCost).toLocaleString("id-ID", {
                      maximumFractionDigits: 4,
                    })}
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

/** Laporan › Laporan Kedaluwarsa: stok mendekati kedaluwarsa & umur stok (RPT-19). */
export function ExpiryReportPage({
  search,
  onSearchChange,
}: {
  search: StockSearch
  onSearchChange: SearchUpdater<StockSearch>
}) {
  const within = search.withinDays
  const q = useQuery(
    stockExpiryQuery({ locationType: search.locationType }, within)
  )
  const data = q.data
  return (
    <div className="flex flex-col gap-4">
      <ReportHeader
        title="Laporan Kedaluwarsa"
        description="Sisa umur simpan dan umur stok (sejak pertama masuk lokasi) per batch per lokasi, diurutkan dari yang paling dekat kedaluwarsa."
        exportKind="stock_expiry"
        exportPermission="inventory.stock.read"
        exportParams={{
          locationType: search.locationType,
          withinDays: within ? String(within) : undefined,
        }}
        readModels={[...RM.stock]}
      />
      <div className="flex flex-wrap items-center gap-3">
        <LocationTypeFilter
          value={search.locationType}
          onChange={(v) => onSearchChange((s) => ({ ...s, locationType: v }))}
        />
        <Select
          value={within ? String(within) : "all"}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              withinDays: !v || v === "all" ? undefined : Number(v),
            }))
          }
        >
          <SelectTrigger className="w-56" aria-label="Rentang kedaluwarsa">
            <SelectValue>
              {(v: string) =>
                v === "all" ? "Semua batch" : `Kedaluwarsa ≤ ${v} hari`
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua batch</SelectItem>
            {[30, 60, 90, 180].map((d) => (
              <SelectItem key={d} value={String(d)}>
                Kedaluwarsa ≤ {d} hari
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <LoadingArea loading={q.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[60rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead>Lokasi</TableHead>
                <TableHead>Batch</TableHead>
                <TableHead>Kedaluwarsa</TableHead>
                <TableHead className="text-right">Sisa Hari</TableHead>
                <TableHead className="text-right">Umur Stok</TableHead>
                <TableHead className="text-right">Qty</TableHead>
                <TableHead className="text-right">Nilai</TableHead>
                <TableHead>Kelompok</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(data?.items ?? []).length === 0 ? (
                <EmptyRow colSpan={9} />
              ) : null}
              {(data?.items ?? []).map((r, i) => (
                <TableRow
                  key={`${r.product.id}-${r.location.id}-${r.batchNumber}-${i}`}
                >
                  <TableCell>
                    <span className="text-muted-foreground tabular-nums">
                      {r.product.sku}
                    </span>{" "}
                    {r.product.name}
                  </TableCell>
                  <TableCell>{r.location.code}</TableCell>
                  <TableCell>{r.batchNumber || "-"}</TableCell>
                  <TableCell>{formatDate(r.expiryDate)}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {r.daysToExpiry ?? "-"}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {r.ageDays === undefined ? "-" : `${r.ageDays} hari`}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {qtyText(r.qty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(r.value)}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        r.bucket === "expired" || r.bucket === "near"
                          ? "destructive"
                          : "secondary"
                      }
                    >
                      {EXPIRY_BUCKET_LABELS[r.bucket]}
                    </Badge>
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
