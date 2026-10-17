import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { PrinterIcon } from "lucide-react"

import { LoadingArea } from "@/components/common/loading-area"
import { Button } from "@/components/ui/button"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"

import {
  orderQuery,
  subjectApprovalsQuery,
  type PurchaseOrder,
} from "../api/procurement-api"
import { terbilang } from "../lib/procurement-lib"

/** CSS cetak: hanya dokumen PO yang tercetak, kertas A4 (NFR-08). */
const PRINT_STYLES = `
@page { size: A4 portrait; margin: 12mm; }
@media print {
  body * { visibility: hidden !important; }
  #po-print-root, #po-print-root * { visibility: visible !important; }
  #po-print-root { position: absolute; inset: 0 auto auto 0; width: 100%; margin: 0; border: 0; box-shadow: none; }
}
`

/**
 * Cetak PO dari browser (template HTML + CSS cetak) sampai template PDF server (Gotenberg) tersedia
 * (Desain Dokumen Cetak 2.2): pemasok, tanggal kirim, termin, uang muka, daftar packaging untuk PO maklon,
 * tanda tangan pembuat dan approver terakhir.
 */
export function OrderPrintPage({ id }: { id: string }) {
  const order = useQuery(orderQuery(id))
  return (
    <LoadingArea loading={order.isPending}>
      {order.data ? <OrderPrint order={order.data} /> : null}
    </LoadingArea>
  )
}

