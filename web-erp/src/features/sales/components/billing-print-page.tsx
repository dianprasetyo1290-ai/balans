import { useQuery } from "@tanstack/react-query"
import { PrinterIcon } from "lucide-react"

import { LoadingArea } from "@/components/common/loading-area"
import { Button } from "@/components/ui/button"
import { terbilang } from "@/features/procurement/lib/procurement-lib"
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"

import { invoiceQuery, proformaQuery } from "../api/sales-documents-api"

/** Keterangan NON-PKP (Desain Dokumen Cetak 2.2, D-04). */
export const NON_PKP_NOTE =
  "Balans bukan Pengusaha Kena Pajak; tidak dipungut PPN."

const PRINT_STYLES = `
@page { size: A4 portrait; margin: 12mm; }
@media print {
  body * { visibility: hidden !important; }
  #billing-print-root, #billing-print-root * { visibility: visible !important; }
  #billing-print-root { position: absolute; inset: 0 auto auto 0; width: 100%; margin: 0; border: 0; box-shadow: none; }
}
`

type PrintLine = {
  id: string
  description: string
  qty: string
  unitPrice: string
  discountAmount: string
  lineTotalAmount: string
}

type PrintDoc = {
  title: string
  number: string
  partyName: string
  address: string
  meta: [string, string][]
  lines: PrintLine[]
  gross: string
  discount: string
  shipping: string
  total: string
  notes: string[]
  paid?: boolean
}

/** Cetak Proforma Invoice dari browser (HTML + CSS cetak A4) dengan keterangan NON-PKP. */
export function ProformaPrintPage({ id }: { id: string }) {
  const pi = useQuery(proformaQuery(id))
  const o = pi.data?.salesOrder
  return (
    <LoadingArea loading={pi.isPending}>
      {pi.data && o ? (
        <PrintView
          doc={{
            title: "Proforma Invoice",
            number: pi.data.number,
            partyName: pi.data.partyName,
            address: o.shipToAddress,
            meta: [
              ["Tanggal", formatDate(pi.data.issueDate)],
              ["Berlaku s.d.", formatDate(pi.data.validUntil)],
              ["Sales Order", o.number ?? ""],
              ...(o.clientPoNumber
                ? ([["PO Klien", o.clientPoNumber]] as [string, string][])
                : []),
            ],
            lines: o.lines.map((l) => ({
              id: l.id,
              description: `${l.product.sku} – ${l.product.name}`,
              qty: l.qty,
              unitPrice: l.unitPrice,
              discountAmount: l.discountAmount,
              lineTotalAmount: l.lineTotalAmount,
            })),
            gross: o.grossAmount,
            discount: (
              Number(o.lineDiscountAmount) + Number(o.documentDiscountAmount)
            ).toFixed(2),
            shipping: o.shippingAmount,
            total: pi.data.amount,
            notes: [
              "Dokumen ini bukan bukti pembayaran.",
              NON_PKP_NOTE,
              `Mohon lakukan pembayaran sebelum ${formatDate(pi.data.validUntil)}. Setelah lewat, pesanan otomatis dibatalkan dan stok dilepas.`,
            ],
          }}
        />
      ) : null}
    </LoadingArea>
  )
}

/** Cetak Invoice NON-PKP dari browser. */
export function InvoicePrintPage({ id }: { id: string }) {
  const inv = useQuery(invoiceQuery(id))
  return (
    <LoadingArea loading={inv.isPending}>
      {inv.data ? (
        <PrintView
          doc={{
            title: "Invoice",
            number: inv.data.number,
            partyName: inv.data.partyName,
            address: inv.data.shipToAddress,
            meta: [
              ["Tanggal", formatDate(inv.data.invoiceDate)],
              ["Jatuh tempo", formatDate(inv.data.dueDate)],
              ["Sales Order", inv.data.salesOrderNumber],
              [
                "Status",
                inv.data.status === "paid"
                  ? "LUNAS"
                  : `Sisa ${formatRupiah(inv.data.outstandingAmount)}`,
              ],
            ],
            lines: inv.data.lines,
            gross: inv.data.grossAmount,
            discount: inv.data.discountAmount,
            shipping: inv.data.shippingAmount,
            total: inv.data.totalAmount,
            notes: [NON_PKP_NOTE],
            paid: inv.data.status === "paid",
          }}
        />
      ) : null}
    </LoadingArea>
  )
}

