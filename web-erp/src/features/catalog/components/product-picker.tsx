import { useQuery } from "@tanstack/react-query"
import { CheckIcon, ChevronsUpDownIcon } from "lucide-react"
import { useEffect, useId, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { cn } from "@/lib/utils"

import {
  productDisplayName,
  productOptionsQuery,
  type Product,
  type ProductType,
} from "../api/catalog-api"

export type ProductOption = Pick<
  Product,
  "id" | "sku" | "name" | "variant" | "uom"
> &
  Partial<Pick<Product, "primaryBarcode">>

type ProductPickerProps = {
  id?: string
  value: ProductOption | null
  onValueChange: (value: Product) => void
  placeholder?: string
  types?: ProductType[]
  isBundle?: boolean
  /** Produk yang tidak boleh dipilih, mis. komponen yang sudah ada di bundel. */
  excludeIds?: string[]
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

/** Pemilih produk aktif dengan pencarian SKU, nama, atau barcode (Cari Produk). */
export function ProductPicker({
  id,
  value,
  onValueChange,
  placeholder = "Pilih produk",
  types,
  isBundle,
  excludeIds = [],
  ...aria
}: ProductPickerProps) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState("")
  const search = useDebounced(q)
  const listId = useId()
  const options = useQuery({
    ...productOptionsQuery(search, { types, isBundle }),
    enabled: open,
  })
  const items = (options.data?.items ?? []).filter(
    (p) => !excludeIds.includes(p.id)
  )

  return (
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
              "w-full min-w-0 justify-between font-normal",
              !value && "text-muted-foreground"
            )}
          />
        }
      >
        <span className="truncate">
          {value ? `${value.sku} · ${productDisplayName(value)}` : placeholder}
        </span>
        <ChevronsUpDownIcon className="opacity-50" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent className="w-(--anchor-width) min-w-72 p-2" align="start">
        <Input
          placeholder="Cari SKU, nama, atau barcode"
          aria-label="Cari Produk"
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <ul
          id={listId}
          role="listbox"
          className="mt-2 max-h-64 overflow-y-auto"
          aria-busy={options.isFetching}
        >
          {items.map((p) => (
            <li key={p.id} role="option" aria-selected={value?.id === p.id}>
              <button
                type="button"
                className="flex min-h-11 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
                onClick={() => {
                  onValueChange(p)
                  setOpen(false)
                }}
              >
                <CheckIcon
                  className={cn(
                    "size-4 shrink-0",
                    value?.id === p.id ? "opacity-100" : "opacity-0"
                  )}
                  aria-hidden="true"
                />
                <span className="flex min-w-0 flex-col">
                  <span className="truncate font-medium">
                    {productDisplayName(p)}
                  </span>
                  <span className="truncate text-xs text-muted-foreground">
                    {p.sku}
                    {p.primaryBarcode ? ` · ${p.primaryBarcode}` : ""}
                  </span>
                </span>
              </button>
            </li>
          ))}
          {!options.isPending && items.length === 0 ? (
            <li className="px-2 py-3 text-sm text-muted-foreground">
              Tidak ada produk yang cocok.
            </li>
          ) : null}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
