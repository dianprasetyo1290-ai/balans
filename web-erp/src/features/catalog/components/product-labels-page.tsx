import { useQuery } from "@tanstack/react-query"
import { PlusIcon, PrinterIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { createPortal } from "react-dom"

import { Barcode } from "@/components/common/barcode"
import { DateInput } from "@/components/common/date-input"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { formatDate } from "@/lib/format/date"
import { cn } from "@/lib/utils"

import {
  productDisplayName,
  productQuery,
  type Product,
} from "../api/catalog-api"
import { ProductPicker } from "./product-picker"

export type LabelFormat = "thermal" | "a4"

/** Batas jumlah label per baris dan total agar halaman cetak tetap ringan. */
export const MAX_LABELS_PER_ROW = 500
export const MAX_LABELS_TOTAL = 2000

type LabelRow = {
  key: string
  product: Product
  qty: number
  batch: string
  expiry: string
}

type LabelItem = Omit<LabelRow, "qty">

/** Menyusun label sesuai qty per baris, dibatasi MAX_LABELS_TOTAL. */
export function expandLabels(rows: LabelRow[]): LabelItem[] {
  const out: LabelItem[] = []
  for (const r of rows) {
    const n = Math.min(Math.max(Math.floor(r.qty) || 0, 0), MAX_LABELS_PER_ROW)
    for (let i = 0; i < n && out.length < MAX_LABELS_TOTAL; i++) {
      out.push(r)
    }
  }
  return out
}

/** CSS cetak: hanya lembar label yang dicetak, ukuran halaman sesuai format (Desain Dokumen Cetak 2.3). */
export function printStyles(format: LabelFormat) {
  const page =
    format === "thermal"
      ? "@page { size: 50mm 30mm; margin: 0; }"
      : "@page { size: A4 portrait; margin: 10mm; }"
  return `${page}
@media print {
  body > *:not(#label-print-root) { display: none !important; }
  #label-print-root { display: block !important; }
  html, body { background: #fff !important; }
}`
}

/** Satu label produk 50 × 30 mm: nama & varian, SKU, barcode utama, batch dan tanggal kedaluwarsa. */
export function ProductLabel({ item }: { item: LabelItem }) {
  const p = item.product
  return (
    <div className="flex h-[30mm] w-[50mm] [break-inside:avoid] flex-col overflow-hidden bg-white px-[2mm] py-[1.5mm] text-black">
      <div className="line-clamp-2 text-[7pt] leading-tight font-semibold">
        {productDisplayName(p)}
      </div>
      <div className="flex justify-between text-[6pt] leading-tight">
        <span>{p.sku}</span>
        {item.batch ? <span>Batch {item.batch}</span> : null}
      </div>
      {p.primaryBarcode ? (
        <Barcode
          value={p.primaryBarcode}
          symbology={p.barcodes.find((b) => b.isPrimary)?.symbology}
          className="mt-[0.5mm] min-h-0 flex-1"
        />
      ) : (
        <div className="flex-1" />
      )}
      {item.expiry ? (
        <div className="text-right text-[6pt] leading-tight">
          ED {formatDate(item.expiry)}
        </div>
      ) : null}
    </div>
  )
}

function LabelSheet({
  items,
  format,
  className,
}: {
  items: LabelItem[]
  format: LabelFormat
  className?: string
}) {
  return (
    <div
      className={cn(
        format === "thermal"
          ? "block [&>*]:break-after-page"
          : "grid grid-cols-[repeat(3,50mm)] justify-center gap-x-[10mm] gap-y-[2mm]",
        className
      )}
    >
      {items.map((item, i) => (
        <ProductLabel key={`${item.key}-${i}`} item={item} />
      ))}
    </div>
  )
}

type ProductLabelsPageProps = {
  /** Produk awal dari tombol Cetak Label Produk di daftar/detail produk. */
  productId?: string
}

/**
 * Logistik › Label Produk (S-113): pilih produk dan jumlah label, cetak ke printer label thermal 50 × 30 mm
 * atau kertas A4 berisi banyak label. Barcode dirender di browser (EAN-13 atau Code 128) dan dicetak lewat
 * dialog cetak browser.
 */
export function ProductLabelsPage({ productId }: ProductLabelsPageProps) {
  const initial = useQuery({
    ...productQuery(productId ?? ""),
    enabled: !!productId,
  })
  const [rows, setRows] = useState<LabelRow[]>([])
  const [initialized, setInitialized] = useState(false)
  const [format, setFormat] = useState<LabelFormat>("thermal")

  // Produk awal dimasukkan sekali saat datanya tersedia (dihitung saat render, bukan di effect).
  if (!initialized && initial.data) {
    setInitialized(true)
    if (initial.data.primaryBarcode && !initial.data.deletedAt) {
      setRows([
        {
          key: crypto.randomUUID(),
          product: initial.data,
          qty: 1,
          batch: "",
          expiry: "",
        },
      ])
    }
  }

  const items = expandLabels(rows)
  const update = (key: string, patch: Partial<LabelRow>) =>
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)))

  return (
    <div className="flex flex-col gap-4">
      <style>{printStyles(format)}</style>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Label Produk
          </h1>
          <p className="text-sm text-muted-foreground">
            Cetak label barcode untuk ditempel ke kemasan. Label memuat nama
            produk, SKU, barcode utama, serta batch dan tanggal kedaluwarsa jika
            diisi.
          </p>
        </div>
        <Button disabled={items.length === 0} onClick={() => window.print()}>
          <PrinterIcon /> Cetak{" "}
          {items.length > 0 ? `${items.length} Label` : "Label"}
        </Button>
      </div>

      <div
        role="radiogroup"
        aria-label="Format kertas"
        className="flex flex-wrap gap-2"
      >
        {(
          [
            ["thermal", "Printer label thermal 50 × 30 mm"],
            ["a4", "Kertas A4 (banyak label)"],
          ] as const
        ).map(([value, label]) => (
          <label
            key={value}
            className="flex min-h-11 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm has-checked:border-primary/40 has-checked:bg-primary/5"
          >
            <input
              type="radio"
              name="label-format"
              className="size-4 accent-(--color-primary)"
              checked={format === value}
              onChange={() => setFormat(value)}
            />
            {label}
          </label>
        ))}
      </div>

      <div className="flex flex-col gap-3">
        {rows.map((r, i) => (
          <div
            key={r.key}
            className="grid gap-2 rounded-lg border p-3 sm:grid-cols-[1fr_6rem_9rem_11rem_auto] sm:items-end"
          >
            <div className="flex flex-col gap-1">
              <span className="text-xs text-muted-foreground">Produk</span>
              <span className="text-sm font-medium">
                {r.product.sku} · {productDisplayName(r.product)}
              </span>
              <span className="font-mono text-xs text-muted-foreground">
                {r.product.primaryBarcode}
              </span>
            </div>
            <div className="flex flex-col gap-1 text-xs text-muted-foreground">
              <label htmlFor={`label-qty-${r.key}`}>Jumlah</label>
              <Input
                id={`label-qty-${r.key}`}
                type="number"
                inputMode="numeric"
                min={1}
                max={MAX_LABELS_PER_ROW}
                aria-label={`Jumlah label baris ${i + 1}`}
                value={String(r.qty)}
                onChange={(e) => update(r.key, { qty: Number(e.target.value) })}
              />
            </div>
            <div className="flex flex-col gap-1 text-xs text-muted-foreground">
              <label htmlFor={`label-batch-${r.key}`}>Batch (opsional)</label>
              <Input
                id={`label-batch-${r.key}`}
                aria-label={`Batch baris ${i + 1}`}
                value={r.batch}
                onChange={(e) => update(r.key, { batch: e.target.value })}
              />
            </div>
            <div className="flex flex-col gap-1 text-xs text-muted-foreground">
              <span>Kedaluwarsa (opsional)</span>
              <DateInput
                value={r.expiry}
                onValueChange={(expiry) => update(r.key, { expiry })}
                placeholder="Pilih tanggal"
              />
            </div>
            <Button
              variant="ghost"
              size="icon"
              aria-label={`Hapus baris ${i + 1}`}
              onClick={() => setRows((rs) => rs.filter((x) => x.key !== r.key))}
            >
              <Trash2Icon />
            </Button>
          </div>
        ))}
        <div className="flex max-w-md flex-col gap-1">
          <span className="text-sm font-medium">
            <PlusIcon className="mr-1 inline size-4" aria-hidden="true" />
            Tambah produk
          </span>
          <ProductPicker
            value={null}
            placeholder="Cari Produk yang punya barcode"
            excludeIds={rows.map((r) => r.product.id)}
            onValueChange={(p) => {
              if (!p.primaryBarcode) return
              setRows((rs) => [
                ...rs,
                {
                  key: crypto.randomUUID(),
                  product: p,
                  qty: 1,
                  batch: "",
                  expiry: "",
                },
              ])
            }}
          />
          <span className="text-xs text-muted-foreground">
            Produk tanpa barcode utama tidak bisa dicetak; isi barcode di Data
            Master › Produk.
          </span>
        </div>
      </div>

      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">
          Pratinjau ({items.length} label
          {items.length >= MAX_LABELS_TOTAL
            ? `, maksimal ${MAX_LABELS_TOTAL}`
            : ""}
          )
        </h2>
        {items.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum ada label. Tambahkan produk untuk mencetak label.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border bg-muted/40 p-4">
            <LabelSheet
              items={items.slice(0, 30)}
              format="a4"
              className="w-max justify-start [&>*]:shadow-sm"
            />
            {items.length > 30 ? (
              <p className="mt-2 text-xs text-muted-foreground">
                Menampilkan 30 label pertama dari {items.length}.
              </p>
            ) : null}
          </div>
        )}
      </section>

      {createPortal(
        <div id="label-print-root" className="hidden">
          <LabelSheet items={items} format={format} />
        </div>,
        document.body
      )}
    </div>
  )
}
