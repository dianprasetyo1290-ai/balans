import { useState, type ComponentProps } from "react"

import { Input } from "@/components/ui/input"
import { formatQuantity, parseQuantityInput } from "@/lib/format/number"

type QuantityInputProps = Omit<ComponentProps<typeof Input>, "value" | "onChange" | "type" | "inputMode"> & {
  /** String desimal kontrak API (maks. 3 desimal). Kosong = belum diisi. */
  value: string
  onValueChange: (value: string) => void
  /** Satuan yang ditampilkan di kanan, mis. "pcs" atau "kg". */
  unit?: string
}

/** Input kuantitas dengan format Indonesia (koma desimal, titik ribuan). */
export function QuantityInput({ value, onValueChange, unit, onFocus, onBlur, ...props }: QuantityInputProps) {
  const [draft, setDraft] = useState<string | null>(null)
  const display = value === "" ? "" : formatQuantity(value)

  return (
    <div className="relative">
      <Input
        {...props}
        type="text"
        inputMode="decimal"
        className={unit ? "pr-12 text-right tabular-nums" : "text-right tabular-nums"}
        value={draft ?? display}
        onFocus={(e) => {
          setDraft(display)
          onFocus?.(e)
        }}
        onChange={(e) => {
          const raw = e.target.value.replace(/[^\d.,]/g, "")
          setDraft(raw)
          onValueChange(parseQuantityInput(raw) ?? "")
        }}
        onBlur={(e) => {
          setDraft(null)
          onBlur?.(e)
        }}
      />
      {unit ? (
        <span className="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-muted-foreground">{unit}</span>
      ) : null}
    </div>
  )
}
