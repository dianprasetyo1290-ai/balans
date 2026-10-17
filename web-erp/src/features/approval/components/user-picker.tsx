import { useQuery } from "@tanstack/react-query"
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

import { userOptionsQuery, type ApprovalUserOption } from "../api/approval-api"

type UserPickerProps = {
  id?: string
  value: ApprovalUserOption | null
  onValueChange: (value: ApprovalUserOption | null) => void
  placeholder?: string
  /** User yang tidak boleh dipilih, mis. diri sendiri sebagai penerima delegasi. */
  excludeIds?: string[]
  clearable?: boolean
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

/** Pemilih pengguna aktif dengan pencarian nama/email (penerima delegasi, approver, pengganti). */
export function UserPicker({
  id,
  value,
  onValueChange,
  placeholder = "Pilih pengguna",
  excludeIds = [],
  clearable = false,
  ...aria
}: UserPickerProps) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState("")
  const search = useDebounced(q)
  const listId = useId()
  const options = useQuery({ ...userOptionsQuery(search), enabled: open })
  const items = (options.data ?? []).filter((u) => !excludeIds.includes(u.id))

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
              aria-expanded={open}
              aria-controls={listId}
              aria-invalid={aria["aria-invalid"]}
              className={cn(
                "min-w-0 flex-1 justify-between font-normal",
                !value && "text-muted-foreground"
              )}
            />
          }
        >
          <span className="truncate">
            {value ? `${value.fullName} · ${value.email}` : placeholder}
          </span>
          <ChevronsUpDownIcon className="opacity-50" aria-hidden="true" />
        </PopoverTrigger>
        <PopoverContent
          className="w-(--anchor-width) min-w-72 p-2"
          align="start"
        >
          <Input
            placeholder="Cari nama atau email"
            aria-label="Cari pengguna"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
          <ul
            id={listId}
            role="listbox"
            className="mt-2 max-h-64 overflow-y-auto"
            aria-busy={options.isFetching}
          >
            {items.map((u) => (
              <li key={u.id} role="option" aria-selected={value?.id === u.id}>
                <button
                  type="button"
                  className="flex min-h-11 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                  onClick={() => {
                    onValueChange(u)
                    setOpen(false)
                  }}
                >
                  <CheckIcon
                    className={cn(
                      "size-4 shrink-0",
                      value?.id === u.id ? "opacity-100" : "opacity-0"
                    )}
                    aria-hidden="true"
                  />
                  <span className="flex min-w-0 flex-col">
                    <span className="truncate font-medium">{u.fullName}</span>
                    <span className="truncate text-xs text-muted-foreground">
                      {u.email}
                    </span>
                  </span>
                </button>
              </li>
            ))}
            {!options.isPending && items.length === 0 ? (
              <li className="px-2 py-3 text-sm text-muted-foreground">
                Tidak ada pengguna yang cocok.
              </li>
            ) : null}
          </ul>
        </PopoverContent>
      </Popover>
      {clearable && value ? (
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
