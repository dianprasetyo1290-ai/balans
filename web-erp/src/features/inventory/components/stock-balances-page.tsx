import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { todayBusinessDate } from "@/lib/format/date"
import { formatQuantity } from "@/lib/format/number"
import { formatRupiah, formatUnitCost } from "@/lib/format/money"

import { locationsQuery } from "@/features/warehouse/api/warehouse-api"

import {
  stockBalancesQuery,
  type StockBalance,
  type StockBalanceSearch,
} from "../api/inventory-api"
import {
  BatchFilter,
  ExpiryCell,
  LocationFilter,
  LocationTypeFilter,
  ProductFilter,
  StatusFilter,
  StockStatusBadge,
} from "./stock-filters"

const route = getRouteApi("/_app/inventory/stock")

const NO_LOCATION = "00000000-0000-0000-0000-000000000000"

/** Persediaan › Stok per Lokasi: saldo per produk × lokasi × batch × status (S-212, S-213). */
export function StockBalancesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const locations = useQuery({
    ...locationsQuery({ page: 1 }),
    enabled: Boolean(search.locationType),
  })
  // Jenis lokasi tanpa lokasi tertentu: semua lokasi jenis itu; jenis tanpa lokasi = tidak ada saldo.
  const typeLocationIds =
    search.locationType && !search.locationId
      ? (locations.data?.items ?? [])
          .filter((l) => l.locationType === search.locationType)
          .map((l) => l.id)
      : undefined
  const balances = useQuery({
    ...stockBalancesQuery(
      search,
      typeLocationIds?.length === 0 ? [NO_LOCATION] : typeLocationIds
    ),
    enabled:
      !search.locationType || Boolean(search.locationId) || locations.isSuccess,
  })
  const setSearch = (patch: Partial<StockBalanceSearch>) =>
    void navigate({ search: (s) => ({ ...s, ...patch, page: 1 }) })

  const columns: DataTableColumn<StockBalance>[] = [
    {
      id: "product",
      header: "Produk",
      cell: (b) => (
        <div className="flex min-w-44 flex-col">
          <span className="font-medium">{b.product.sku}</span>
          <span className="text-xs text-muted-foreground">
            {b.product.name}
          </span>
        </div>
      ),
    },
    {
      id: "location",
      header: "Lokasi",
      cell: (b) => (
        <div className="flex min-w-32 flex-col">
          <span>{b.location.code}</span>
          <span className="text-xs text-muted-foreground">
            {b.location.warehouseName ?? b.location.name}
          </span>
        </div>
      ),
    },
    {
      id: "batch",
      header: "Batch",
      cell: (b) => (
        <span className="whitespace-nowrap">{b.batchNumber ?? "—"}</span>
      ),
    },
    {
      id: "expiry",
      header: "Kedaluwarsa",
      cell: (b) => (
        <ExpiryCell expiryDate={b.expiryDate} state={b.expiryState} />
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (b) => <StockStatusBadge status={b.status} />,
    },
    {
      id: "qty",
      header: "Qty",
      align: "right",
      cell: (b) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatQuantity(b.qty)} {b.product.uom}
        </span>
      ),
    },
    {
      id: "unitCost",
      header: "HPP/unit",
      align: "right",
      cell: (b) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatUnitCost(b.unitCost)}
        </span>
      ),
    },
    {
      id: "value",
      header: "Nilai",
      align: "right",
      cell: (b) => (
        <span className="whitespace-nowrap tabular-nums">
          {formatRupiah(b.value)}
        </span>
      ),
    },
    {
      id: "card",
      header: "",
      align: "right",
      cell: (b) => (
        <Link
          to="/inventory/stock-card"
          search={{
            page: 1,
            dateFrom: `${todayBusinessDate().slice(0, 8)}01`,
            dateTo: todayBusinessDate(),
            productId: b.product.id,
            locationId: b.location.id,
            batchId: b.batchId,
          }}
          className="inline-flex min-h-11 items-center whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          Kartu Stok
        </Link>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Stok per Lokasi
        </h1>
        <p className="text-sm text-muted-foreground">
          Saldo stok per produk, lokasi, batch, dan status. Hanya stok Tersedia
          yang bisa dialokasikan; nilai memakai HPP rata-rata sisa cost layer
          batch.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <ProductFilter
          id="stock-product"
          value={search.productId}
          onChange={(productId) => setSearch({ productId, batchId: undefined })}
        />
        <LocationTypeFilter
          id="stock-location-type"
          value={search.locationType}
          onChange={(locationType) =>
            setSearch({ locationType, locationId: undefined })
          }
        />
        <LocationFilter
          id="stock-location"
          value={search.locationId}
          onChange={(locationId) => setSearch({ locationId })}
        />
        <BatchFilter
          id="stock-batch"
          productId={search.productId}
          value={search.batchId}
          onChange={(batchId) => setSearch({ batchId })}
        />
        <StatusFilter
          id="stock-status"
          value={search.status}
          onChange={(status) => setSearch({ status })}
        />
      </div>
      {balances.data ? (
        <p className="text-sm text-muted-foreground">
          Total qty sesuai filter:{" "}
          <span className="font-medium text-foreground tabular-nums">
            {formatQuantity(balances.data.totalQty)}
          </span>
        </p>
      ) : null}
      <DataTable
        columns={columns}
        rows={balances.data?.items}
        getRowId={(b) =>
          `${b.product.id}-${b.location.id}-${b.batchId ?? "-"}-${b.status}`
        }
        loading={balances.isPending}
        fetching={balances.isFetching}
        page={search.page}
        totalPages={balances.data?.totalPages ?? 0}
        totalItems={balances.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada stok yang cocok dengan filter."
      />
    </div>
  )
}
