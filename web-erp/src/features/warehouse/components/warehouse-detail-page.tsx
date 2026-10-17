import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { ArrowLeftIcon, PlusIcon } from "lucide-react"
import { useState } from "react"

import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
import { Button } from "@/components/ui/button"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { usePermissions } from "@/features/auth/api/auth-api"
import { formatDateTime } from "@/lib/format/date"

import { warehouseQuery } from "../api/warehouse-api"
import { LocationFormDialog } from "./location-form-dialog"
import { LocationsTable } from "./locations-table"
import { WarehouseFormDialog } from "./warehouse-form-dialog"

const WAREHOUSE_FIELD_LABELS: Record<string, string> = {
  code: "Kode",
  name: "Nama",
  address: "Alamat",
  isActive: "Aktif",
  deletedAt: "Dihapus pada",
}

type Tab = "locations" | "history"

type WarehouseDetailPageProps = {
  id: string
  tab: Tab
  page: number
  onSearchChange: (patch: { tab?: Tab; page?: number }) => void
}

/** Detail gudang dengan tab Lokasi dan Riwayat (S-123, S-105). Gudang terhapus tetap bisa dibuka. */
export function WarehouseDetailPage({
  id,
  tab,
  page,
  onSearchChange,
}: WarehouseDetailPageProps) {
  const { can } = usePermissions()
  const warehouse = useQuery(warehouseQuery(id))
  const [formOpen, setFormOpen] = useState(false)
  const [locationOpen, setLocationOpen] = useState(false)
  const w = warehouse.data

  if (warehouse.isError) {
    return (
      <p className="text-sm text-destructive">
        Gudang tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }

  return (
    <LoadingArea loading={warehouse.isPending}>
      {w ? (
        <div className="flex flex-col gap-4">
          <div>
            <Button
              variant="ghost"
              size="sm"
              className="-ml-2"
              nativeButton={false}
              render={
                <Link
                  to="/master-data/warehouses"
                  search={{ tab: "warehouses", page: 1 }}
                />
              }
            >
              <ArrowLeftIcon /> Gudang & Lokasi
            </Button>
          </div>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {w.name}
                </h1>
                <RecordStatusBadge
                  isActive={w.isActive}
                  deletedAt={w.deletedAt}
                />
              </div>
              <p className="text-sm text-muted-foreground">
                {w.code}
                {w.address ? ` · ${w.address}` : ""}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {w.allowedActions.includes("update") ? (
                <Button variant="outline" onClick={() => setFormOpen(true)}>
                  Ubah Gudang
                </Button>
              ) : null}
              {!w.deletedAt && can("warehouse.location.write") ? (
                <Button onClick={() => setLocationOpen(true)}>
                  <PlusIcon /> Tambah Lokasi
                </Button>
              ) : null}
            </div>
          </div>
          {w.deletedAt ? (
            <p className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm">
              Dihapus {formatDateTime(w.deletedAt)} beserta lokasinya. Pulihkan
              dari daftar Gudang & Lokasi.
            </p>
          ) : null}
          <PageTabs
            label="Bagian gudang"
            value={tab}
            onValueChange={(t) => onSearchChange({ tab: t, page: 1 })}
            tabs={[
              { id: "locations", label: "Lokasi" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "locations" ? (
            <LocationsTable
              search={{ page, warehouseId: w.id, deleted: !!w.deletedAt }}
              onPageChange={(p) => onSearchChange({ page: p })}
              emptyMessage="Belum ada lokasi."
            />
          ) : (
            <EntityHistory
              entityType="warehouse"
              entityId={w.id}
              fieldLabels={WAREHOUSE_FIELD_LABELS}
            />
          )}
          <WarehouseFormDialog
            open={formOpen}
            onOpenChange={setFormOpen}
            warehouse={w}
          />
          <LocationFormDialog
            open={locationOpen}
            onOpenChange={setLocationOpen}
            warehouse={w}
          />
        </div>
      ) : null}
    </LoadingArea>
  )
}
