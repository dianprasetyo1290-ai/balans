import { useMemo } from "react"

import {
  barcodeModules,
  barRuns,
  detectSymbology,
  QUIET_ZONE,
  type Symbology,
} from "@/lib/barcode/barcode"
import { cn } from "@/lib/utils"

type BarcodeProps = {
  value: string
  /** Kosong = dideteksi otomatis (EAN-13 jika valid, selain itu Code 128). */
  symbology?: Symbology
  /** Tampilkan teks barcode di bawah batang. */
  showText?: boolean
  className?: string
}

/**
 * Barcode SVG untuk label cetak. Lebar mengikuti kontainer (batang diskalakan rata), sehingga ukuran label
 * diatur lewat CSS mm di halaman cetak.
 */
export function Barcode({
  value,
  symbology,
  showText = true,
  className,
}: BarcodeProps) {
  const sym = symbology ?? detectSymbology(value)
  const result = useMemo(() => {
    try {
      const modules = barcodeModules(value, sym)
      return { runs: barRuns(modules), modules: modules.length, ok: true }
    } catch {
      return { runs: [], modules: 0, ok: false }
    }
  }, [value, sym])

  if (!result.ok) {
    return (
      <span className={cn("text-xs text-destructive", className)}>
        Barcode tidak valid
      </span>
    )
  }
  const width = result.modules + QUIET_ZONE * 2
  return (
    <div className={cn("flex flex-col items-center", className)}>
      <svg
        role="img"
        aria-label={`Barcode ${value}`}
        viewBox={`0 0 ${width} 40`}
        preserveAspectRatio="none"
        className="block h-full min-h-0 w-full flex-1"
        shapeRendering="crispEdges"
      >
        <rect width={width} height={40} fill="#fff" />
        {result.runs.map(([x, w]) => (
          <rect key={x} x={x + QUIET_ZONE} width={w} height={40} fill="#000" />
        ))}
      </svg>
      {showText ? (
        <span className="font-mono text-[7pt] leading-tight tracking-wider text-black">
          {value}
        </span>
      ) : null}
    </div>
  )
}
