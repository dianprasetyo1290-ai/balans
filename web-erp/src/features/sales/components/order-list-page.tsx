import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DateRange } from "@/features/procurement/components/request-list-page"
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { statusLabel } from "@/lib/format/status-labels"

import {
  ORDER_KINDS,
  ORDER_STATUSES,
  ORDER_TYPE_LABELS,
  ordersQuery,
  type OrderKind,
  type OrderListSearch,
  type SalesDocumentStatus,
  type SalesOrderSummary,
  type SalesOrderType,
} from "../api/sales-documents-api"

const ALL = "all"

type Props = {
  kind: OrderKind
  search: OrderListSearch
  onSearchChange: (update: (s: OrderListSearch) => OrderListSearch) => void
  /** Konten tambahan di bawah judul, mis. panel impor marketplace. */
  children?: React.ReactNode
}

/**
 * Penjualan › Sales Order / Modern Trade / Penjualan Marketplace / Pesanan WhatsApp/Instagram / Penjualan
 * Karyawan: daftar pesanan per channel dengan filter di URL (S-301, S-306 – S-308, Desain UI/UX 3.2).
 */
export function OrderListPage({
  kind,
  search,
  onSearchChange,
  children,
}: Props) {
  const spec = ORDER_KINDS[kind]
  const { can } = usePermissions()
  const orders = useQuery(ordersQuery(kind, search))
  const [q, setQ] = useState(search.q ?? "")
  const marketplace = kind === "marketplace"
  const canCreate = marketplace
    ? can("sales.marketplace_order.write")
    : can("sales.sales_order.write")
  const filterTypes = kind === "all" || kind === "modern_trade"

  const columns: DataTableColumn<SalesOrderSummary>[] = [
    {
      id: "number",
      header: "Nomor SO",
      sortKey: "number",
      cell: (o) => (
        <Link
          to="/sales/orders/$id"
          params={{ id: o.id }}
          search={{}}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {o.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      sortKey: "businessDate",
      cell: (o) => (
        <span className="whitespace-nowrap">{formatDate(o.businessDate)}</span>
      ),
    },
    {
      id: "party",
      header: marketplace ? "Pembeli" : "Pelanggan / Mitra",
      cell: (o) => (
        <div className="flex flex-col">
          <span>{o.partyName || "—"}</span>
          <span className="text-xs text-muted-foreground">
            {marketplace
              ? o.externalOrderId
              : [o.partyCode, o.partyPhone, o.clientPoNumber]
                  .filter(Boolean)
                  .join(" · ")}
          </span>
        </div>
      ),
    },
    {
      id: "channel",
      header: "Channel",
      cell: (o) => (
        <div className="flex flex-col gap-1">
          {filterTypes ? (
            <Badge variant="secondary" className="w-fit">
              {ORDER_TYPE_LABELS[o.orderType]}
            </Badge>
          ) : null}
          <span className="text-xs text-muted-foreground">
            {o.subChannelName}
          </span>
        </div>
      ),
    },
    {
      id: "total",
      header: "Total",
      sortKey: "totalAmount",
      align: "right",
      cell: (o) => formatRupiah(o.totalAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (o) => <StatusBadge status={o.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {spec.title}
          </h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            {spec.description}
          </p>
        </div>
        {canCreate ? (
          <div className="flex flex-wrap gap-2">
            {spec.create.map((t) => (
              <Button
                key={t}
                variant={t === spec.create[0] ? "default" : "outline"}
                render={
                  <Link to="/sales/orders/new" search={{ orderType: t }} />
                }
              >
                <PlusIcon />
                {marketplace
                  ? "Catat Penjualan Marketplace"
                  : `Buat Sales Order ${spec.create.length > 1 ? ORDER_TYPE_LABELS[t] : ""}`.trim()}
              </Button>
            ))}
          </div>
        ) : null}
      </div>
      {children}
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange((s) => ({ ...s, q: q || undefined, page: 1 }))
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label={`Cari ${spec.title}`}
            placeholder={
              marketplace
                ? "Cari nomor SO, nomor order, pembeli"
                : "Cari nomor SO, nama, HP, PO klien"
            }
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              status: v === ALL || !v ? undefined : (v as SalesDocumentStatus),
              page: 1,
            }))
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === ALL ? "Semua status" : statusLabel(v).label
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {ORDER_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {statusLabel(s).label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {filterTypes ? (
          <Select
            value={search.orderType ?? ALL}
            onValueChange={(v) =>
              onSearchChange((s) => ({
                ...s,
                orderType: v === ALL || !v ? undefined : (v as SalesOrderType),
                page: 1,
              }))
            }
          >
            <SelectTrigger className="w-48" aria-label="Filter channel">
              <SelectValue>
                {(v: string) =>
                  v === ALL
                    ? "Semua channel"
                    : ORDER_TYPE_LABELS[v as SalesOrderType]
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>Semua channel</SelectItem>
              {(kind === "all"
                ? (Object.keys(ORDER_TYPE_LABELS) as SalesOrderType[])
                : spec.types
              ).map((t) => (
                <SelectItem key={t} value={t}>
                  {ORDER_TYPE_LABELS[t]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : null}
        <DateRange
          from={search.dateFrom}
          to={search.dateTo}
          onChange={(dateFrom, dateTo) =>
            onSearchChange((s) => ({ ...s, dateFrom, dateTo, page: 1 }))
          }
        />
      </div>
      <DataTable
        columns={columns}
        rows={orders.data?.items}
        getRowId={(o) => o.id}
        loading={orders.isPending}
        fetching={orders.isFetching}
        page={search.page}
        totalPages={orders.data?.totalPages ?? 0}
        totalItems={orders.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          onSearchChange((s) => ({ ...s, sort, page: 1 }))
        }
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage={`Belum ada ${spec.title}.`}
      />
    </div>
  )
}
