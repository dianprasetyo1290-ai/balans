import { useQueryClient } from "@tanstack/react-query"
import { MoreHorizontalIcon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteDimension,
  DIMENSION_LABELS,
  type Dimension,
} from "../api/accounting-api"

export type DimensionColumn = {
  header: string
  cell: (d: Dimension) => ReactNode
}

type DimensionTableProps = {
  rows: Dimension[] | undefined
  loading: boolean
  canWrite: boolean
  extraColumns?: DimensionColumn[]
  onEdit: (d: Dimension) => void
  emptyMessage: string
}

/** Tabel master dimensi dengan aksi Ubah dan Hapus (soft delete). */
export function DimensionTable({
  rows,
  loading,
  canWrite,
  extraColumns = [],
  onEdit,
  emptyMessage,
}: DimensionTableProps) {
  const queryClient = useQueryClient()
  const [deleting, setDeleting] = useState<Dimension | null>(null)
  const remove = useIdempotentMutation({
    mutationFn: (d: Dimension, key) => deleteDimension(d, key),
    onSuccess: async (_, d) => {
      toast.success(`${DIMENSION_LABELS[d.kind]} dihapus`)
      setDeleting(null)
      await queryClient.invalidateQueries({ queryKey: ["accounting"] })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <LoadingArea loading={loading}>
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Kode</TableHead>
              <TableHead>Nama</TableHead>
              {extraColumns.map((c) => (
                <TableHead key={c.header}>{c.header}</TableHead>
              ))}
              <TableHead>Status</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {(rows ?? []).map((d) => (
              <TableRow
                key={d.id}
                className={cn(!d.isActive && "text-muted-foreground")}
              >
                <TableCell className="font-medium whitespace-nowrap">
                  {d.code}
                </TableCell>
                <TableCell>{d.name}</TableCell>
                {extraColumns.map((c) => (
                  <TableCell key={c.header}>{c.cell(d)}</TableCell>
                ))}
                <TableCell>
                  {d.isActive ? (
                    <Badge variant="secondary">Aktif</Badge>
                  ) : (
                    <Badge variant="outline">Nonaktif</Badge>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  {canWrite ? (
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <Button
                            variant="ghost"
                            size="icon"
                            aria-label={`Aksi untuk ${d.code}`}
                          />
                        }
                      >
                        <MoreHorizontalIcon />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => onEdit(d)}>
                          Ubah
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => setDeleting(d)}
                        >
                          Hapus
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
            {rows && rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={4 + extraColumns.length}
                  className="text-center text-muted-foreground"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </div>
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setDeleting(null)}
          title={`Hapus ${DIMENSION_LABELS[deleting.kind]} ${deleting.code}?`}
          description="Data yang sudah dipakai jurnal tidak bisa dihapus; nonaktifkan bila tidak dipakai lagi. Kode tidak bisa dipakai ulang."
          confirmLabel={`Hapus ${DIMENSION_LABELS[deleting.kind]}`}
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </LoadingArea>
  )
}
