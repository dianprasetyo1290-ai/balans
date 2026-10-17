import { SearchIcon } from "lucide-react"
import { useState } from "react"

import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { statusLabel } from "@/lib/format/status-labels"

import { DateRange } from "./request-list-page"

const ALL = "all"

type Props<S extends string> = {
  searchLabel: string
  searchPlaceholder: string
  q?: string
  status?: S
  statuses: S[]
  dateFrom?: string
  dateTo?: string
  onSearch: (q: string | undefined) => void
  onStatus: (status: S | undefined) => void
  onDates: (from: string | undefined, to: string | undefined) => void
}

/** Baris filter daftar dokumen pengadaan: cari, status, rentang tanggal (disimpan di URL oleh halaman). */
export function DocumentToolbar<S extends string>(props: Props<S>) {
  const [q, setQ] = useState(props.q ?? "")
  return (
    <div className="flex flex-wrap items-center gap-3">
      <form
        className="relative w-full sm:w-72"
        onSubmit={(e) => {
          e.preventDefault()
          props.onSearch(q || undefined)
        }}
      >
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          aria-label={props.searchLabel}
          placeholder={props.searchPlaceholder}
          className="pl-8"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
      </form>
      <Select
        value={props.status ?? ALL}
        onValueChange={(v) =>
          props.onStatus(v === ALL || !v ? undefined : (v as S))
        }
      >
        <SelectTrigger className="w-48" aria-label="Filter status">
          <SelectValue>
            {(v: string) => (v === ALL ? "Semua status" : statusLabel(v).label)}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Semua status</SelectItem>
          {props.statuses.map((s) => (
            <SelectItem key={s} value={s}>
              {statusLabel(s).label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <DateRange
        from={props.dateFrom}
        to={props.dateTo}
        onChange={props.onDates}
      />
    </div>
  )
}
