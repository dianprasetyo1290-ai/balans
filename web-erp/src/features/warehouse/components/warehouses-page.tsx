import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { PageTabs } from "@/components/common/page-tabs"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteWarehouse,
  restoreWarehouse,
  warehousesQuery,
  type Warehouse,
} from "../api/warehouse-api"
import { LocationFormDialog } from "./location-form-dialog"
import { LocationsTable } from "./locations-table"
import { WarehouseFormDialog } from "./warehouse-form-dialog"

export type WarehousesSearch = {
  tab: "warehouses" | "external"
  page: number
  sort?: string
  q?: string
  deleted?: boolean
}

type WarehousesPageProps = {
  search: WarehousesSearch
  onSearchChange: (patch: Partial<WarehousesSearch>) => void
}

type Pending = { kind: "delete" | "restore"; warehouse: Warehouse }

/** Data Master › Gudang & Lokasi (S-123): gudang fisik dan lokasi eksternal per pihak. */
export function WarehousesPage({
  search,
  onSearchChange,
}: WarehousesPageProps) {
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const warehouses = useQuery({
    ...warehousesQuery(search),
    enabled: search.tab === "warehouses",
  })
  const [q, setQ] = useState(search.q ?? "")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Warehouse | undefined>()
  const [locationOpen, setLocationOpen] = useState(false)
  const [pending, setPending] = useState<Pending | null>(null)
  const canWriteWarehouse = can("warehouse.warehouse.write")
  const canWriteLocation = can("warehouse.location.write")

  const action = useIdempotentMutation({
    mutationFn: (p: Pending, key) =>
      p.kind === "delete"
        ? deleteWarehouse(p.warehouse.id, key)
        : restoreWarehouse(p.warehouse.id, key),
    onSuccess: async (_, p) => {
      toast.success(
        p.kind === "delete"
          ? "Gudang dan lokasinya dihapus"
          : "Gudang dan lokasinya dipulihkan"
      )
      setPending(null)
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
    },
    onError: (err) => {
      setPending(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<Warehouse>[] = [
    {
      id: "code",
      header: "Kode",
      sortKey: "code",
      cell: (w) => (
        <Link
          to="/master-data/warehouses/$id"
          params={{ id: w.id }}
          className="font-medium text-primary underline-offset-4 hover:underline"
        >
          {w.code}
        </Link>
      ),
    },
    { id: "name", header: "Nama", sortKey: "name", cell: (w) => w.name },
    {
      id: "address",
      header: "Alamat",
      cell: (w) => (
        <span className="line-clamp-2 min-w-48 text-sm">
          {w.address || "—"}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (w) => (
        <RecordStatusBadge isActive={w.isActive} deletedAt={w.deletedAt} />
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (w) =>
        w.allowedActions.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Aksi untuk ${w.code}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {w.allowedActions.includes("update") ? (
                <DropdownMenuItem
                  onClick={() => {
                    setEditing(w)
                    setFormOpen(true)
                  }}
                >
                  Ubah
                </DropdownMenuItem>
              ) : null}
              {w.allowedActions.includes("restore") ? (
                <DropdownMenuItem
                  onClick={() => setPending({ kind: "restore", warehouse: w })}
                >
                  Pulihkan
                </DropdownMenuItem>
              ) : null}
              {w.allowedActions.includes("delete") ? (
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setPending({ kind: "delete", warehouse: w })}
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
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Gudang & Lokasi
          </h1>
          <p className="text-sm text-muted-foreground">
            Gudang fisik beserta lokasi standarnya, dan lokasi eksternal di
            maklon, mitra konsinyasi, buffer vending, dan mesin vending.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons kind="warehouse.location" />
          {search.tab === "warehouses" && canWriteWarehouse ? (
            <Button
              onClick={() => {
                setEditing(undefined)
                setFormOpen(true)
              }}
            >
              <PlusIcon /> Tambah Gudang
            </Button>
          ) : null}
          {search.tab === "external" && canWriteLocation ? (
            <Button onClick={() => setLocationOpen(true)}>
              <PlusIcon /> Tambah Lokasi Eksternal
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian gudang dan lokasi"
        value={search.tab}
        onValueChange={(tab) => {
          setQ("")
          onSearchChange({ tab, page: 1, q: undefined, sort: undefined })
        }}
        tabs={[
          { id: "warehouses", label: "Gudang" },
          { id: "external", label: "Lokasi Eksternal" },
        ]}
      />
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="flex w-full max-w-md gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange({ q: q || undefined, page: 1 })
          }}
        >
          <Input
            placeholder="Cari kode atau nama"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label={
              search.tab === "warehouses" ? "Cari gudang" : "Cari lokasi"
            }
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4 accent-(--color-primary)"
            checked={!!search.deleted}
            onChange={(e) =>
              onSearchChange({
                deleted: e.target.checked || undefined,
                page: 1,
              })
            }
          />
          Tampilkan yang dihapus
        </label>
      </div>
      {search.tab === "warehouses" ? (
        <DataTable
          columns={columns}
          rows={warehouses.data?.items}
          getRowId={(w) => w.id}
          loading={warehouses.isPending}
          fetching={warehouses.isFetching}
          page={search.page}
          totalPages={warehouses.data?.totalPages ?? 0}
          totalItems={warehouses.data?.totalItems ?? 0}
          sort={search.sort}
          onSortChange={(sort) => onSearchChange({ sort, page: 1 })}
          onPageChange={(page) => onSearchChange({ page })}
          emptyMessage={
            search.deleted
              ? "Tidak ada gudang yang dihapus."
              : "Belum ada gudang. Tambah Gudang untuk mulai."
          }
        />
      ) : (
        <LocationsTable
          search={{
            page: search.page,
            q: search.q,
            external: true,
            deleted: search.deleted,
          }}
          onPageChange={(page) => onSearchChange({ page })}
          emptyMessage={
            search.deleted
              ? "Tidak ada lokasi eksternal yang dihapus."
              : "Belum ada lokasi eksternal."
          }
        />
      )}
      <WarehouseFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        warehouse={editing}
      />
      <LocationFormDialog open={locationOpen} onOpenChange={setLocationOpen} />
      {pending ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setPending(null)}
          title={
            pending.kind === "delete"
              ? `Hapus gudang ${pending.warehouse.code}?`
              : `Pulihkan gudang ${pending.warehouse.code}?`
          }
          description={
            pending.kind === "delete"
              ? "Gudang dan semua lokasinya tidak bisa dipilih di transaksi baru, tetapi tetap tampil di dokumen lama. Kode tidak bisa dipakai ulang."
              : "Gudang dan lokasi yang terhapus bersamanya kembali aktif."
          }
          confirmLabel={
            pending.kind === "delete" ? "Hapus Gudang" : "Pulihkan Gudang"
          }
          destructive={pending.kind === "delete"}
          loading={action.isPending}
          onConfirm={() => action.mutate(pending)}
        />
      ) : null}
    </div>
  )
}
