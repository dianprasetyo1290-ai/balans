import {
  useQuery,
  type QueryKey,
  type UseQueryOptions,
} from "@tanstack/react-query"
import { CheckIcon, ChevronsUpDownIcon, XIcon } from "lucide-react"
import { useEffect, useId, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"

export type PickerOption = {
  id: string
  label: string
  description?: string
}

type SearchPickerProps<T, TKey extends QueryKey> = {
  id?: string
  value: PickerOption | null
  onValueChange: (value: PickerOption | null) => void
  /** Query pilihan berdasarkan kata kunci (debounce 250 ms). */
  query: (q: string) => UseQueryOptions<T[], Error, T[], TKey>
  toOption: (item: T) => PickerOption
  placeholder?: string
  searchPlaceholder?: string
  emptyMessage?: string
  clearable?: boolean
  disabled?: boolean
  "aria-invalid"?: boolean
}

function useDebounced(value: string, ms = 250) {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return debounced
}

/**
 * Pemilih data master dengan pencarian di server (pelanggan, karyawan, mitra, produk, pengguna). Daftar baru
 * diambil saat popover dibuka; target sentuh minimal 44 px untuk HP.
 */
export function SearchPicker<T, TKey extends QueryKey = QueryKey>({
  id,
  value,
  onValueChange,
  query,
  toOption,
  placeholder = "Pilih data",
  searchPlaceholder = "Cari",
  emptyMessage = "Tidak ada data yang cocok.",
  clearable = false,
  disabled = false,
  ...aria
}: SearchPickerProps<T, TKey>) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState("")
  const search = useDebounced(q)
  const listId = useId()
  const options = useQuery({ ...query(search), enabled: open })
  const items = (options.data ?? []).map(toOption)

  return (
    <div className="flex gap-1">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              id={id}
              type="button"
              variant="outline"
              role="combobox"
              disabled={disabled}
              aria-expanded={open}
              aria-controls={listId}
              aria-invalid={aria["aria-invalid"]}
              className={cn(
                "min-h-11 min-w-0 flex-1 justify-between font-normal sm:min-h-9",
                !value && "text-muted-foreground"
              )}
            />
          }
        >
          <span className="truncate">
            {value
              ? value.description
                ? `${value.label} · ${value.description}`
                : value.label
              : placeholder}
          </span>
          <ChevronsUpDownIcon className="opacity-50" aria-hidden="true" />
        </PopoverTrigger>
        <PopoverContent
          className="w-(--anchor-width) min-w-72 p-2"
          align="start"
        >
          <Input
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <ul
            id={listId}
            role="listbox"
            className="mt-2 max-h-64 overflow-y-auto"
            aria-busy={options.isFetching}
          >
            {items.map((o) => (
              <li key={o.id} role="option" aria-selected={value?.id === o.id}>
                <button
                  type="button"
                  className="flex min-h-11 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                  onClick={() => {
                    onValueChange(o)
                    setOpen(false)
                  }}
                >
                  <CheckIcon
                    className={cn(
                      "size-4 shrink-0",
                      value?.id === o.id ? "opacity-100" : "opacity-0"
                    )}
                    aria-hidden="true"
                  />
                  <span className="flex min-w-0 flex-col">
                    <span className="truncate font-medium">{o.label}</span>
                    {o.description ? (
                      <span className="truncate text-xs text-muted-foreground">
                        {o.description}
                      </span>
                    ) : null}
                  </span>
                </button>
              </li>
            ))}
            {options.isError ? (
              <li className="px-2 py-3 text-sm text-destructive">
                Data tidak dapat dimuat.
              </li>
            ) : !options.isPending && items.length === 0 ? (
              <li className="px-2 py-3 text-sm text-muted-foreground">
                {emptyMessage}
              </li>
            ) : null}
          </ul>
        </PopoverContent>
      </Popover>
      {clearable && value && !disabled ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label="Kosongkan pilihan"
          onClick={() => onValueChange(null)}
        >
          <XIcon />
        </Button>
      ) : null}
    </div>
  )
}
