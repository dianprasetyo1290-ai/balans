import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import Big from "big.js"
import { SearchIcon } from "lucide-react"
import { useState } from "react"

import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
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
import { formatQuantity } from "@/lib/format/number"

import {
  grniReportQuery,
  type GoodsReceivedNotInvoicedReport,
} from "../api/documents-api"
import { Info } from "./shared"

const route = getRouteApi("/_app/finance/goods-received-not-invoiced")

type Line = GoodsReceivedNotInvoicedReport["lines"][number]

/** Kelompokkan baris laporan per PO (urutan dari server: pemasok, PO, GR). */
function groupByOrder(lines: Line[]) {
  const groups: {
    orderId: string
    orderNumber: string
    supplierName: string
    lines: Line[]
    total: Big
  }[] = []
  for (const l of lines) {
    let g = groups.find((x) => x.orderId === l.purchaseOrderId)
    if (!g) {
      g = {
        orderId: l.purchaseOrderId,
        orderNumber: l.purchaseOrderNumber,
        supplierName: l.supplierName,
        lines: [],
        total: new Big(0),
      }
      groups.push(g)
    }
    g.lines.push(l)
    g.total = g.total.plus(l.openAmount)
  }
  return groups
}

/** Laporan Barang Diterima Belum Ditagih per PO/GR, direkonsiliasi ke saldo akun 2102 (S-209, ACC-22). */
export function GoodsReceivedNotInvoicedPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [q, setQ] = useState(search.q ?? "")
  const report = useQuery(grniReportQuery(search.q ?? ""))
  const data = report.data
  const balanced = data ? new Big(data.difference).eq(0) : true

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Barang Diterima Belum Ditagih
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Saldo penerimaan barang yang belum ditagih pemasok per Purchase Order
          dan Penerimaan Barang. Total sub-ledger harus sama dengan saldo akun
          2102 di Buku Besar.
        </p>
      </div>
      <form
        className="relative w-full sm:w-80"
        onSubmit={(e) => {
          e.preventDefault()
          void navigate({ search: { q: q || undefined } })
        }}
      >
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          aria-label="Cari PO, GR, pemasok"
          placeholder="Cari nomor PO, GR, atau pemasok"
          className="pl-8"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </form>
      <LoadingArea loading={report.isPending}>
        {data ? (
          <>
            <dl className="grid gap-x-6 gap-y-3 rounded-lg border p-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <Info label="Per Tanggal" value={formatDate(data.asOf)} />
              <Info
                label="Total Sub-ledger (semua GR)"
                value={formatRupiah(data.subledgerTotal)}
              />
              <Info
                label={`Saldo Akun ${data.accountCode}`}
                value={formatRupiah(data.ledgerBalance)}
              />
              <Info
                label="Selisih"
                value={
                  <span className="flex items-center gap-2">
                    {formatRupiah(data.difference)}
                    <Badge variant={balanced ? "outline" : "destructive"}>
                      {balanced ? "Cocok" : "Tidak cocok"}
                    </Badge>
                  </span>
                }
              />
            </dl>
            {data.lines.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Tidak ada barang diterima yang belum ditagih.
              </p>
            ) : (
              <div className="overflow-x-auto rounded-lg border">
                <Table className="min-w-[64rem]">
                  <TableHeader>
                    <TableRow>
                      <TableHead>Penerimaan / Produk</TableHead>
                      <TableHead>Tanggal</TableHead>
                      <TableHead className="text-right">Diterima</TableHead>
                      <TableHead className="text-right">Ditagih</TableHead>
                      <TableHead className="text-right">Diretur</TableHead>
                      <TableHead className="text-right">
                        Belum Ditagih
                      </TableHead>
                      <TableHead className="text-right">Saldo</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groupByOrder(data.lines).map((g) => (
                      <GroupRows key={g.orderId} group={g} />
                    ))}
                  </TableBody>
                  <TableFooter>
                    <TableRow>
                      <TableCell
                        colSpan={6}
                        className="text-right font-semibold"
                      >
                        Total ditampilkan
                      </TableCell>
                      <TableCell className="text-right font-semibold">
                        {formatRupiah(data.filteredTotal)}
                      </TableCell>
                    </TableRow>
                  </TableFooter>
                </Table>
              </div>
            )}
          </>
        ) : null}
      </LoadingArea>
    </div>
  )
}

function GroupRows({
  group,
}: {
  group: ReturnType<typeof groupByOrder>[number]
}) {
  return (
    <>
      <TableRow className="bg-muted/40">
        <TableCell colSpan={6}>
          <Link
            to="/procurement/purchase-orders/$id"
            params={{ id: group.orderId }}
            search={{}}
            className="font-medium text-primary underline-offset-4 hover:underline"
          >
            {group.orderNumber}
          </Link>
          <span className="text-muted-foreground"> · {group.supplierName}</span>
        </TableCell>
        <TableCell className="text-right font-medium">
          {formatRupiah(group.total.toFixed(2))}
        </TableCell>
      </TableRow>
      {group.lines.map((l) => (
        <TableRow key={l.goodsReceiptLineId}>
          <TableCell>
            <Link
              to="/procurement/goods-receipts/$id"
              params={{ id: l.goodsReceiptId }}
              search={{}}
              className="text-primary underline-offset-4 hover:underline"
            >
              {l.goodsReceiptNumber}
            </Link>
            <div className="text-xs text-muted-foreground">
              {l.product.sku} · {l.product.name} · {formatRupiah(l.unitPrice)}
            </div>
          </TableCell>
          <TableCell className="whitespace-nowrap">
            {formatDate(l.businessDate)}
          </TableCell>
          <TableCell className="text-right tabular-nums">
            {formatQuantity(l.receivedQty)}
          </TableCell>
          <TableCell className="text-right tabular-nums">
            {formatQuantity(l.billedQty)}
          </TableCell>
          <TableCell className="text-right tabular-nums">
            {formatQuantity(l.returnedQty)}
          </TableCell>
          <TableCell className="text-right tabular-nums">
            {formatQuantity(l.openQty)}
          </TableCell>
          <TableCell className="text-right">
            {formatRupiah(l.openAmount)}
          </TableCell>
        </TableRow>
      ))}
    </>
  )
}