function OrderPrint({ order }: { order: PurchaseOrder }) {
  const approvals = useQuery(subjectApprovalsQuery("purchase_order", order.id))
  const approved = approvals.data?.items.find((r) => r.status === "approved")
  const approvedSteps =
    approved?.steps.filter((s) => s.status === "approved") ?? []
  const lastStep = approvedSteps[approvedSteps.length - 1] as
    (typeof approvedSteps)[number] | undefined
  const toll =
    order.orderType === "toll_manufacturing" || order.orderType === "reprocess"
  const postedAdvances = order.advances.filter((a) => a.status === "posted")
  const printable = order.allowedActions.includes("print")

  return (
    <div className="flex flex-col gap-4">
      <style>{PRINT_STYLES}</style>
      <div className="flex flex-wrap items-center justify-between gap-3 print:hidden">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Cetak PO {order.number}
          </h1>
          <p className="text-sm text-muted-foreground">
            Pratinjau A4. Tekan Cetak untuk mencetak atau simpan sebagai PDF
            dari dialog cetak browser.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            render={
              <Link
                to="/procurement/purchase-orders/$id"
                params={{ id: order.id }}
                search={{}}
              />
            }
          >
            Kembali
          </Button>
          <Button disabled={!printable} onClick={() => window.print()}>
            <PrinterIcon /> Cetak
          </Button>
        </div>
      </div>
      {!printable ? (
        <p className="text-sm text-destructive print:hidden">
          PO baru bisa dicetak setelah disetujui.
        </p>
      ) : null}
      <article
        id="po-print-root"
        className="mx-auto w-full max-w-[210mm] rounded-lg border bg-white p-8 text-[12px] leading-relaxed text-black shadow-sm"
      >
        <header className="flex items-start justify-between gap-4 border-b border-black pb-3">
          <div>
            <p className="text-lg font-bold tracking-tight">Balans Nutrition</p>
            <p>Unified Business Platform</p>
          </div>
          <div className="text-right">
            <p className="text-lg font-bold">
              {toll
                ? `Purchase Order ${order.orderTypeLabel}`
                : "Purchase Order"}
            </p>
            <p className="font-mono">{order.number}</p>
          </div>
        </header>
        <section className="mt-4 grid grid-cols-2 gap-6">
          <div>
            <p className="font-semibold">Kepada</p>
            <p>{order.supplier?.name ?? order.supplierName}</p>
            {order.supplier?.address ? (
              <p className="whitespace-pre-line">{order.supplier.address}</p>
            ) : null}
            {order.supplier?.phone ? <p>Telp. {order.supplier.phone}</p> : null}
            {order.supplier?.picName ? (
              <p>U.p. {order.supplier.picName}</p>
            ) : null}
          </div>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3">
            <dt>Tanggal PO</dt>
            <dd>{formatDate(order.businessDate)}</dd>
            <dt>Tanggal Kirim Diharapkan</dt>
            <dd>{formatDate(order.expectedDate)}</dd>
            <dt>Syarat Pembayaran</dt>
            <dd>
              {order.paymentTermDays === 0
                ? "Tunai"
                : `${order.paymentTermDays} hari`}
            </dd>
            <dt>Divisi</dt>
            <dd>{order.divisionName}</dd>
            {order.tollLocation ? (
              <>
                <dt>Lokasi Maklon</dt>
                <dd>{order.tollLocation.name}</dd>
              </>
            ) : null}
          </dl>
        </section>
        <table className="mt-4 w-full border-collapse">
          <thead>
            <tr className="border-y border-black text-left">
              <th className="py-1 pr-2">No</th>
              <th className="py-1 pr-2">SKU</th>
              <th className="py-1 pr-2">
                {toll ? "Produk Jadi (hasil)" : "Nama Barang"}
              </th>
              <th className="py-1 pr-2 text-right">Qty</th>
              <th className="py-1 pr-2">Satuan</th>
              <th className="py-1 pr-2 text-right">
                {toll ? "Harga Jasa" : "Harga"}
              </th>
              <th className="py-1 text-right">Jumlah</th>
            </tr>
          </thead>
          <tbody>
            {order.lines.map((l) => (
              <tr key={l.id} className="border-b border-gray-300 align-top">
                <td className="py-1 pr-2">{l.lineNo}</td>
                <td className="py-1 pr-2 font-mono">{l.product?.sku ?? "—"}</td>
                <td className="py-1 pr-2">
                  {l.product?.name ?? l.description}
                  {l.note ? (
                    <span className="block text-[11px] text-gray-600">
                      {l.note}
                    </span>
                  ) : null}
                </td>
                <td className="py-1 pr-2 text-right tabular-nums">
                  {formatQuantity(l.orderedQty)}
                </td>
                <td className="py-1 pr-2">{l.product?.uom ?? ""}</td>
                <td className="py-1 pr-2 text-right tabular-nums">
                  {formatRupiah(l.unitPrice)}
                </td>
                <td className="py-1 text-right tabular-nums">
                  {formatRupiah(l.amount)}
                </td>
              </tr>
            ))}
          </tbody>
          <tfoot>
            <tr>
              <td colSpan={6} className="pt-2 text-right font-semibold">
                Total
              </td>
              <td className="pt-2 text-right font-semibold tabular-nums">
                {formatRupiah(order.totalAmount)}
              </td>
            </tr>
          </tfoot>
        </table>
        <p className="mt-1 italic">Terbilang: {terbilang(order.totalAmount)}</p>
        {order.materials.length > 0 ? (
          <section className="mt-4">
            <p className="font-semibold">
              {order.orderType === "reprocess"
                ? "Bahan GM yang dikirim ke maklon"
                : "Packaging yang dikirim / dipakai maklon"}
              {order.materialSourceLocation
                ? ` (dari ${order.materialSourceLocation.name})`
                : ""}
            </p>
            <table className="mt-1 w-full border-collapse">
              <thead>
                <tr className="border-y border-black text-left">
                  <th className="py-1 pr-2">SKU</th>
                  <th className="py-1 pr-2">Nama</th>
                  <th className="py-1 pr-2 text-right">Qty Rencana</th>
                  <th className="py-1">Satuan</th>
                </tr>
              </thead>
              <tbody>
                {order.materials.map((m) => (
                  <tr key={m.id} className="border-b border-gray-300">
                    <td className="py-1 pr-2 font-mono">{m.product.sku}</td>
                    <td className="py-1 pr-2">{m.product.name}</td>
                    <td className="py-1 pr-2 text-right tabular-nums">
                      {formatQuantity(m.plannedQty)}
                    </td>
                    <td className="py-1">{m.product.uom}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        ) : null}
        {postedAdvances.length > 0 ? (
          <section className="mt-4">
            <p className="font-semibold">Uang muka</p>
            <ul>
              {postedAdvances.map((a) => (
                <li key={a.id}>
                  {a.number} · {formatDate(a.paymentDate)} ·{" "}
                  {formatRupiah(a.amount)}
                </li>
              ))}
            </ul>
          </section>
        ) : null}
        {order.note ? (
          <section className="mt-4">
            <p className="font-semibold">Keterangan</p>
            <p className="whitespace-pre-line">{order.note}</p>
          </section>
        ) : null}
        <section className="mt-10 grid grid-cols-2 gap-10 text-center">
          <div>
            <p>Dibuat oleh</p>
            <div className="h-16" />
            <p className="border-t border-black pt-1 font-semibold">
              {order.createdByName || "—"}
            </p>
            <p className="text-[11px]">
              {order.submittedAt ? formatDateTime(order.submittedAt) : ""}
            </p>
          </div>
          <div>
            <p>Disetujui</p>
            <div className="h-16" />
            <p className="border-t border-black pt-1 font-semibold">
              {lastStep?.decidedByName ?? "—"}
            </p>
            <p className="text-[11px]">
              {lastStep?.decidedAt ? formatDateTime(lastStep.decidedAt) : ""}
            </p>
          </div>
        </section>
      </article>
    </div>
  )
}
