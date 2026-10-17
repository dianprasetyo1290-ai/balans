import { useQuery } from "@tanstack/react-query"
import { AlertTriangleIcon, PrinterIcon } from "lucide-react"
import { useState } from "react"
import { createPortal } from "react-dom"

import { LoadingArea } from "@/components/common/loading-area"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  ProductLabelView,
  type ProductLabelContent,
} from "@/features/catalog/components/product-labels-page"
import { formatDate } from "@/lib/format/date"

import {
  batchText,
  colliLabels,
  DELIVERY_TYPE_LABELS,
  deliveryOrderQuery,
  deliveryOrdersQuery,
  type ColliLabel,
  type DeliveryOrder,
  type DeliveryProductLabel,
} from "../api/logistics-api"

const DO_PRINT = `
@page { size: A4 portrait; margin: 12mm; }
@media print {
  body * { visibility: hidden !important; }
  #do-print-root, #do-print-root * { visibility: visible !important; }
  #do-print-root { position: absolute; inset: 0 auto auto 0; width: 100%; margin: 0; border: 0; box-shadow: none; }
}`

/** Cetak Delivery Order / surat jalan tanpa harga dengan batch & kedaluwarsa (Desain Dokumen Cetak, NFR-08). */
export function DeliveryOrderPrintPage({ id }: { id: string }) {
  const q = useQuery(deliveryOrderQuery(id))
  const d = q.data
  return (
    <LoadingArea loading={q.isPending}>
      {d ? (
        <div className="flex flex-col gap-4">
          <style>{DO_PRINT}</style>
          <div className="flex justify-end">
            <Button onClick={() => window.print()}>
              <PrinterIcon /> Cetak
            </Button>
          </div>
          <article
            id="do-print-root"
            className="mx-auto w-full max-w-[210mm] rounded-lg border bg-white p-8 text-sm text-black shadow-sm"
          >
            <header className="flex items-start justify-between gap-4 border-b pb-3">
              <div>
                <div className="text-lg font-bold">Balans Nutrition</div>
                <div className="text-xs">Delivery Order / Surat Jalan</div>
              </div>
              <div className="text-right">
                <div className="text-base font-semibold">{d.number}</div>
                <div>{formatDate(d.businessDate)}</div>
                <div>{DELIVERY_TYPE_LABELS[d.deliveryType]}</div>
              </div>
            </header>
            <section className="grid grid-cols-2 gap-4 py-3">
              <div>
                <div className="text-xs text-gray-600">Kepada</div>
                <div className="font-medium">{d.recipientName}</div>
                <div>{d.recipientPhone}</div>
                <div className="whitespace-pre-line">{d.shipToAddress}</div>
              </div>
              <div>
                <div className="text-xs text-gray-600">Dokumen asal</div>
                <div>{d.sourceLabel}</div>
                <div className="mt-1 text-xs text-gray-600">Dari</div>
                <div>{d.location.warehouseName ?? d.location.name}</div>
                {d.shipment ? (
                  <>
                    <div className="mt-1 text-xs text-gray-600">Kurir</div>
                    <div>
                      {d.shipment.courier.name}{" "}
                      {d.shipment.trackingNumber || d.shipment.marketplaceAwb}
                      {d.shipment.internalCourierName
                        ? ` · ${d.shipment.internalCourierName}`
                        : ""}
                    </div>
                  </>
                ) : null}
              </div>
            </section>
            <table className="w-full border-collapse">
              <thead>
                <tr>
                  {["No", "Kode", "Produk", "Batch", "Kedaluwarsa", "Qty"].map(
                    (h) => (
                      <th key={h} className="border px-2 py-1 text-left">
                        {h}
                      </th>
                    )
                  )}
                </tr>
              </thead>
              <tbody>
                {d.printLines.map((l, i) => (
                  <tr key={i}>
                    <td className="border px-2 py-1">{i + 1}</td>
                    <td className="border px-2 py-1">{l.product.sku}</td>
                    <td className="border px-2 py-1">{l.product.name}</td>
                    <td className="border px-2 py-1">{batchText(l.batch)}</td>
                    <td className="border px-2 py-1">
                      {l.batch?.expiryDate
                        ? formatDate(l.batch.expiryDate)
                        : "—"}
                    </td>
                    <td className="border px-2 py-1 text-right">
                      {Number(l.qty)} {l.product.uom}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="mt-2 text-xs">
              Jumlah koli: {d.packages.length || "—"}
            </p>
            {d.note ? <p className="mt-1 text-xs">Catatan: {d.note}</p> : null}
            <footer className="mt-10 grid grid-cols-3 gap-6 text-center text-xs">
              {["Disiapkan", "Pengirim / Kurir", "Penerima"].map((s) => (
                <div key={s}>
                  {s}
                  <div className="mt-14 border-t" />
                </div>
              ))}
            </footer>
          </article>
        </div>
      ) : null}
    </LoadingArea>
  )
}

const COLLI_PRINT = `
@page { size: 100mm 150mm; margin: 0; }
@media print {
  body > *:not(#label-print-root) { display: none !important; }
  #label-print-root { display: block !important; }
  html, body { background: #fff !important; }
}`

const PRODUCT_PRINT = `
@page { size: 50mm 30mm; margin: 0; }
@media print {
  body > *:not(#label-print-root) { display: none !important; }
  #label-print-root { display: block !important; }
  html, body { background: #fff !important; }
}`

/** Label koli 100 × 150 mm: nomor DO, koli n/total, penerima, isi (S-316). */
export function ColliLabelView({ label }: { label: ColliLabel }) {
  return (
    <div className="flex h-[150mm] w-[100mm] [break-inside:avoid] flex-col gap-[2mm] overflow-hidden border bg-white p-[5mm] text-black">
      <div className="flex items-start justify-between">
        <div className="text-[10pt] font-bold">Balans Nutrition</div>
        <div className="text-right text-[20pt] leading-none font-bold">
          {label.n}/{label.total}
        </div>
      </div>
      <div className="text-[14pt] font-bold">{label.doNumber}</div>
      <div className="border-t pt-[2mm]">
        <div className="text-[8pt]">Penerima</div>
        <div className="text-[12pt] font-semibold">{label.recipient}</div>
        <div className="text-[9pt]">{label.phone}</div>
        <div className="text-[9pt] whitespace-pre-line">{label.address}</div>
      </div>
      <div className="flex-1 border-t pt-[2mm]">
        <div className="text-[8pt]">
          Isi koli
          {label.weightGrams
            ? ` · ${(label.weightGrams / 1000).toLocaleString("id-ID")} kg`
            : ""}
        </div>
        <ul className="text-[8pt] leading-tight">
          {label.contents.map((c, i) => (
            <li key={i}>{c}</li>
          ))}
        </ul>
      </div>
    </div>
  )
}

/** Isi label produk dari baris DO: kode artikel & barcode mitra bila ada, selain itu SKU & barcode produk. */
export function partnerLabelContent(
  l: DeliveryProductLabel
): ProductLabelContent {
  return {
    name: l.product.name,
    sku: l.product.sku,
    articleCode: l.partnerArticleCode ?? undefined,
    barcode: l.partnerBarcode || l.product.barcode || undefined,
    batch: l.batch?.number,
    expiry: l.batch?.expiryDate ?? undefined,
  }
}

type LabelKind = "colli" | "product"

/** Logistik › Label Koli: pilih DO lalu cetak label koli per koli dan label produk (kode produk mitra). */
export function PackageLabelsPage({
  deliveryOrderId,
  onSelect,
}: {
  deliveryOrderId?: string
  onSelect: (id?: string) => void
}) {
  const [q, setQ] = useState("")
  const list = useQuery({
    ...deliveryOrdersQuery({ page: 1, q }),
    enabled: !deliveryOrderId,
  })
  const detail = useQuery({
    ...deliveryOrderQuery(deliveryOrderId ?? ""),
    enabled: !!deliveryOrderId,
  })
  const [kind, setKind] = useState<LabelKind>("colli")
  const d = detail.data
  return (
    <div className="flex flex-col gap-4">
      <style>{kind === "colli" ? COLLI_PRINT : PRODUCT_PRINT}</style>
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Label Koli</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Label koli 100 × 150 mm per koli (nomor DO, koli ke-n dari total,
          penerima, isi) dan label produk 50 × 30 mm yang memakai kode produk
          mitra bila ada.
        </p>
      </div>
      {!deliveryOrderId ? (
        <div className="flex flex-col gap-2">
          <Input
            aria-label="Cari Delivery Order"
            placeholder="Cari nomor DO atau penerima"
            className="min-h-11 sm:w-80"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <ul className="flex flex-col gap-2">
            {(list.data?.items ?? [])
              .filter((x) => x.number)
              .map((x) => (
                <li key={x.id}>
                  <button
                    type="button"
                    className="min-h-11 w-full rounded-md border p-3 text-left text-sm hover:bg-muted"
                    onClick={() => onSelect(x.id)}
                  >
                    <span className="font-medium">{x.number}</span> ·{" "}
                    {x.recipientName} · {formatDate(x.businessDate)}
                  </button>
                </li>
              ))}
          </ul>
        </div>
      ) : (
        <LoadingArea loading={detail.isPending}>
          {d ? (
            <LabelsForDO
              d={d}
              kind={kind}
              onKind={setKind}
              onChange={() => onSelect(undefined)}
            />
          ) : null}
        </LoadingArea>
      )}
    </div>
  )
}

function LabelsForDO({
  d,
  kind,
  onKind,
  onChange,
}: {
  d: DeliveryOrder
  kind: LabelKind
  onKind: (k: LabelKind) => void
  onChange: () => void
}) {
  const colli = colliLabels(d)
  const products = d.productLabels.flatMap((l) =>
    Array.from(
      { length: Math.min(Math.max(Math.floor(Number(l.qty)), 0), 500) },
      () => l
    )
  )
  const missing = d.productLabels.filter((l) => l.partnerCodeMissing)
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm">
          <span className="font-medium">{d.number}</span> · {d.recipientName}{" "}
          <Button variant="link" size="sm" onClick={onChange}>
            Ganti DO
          </Button>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant={kind === "colli" ? "default" : "outline"}
            className="min-h-11"
            onClick={() => onKind("colli")}
          >
            Label Koli ({colli.length})
          </Button>
          <Button
            variant={kind === "product" ? "default" : "outline"}
            className="min-h-11"
            onClick={() => onKind("product")}
          >
            Label Produk ({products.length})
          </Button>
          <Button
            className="min-h-11"
            disabled={(kind === "colli" ? colli.length : products.length) === 0}
            onClick={() => window.print()}
          >
            <PrinterIcon /> Cetak
          </Button>
        </div>
      </div>
      {kind === "colli" && colli.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Koli belum dicatat saat packing di Gudang › Outbound.
        </p>
      ) : null}
      {kind === "product" && missing.length > 0 ? (
        <p
          role="alert"
          className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950/30 dark:text-amber-200"
        >
          <AlertTriangleIcon
            className="mt-0.5 size-4 shrink-0"
            aria-hidden="true"
          />
          Kode produk mitra belum ada untuk{" "}
          {missing.map((m) => m.product.sku).join(", ")}; label memakai SKU dan
          barcode Balans. Lengkapi di Data Master › Kode Produk Mitra.
        </p>
      ) : null}
      <div className="overflow-x-auto rounded-lg border bg-muted/40 p-4">
        <div className="flex w-max flex-wrap gap-4">
          {kind === "colli"
            ? colli.map((c) => <ColliLabelView key={c.key} label={c} />)
            : products
                .slice(0, 30)
                .map((l, i) => (
                  <ProductLabelView key={i} label={partnerLabelContent(l)} />
                ))}
        </div>
      </div>
      {createPortal(
        <div id="label-print-root" className="hidden">
          <div className="block [&>*]:break-after-page">
            {kind === "colli"
              ? colli.map((c) => <ColliLabelView key={c.key} label={c} />)
              : products.map((l, i) => (
                  <ProductLabelView key={i} label={partnerLabelContent(l)} />
                ))}
          </div>
        </div>,
        document.body
      )}
    </div>
  )
}