function PrintView({ doc }: { doc: PrintDoc }) {
  return (
    <div className="flex flex-col gap-4">
      <style>{PRINT_STYLES}</style>
      <div className="flex flex-wrap items-center justify-between gap-3 print:hidden">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Cetak {doc.title} {doc.number}
          </h1>
          <p className="text-sm text-muted-foreground">
            Pratinjau A4. Tekan Cetak untuk mencetak atau simpan sebagai PDF
            dari dialog cetak browser.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => window.history.back()}>
            Kembali
          </Button>
          <Button onClick={() => window.print()}>
            <PrinterIcon /> Cetak
          </Button>
        </div>
      </div>
      <article
        id="billing-print-root"
        className="mx-auto w-full max-w-[210mm] rounded-lg border bg-white p-8 text-[12px] leading-relaxed text-black shadow-sm"
      >
        <header className="flex items-start justify-between gap-4 border-b border-black pb-3">
          <div>
            <p className="text-lg font-bold tracking-tight">Balans Nutrition</p>
            <p>NON-PKP</p>
          </div>
          <div className="text-right">
            <p className="text-lg font-bold">{doc.title}</p>
            <p className="font-mono">{doc.number}</p>
            {doc.paid ? (
              <p className="mt-1 inline-block border border-black px-2 font-bold">
                LUNAS
              </p>
            ) : null}
          </div>
        </header>
        <section className="mt-4 grid grid-cols-2 gap-6">
          <div>
            <p className="font-semibold">Kepada</p>
            <p>{doc.partyName}</p>
            {doc.address ? (
              <p className="whitespace-pre-line">{doc.address}</p>
            ) : null}
          </div>
          <dl className="grid grid-cols-[auto_1fr] gap-x-3">
            {doc.meta.map(([k, v]) => (
              <div key={k} className="contents">
                <dt>{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
        </section>
        <table className="mt-4 w-full border-collapse">
          <thead>
            <tr className="border-y border-black text-left">
              <th className="py-1 pr-2">No</th>
              <th className="py-1 pr-2">Uraian</th>
              <th className="py-1 pr-2 text-right">Qty</th>
              <th className="py-1 pr-2 text-right">Harga</th>
              <th className="py-1 pr-2 text-right">Diskon</th>
              <th className="py-1 text-right">Jumlah</th>
            </tr>
          </thead>
          <tbody>
            {doc.lines.map((l, i) => (
              <tr key={l.id} className="border-b border-gray-300">
                <td className="py-1 pr-2">{i + 1}</td>
                <td className="py-1 pr-2">{l.description}</td>
                <td className="py-1 pr-2 text-right">
                  {formatQuantity(l.qty)}
                </td>
                <td className="py-1 pr-2 text-right">
                  {formatRupiah(l.unitPrice)}
                </td>
                <td className="py-1 pr-2 text-right">
                  {Number(l.discountAmount) > 0
                    ? formatRupiah(l.discountAmount)
                    : "—"}
                </td>
                <td className="py-1 text-right">
                  {formatRupiah(l.lineTotalAmount)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <section className="mt-3 ml-auto w-72">
          <dl className="grid grid-cols-[1fr_auto] gap-x-3">
            <dt>Subtotal (sebelum diskon)</dt>
            <dd className="text-right">{formatRupiah(doc.gross)}</dd>
            {Number(doc.discount) > 0 ? (
              <>
                <dt>Diskon</dt>
                <dd className="text-right">−{formatRupiah(doc.discount)}</dd>
              </>
            ) : null}
            {Number(doc.shipping) > 0 ? (
              <>
                <dt>Ongkos kirim</dt>
                <dd className="text-right">{formatRupiah(doc.shipping)}</dd>
              </>
            ) : null}
            <dt className="border-t border-black pt-1 font-bold">Total</dt>
            <dd className="border-t border-black pt-1 text-right font-bold">
              {formatRupiah(doc.total)}
            </dd>
          </dl>
        </section>
        <p className="mt-2 italic">Terbilang: {terbilang(doc.total)}</p>
        <section className="mt-6 border-t border-gray-400 pt-3">
          {doc.notes.map((n) => (
            <p key={n}>{n}</p>
          ))}
        </section>
        <section className="mt-10 grid grid-cols-2 gap-6 text-center">
          <div />
          <div>
            <p>Finance & Accounting</p>
            <div className="h-16" />
            <p className="border-t border-black pt-1">Balans Nutrition</p>
          </div>
        </section>
      </article>
    </div>
  )
}
