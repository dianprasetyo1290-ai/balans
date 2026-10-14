import { ArrowDownIcon, ArrowUpDownIcon, ArrowUpIcon } from "lucide-react"
import type { ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { useDelayedVisibility } from "@/hooks/use-delayed-visibility"
import { cn } from "@/lib/utils"

import { CircularProgress } from "./circular-progress"
import { LoadingArea } from "./loading-area"

export type DataTableColumn<T> = {
  id: string
  header: string
  cell: (row: T) => ReactNode
  /** Nama field sort di API; kolom tanpa sortKey tidak bisa diurutkan. */
  sortKey?: string
  align?: "left" | "right"
}

type DataTableProps<T> = {
  columns: DataTableColumn<T>[]
  rows: T[] | undefined
  getRowId: (row: T) => string
  loading: boolean
  /** Refetch di latar: data lama tetap tampil dengan indikator kecil di header. */
  fetching?: boolean
  page: number
  totalPages: number
  totalItems: number
  /** Format API: `-createdAt` (minus = descending). */
  sort?: string
  onSortChange: (sort: string) => void
  onPageChange: (page: number) => void
  emptyMessage?: string
  caption?: string
}

function nextSort(current: string | undefined, key: string): string {
  if (current === key) {
    return `-${key}`
  }
  return key
}

/**
 * Tabel list standar. Filter, sort, dan halaman disimpan di URL oleh route pemakai
 * (Technical Guideline 10.2); komponen ini hanya memanggil callback.
 */
export function DataTable<T>({
  columns,
  rows,
  getRowId,
  loading,
  fetching = false,
  page,
  totalPages,
  totalItems,
  sort,
  onSortChange,
  onPageChange,
  emptyMessage = "Belum ada data.",
  caption,
}: DataTableProps<T>) {
  const showRefetch = useDelayedVisibility(fetching && !loading)

  return (
    <div className="flex flex-col gap-3">
      <div className="flex min-h-5 items-center justify-between text-sm text-muted-foreground">
        <span>{caption}</span>
        {showRefetch ? <CircularProgress size="sm" label="Memperbarui data" /> : null}
      </div>
      <LoadingArea loading={loading}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                {columns.map((col) => {
                  const active = sort === col.sortKey || sort === `-${col.sortKey}`
                  const descending = sort === `-${col.sortKey}`
                  return (
                    <TableHead
                      key={col.id}
                      className={cn(col.align === "right" && "text-right")}
                      aria-sort={active ? (descending ? "descending" : "ascending") : undefined}
                    >
                      {col.sortKey ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="-ml-2"
                          onClick={() => col.sortKey && onSortChange(nextSort(sort, col.sortKey))}
                        >
                          {col.header}
                          {active ? (
                            descending ? <ArrowDownIcon aria-hidden="true" /> : <ArrowUpIcon aria-hidden="true" />
                          ) : (
                            <ArrowUpDownIcon aria-hidden="true" className="opacity-40" />
                          )}
                        </Button>
                      ) : (
                        col.header
                      )}
                    </TableHead>
                  )
                })}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows && rows.length > 0 ? (
                rows.map((row) => (
                  <TableRow key={getRowId(row)}>
                    {columns.map((col) => (
                      <TableCell key={col.id} className={cn(col.align === "right" && "text-right tabular-nums")}>
                        {col.cell(row)}
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={columns.length} className="h-24 text-center text-muted-foreground">
                    {emptyMessage}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <nav className="flex items-center justify-between gap-2 text-sm" aria-label="Halaman tabel">
        <span className="text-muted-foreground">
          {totalItems.toLocaleString("id-ID")} data · Halaman {Math.min(page, Math.max(totalPages, 1))} dari {Math.max(totalPages, 1)}
        </span>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" disabled={page <= 1 || loading} onClick={() => onPageChange(page - 1)}>
            Sebelumnya
          </Button>
          <Button variant="outline" size="sm" disabled={page >= totalPages || loading} onClick={() => onPageChange(page + 1)}>
            Berikutnya
          </Button>
        </div>
      </nav>
    </div>
  )
}
