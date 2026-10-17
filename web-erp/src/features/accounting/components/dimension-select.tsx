import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import type { Dimension } from "../api/accounting-api"

const NONE = "none"

type DimensionSelectProps = {
  id?: string
  value: string | null | undefined
  onValueChange: (value: string | null) => void
  options: Dimension[]
  placeholder: string
  disabled?: boolean
  "aria-invalid"?: boolean
  "aria-label"?: string
}

/** Pilihan dimensi jurnal (channel, sub-channel, divisi, kampanye) dengan opsi kosong. */
export function DimensionSelect({
  id,
  value,
  onValueChange,
  options,
  placeholder,
  disabled,
  ...aria
}: DimensionSelectProps) {
  const label = (v: string) => {
    const o = options.find((d) => d.id === v)
    return o ? (o.kind === "campaign" ? o.code : o.name) : placeholder
  }
  return (
    <Select
      value={value ?? NONE}
      disabled={disabled}
      onValueChange={(v) => onValueChange(!v || v === NONE ? null : String(v))}
    >
      <SelectTrigger
        id={id}
        className="w-full min-w-32"
        aria-invalid={aria["aria-invalid"]}
        aria-label={aria["aria-label"]}
      >
        <SelectValue>
          {(v: string) => (v === NONE ? placeholder : label(v))}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NONE}>{placeholder}</SelectItem>
        {options.map((d) => (
          <SelectItem key={d.id} value={d.id}>
            {d.kind === "campaign" ? `${d.code} — ${d.name}` : d.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
