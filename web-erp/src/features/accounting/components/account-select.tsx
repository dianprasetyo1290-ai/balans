import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

import type { Account } from "../api/accounting-api"

type AccountSelectProps = {
  id?: string
  value: string
  onValueChange: (value: string) => void
  accounts: Account[]
  placeholder?: string
  "aria-invalid"?: boolean
}

/** Pilihan akun COA (kode + nama) dari daftar yang sudah difilter pemanggil. */
export function AccountSelect({
  id,
  value,
  onValueChange,
  accounts,
  placeholder = "Pilih akun",
  ...aria
}: AccountSelectProps) {
  const label = (v: string) => {
    const a = accounts.find((x) => x.id === v)
    return a ? `${a.code} ${a.name}` : placeholder
  }
  return (
    <Select
      value={value || null}
      onValueChange={(v) => onValueChange(v ? String(v) : "")}
    >
      <SelectTrigger
        id={id}
        className="w-full"
        aria-invalid={aria["aria-invalid"]}
      >
        <SelectValue>
          {(v: string | null) => (v ? label(v) : placeholder)}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {accounts.map((a) => (
          <SelectItem key={a.id} value={a.id}>
            {a.code} {a.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
