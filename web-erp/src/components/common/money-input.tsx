import { useState, type ComponentProps } from "react"

import { Input } from "@/components/ui/input"
import { formatMoneyInput, parseMoneyInput } from "@/lib/format/money"

type MoneyInputProps = Omit<ComponentProps<typeof Input>, "value" | "onChange" | "type" | "inputMode"> & {
  /** String desimal kontrak API, mis. "1250000.00". Kosong = belum diisi. */
  value: string
  onValueChange: (value: string) => void
}

/**
 * Input uang dengan format `1.250.000`. Nilai yang dikirim ke form adalah string desimal,
 * bukan number JavaScript (Technical Guideline 10.5).
 */
export function MoneyInput({ value, onValueChange, onBlur, onFocus, ...props }: MoneyInputProps) {
  const [draft, setDraft] = useState<string | null>(null)

  return (
    <div className="relative">
      <span className="pointer-events-none absolute inset-y-0 left-3 flex items-center text-sm text-muted-foreground">Rp</span>
      <Input
        {...props}
        type="text"
        inputMode="numeric"
        className="pl-9 text-right tabular-nums"
        value={draft ?? formatMoneyInput(value)}
        onFocus={(e) => {
          setDraft(formatMoneyInput(value))
          onFocus?.(e)
        }}
        onChange={(e) => {
          const raw = e.target.value.replace(/[^\d]/g, "")
          setDraft(raw === "" ? "" : formatMoneyInput(raw))
          onValueChange(parseMoneyInput(raw) ?? "")
        }}
        onBlur={(e) => {
          setDraft(null)
          onBlur?.(e)
        }}
      />
    </div>
  )
}
