import { useQuery, useQueryClient } from "@tanstack/react-query"
import { MoreHorizontalIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteLocation,
  locationsQuery,
  PARTY_TYPE_LABELS,
  restoreLocation,
  type Location,
  type LocationListSearch,
} from "../api/warehouse-api"
import { LocationFormDialog } from "./location-form-dialog"

type LocationsTableProps = {
  search: LocationListSearch
  onPageChange: (page: number) => void
  emptyMessage: string
}

type Pending = { kind: "delete" | "restore"; location: Location }

/** Daftar lokasi stok (lokasi gudang atau lokasi eksternal) dengan aksi ubah, hapus, dan pulihkan. */
export function LocationsTable({
  search,
  onPageChange,
  emptyMessage,
}: LocationsTableProps) {
  const queryClient = useQueryClient()
  const locations = useQuery(locationsQuery(search))
  const [editing, setEditing] = useState<Location | null>(null)
  const [pending, setPending] = useState<Pending | null>(null)

  const action = useIdempotentMutation({
    mutationFn: (p: Pending, key) =>
      p.kind === "delete"
        ? deleteLocation(p.location.id, key)
        : restoreLocation(p.location.id, key),
    onSuccess: async (_, p) => {
      toast.success(
        p.kind === "delete" ? "Lokasi dihapus" : "Lokasi dipulihkan"
      )
      setPending(null)
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
    },
    onError: (err) => {
      setPending(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<Location>[] = [
    {
      id: "code",
      header: "Kode",
      cell: (l) => (
        <span className="font-medium whitespace-nowrap">{l.code}</span>
      ),
    },
    {
      id: "name",
      header: "Nama",
      cell: (l) => (
        <div className="flex min-w-40 flex-col">
          <span>{l.name}</span>
          {l.isStandard ? (
            <span className="text-xs text-muted-foreground">
              Lokasi standar
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (l) => <Badge variant="secondary">{l.locationTypeLabel}</Badge>,
    },
    {
      id: "owner",
      header: search.external ? "Pihak" : "Gudang",
      cell: (l) =>
        l.isExternal ? (
          <div className="flex flex-col">
            <span className="text-xs">
              {l.externalPartyType
                ? PARTY_TYPE_LABELS[l.externalPartyType]
                : "—"}
            </span>
            <span className="font-mono text-[11px] text-muted-foreground">
              {l.externalPartyId}
            </span>
          </div>
        ) : (
          (l.warehouseName ?? "—")
        ),
    },
    {
      id: "sellable",
      header: "Bisa dijual",
      cell: (l) => (l.isSellable ? "Ya" : "Tidak"),
    },
    {
      id: "status",
      header: "Status",
      cell: (l) => (
        <RecordStatusBadge isActive={l.isActive} deletedAt={l.deletedAt} />
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (l) =>
        l.allowedActions.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Aksi untuk ${l.code}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {l.allowedActions.includes("update") ? (
                <DropdownMenuItem onClick={() => setEditing(l)}>
                  Ubah
                </DropdownMenuItem>
              ) : null}
              {l.allowedActions.includes("restore") ? (
                <DropdownMenuItem
                  onClick={() => setPending({ kind: "restore", location: l })}
                >
                  Pulihkan
                </DropdownMenuItem>
              ) : null}
              {l.allowedActions.includes("delete") ? (
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setPending({ kind: "delete", location: l })}
                >
                  Hapus
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null,
    },
  ]

  return (
    <>
      <DataTable
        columns={columns}
        rows={locations.data?.items}
        getRowId={(l) => l.id}
        loading={locations.isPending}
        fetching={locations.isFetching}
        page={search.page}
        totalPages={locations.data?.totalPages ?? 0}
        totalItems={locations.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={onPageChange}
        emptyMessage={emptyMessage}
      />
      <LocationFormDialog
        open={!!editing}
        onOpenChange={(open) => !open && setEditing(null)}
        location={editing ?? undefined}
      />
      {pending ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setPending(null)}
          title={
            pending.kind === "delete"
              ? `Hapus lokasi ${pending.location.code}?`
              : `Pulihkan lokasi ${pending.location.code}?`
          }
          description={
            pending.kind === "delete"
              ? "Lokasi tidak bisa dipilih di transaksi baru, tetapi tetap tampil di dokumen lama. Kode tidak bisa dipakai ulang."
              : "Lokasi kembali bisa dipilih di transaksi baru."
          }
          confirmLabel={
            pending.kind === "delete" ? "Hapus Lokasi" : "Pulihkan Lokasi"
          }
          destructive={pending.kind === "delete"}
          loading={action.isPending}
          onConfirm={() => action.mutate(pending)}
        />
      ) : null}
    </>
  )
}
