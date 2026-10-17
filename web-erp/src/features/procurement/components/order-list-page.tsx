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
  type PurchaseOrderStatus,
  type PurchaseOrderSummary,
  type PurchaseOrderType,
} from "../api/procurement-api"
import { DateRange } from "./request-list-page"

const ALL = "all"

type Props = {
  kind: OrderKind
  search: OrderListSearch
  onSearchChange: (update: (s: OrderListSearch) => OrderListSearch) => void
}

/**
 * Pengadaan › Purchase Order / Purchase Order Maklon / Purchase Order Reprocess: daftar PO per kelompok jenis
 * dengan filter di URL (S-202 – S-204, Desain UI/UX 3.2).
 */
export function OrderListPage({ kind, search, onSearchChange }: Props) {
  const spec = ORDER_KINDS[kind]
  const { can } = usePermissions()
  const orders = useQuery(ordersQuery(kind, search))
  const [q, setQ] = useState(search.q ?? "")

  const columns: DataTableColumn<PurchaseOrderSummary>[] = [
    {
      id: "number",
      header: "Nomor PO",
      sortKey: "number",
      cell: (o) => (
        <Link
          to="/procurement/purchase-orders/$id"
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
      id: "supplier",
      header: kind === "regular" ? "Pemasok" : "Maklon",
      cell: (o) => (
        <div className="flex flex-col">
          <span>{o.supplierName}</span>
          <span className="text-xs text-muted-foreground">
            {o.supplierCode}
          </span>
        </div>
      ),
    },
    ...(kind === "regular"
      ? [
          {
            id: "type",
            header: "Jenis",
            cell: (o: PurchaseOrderSummary) => (
              <Badge variant="secondary">{o.orderTypeLabel}</Badge>
            ),
          },
        ]
      : []),
    {
      id: "expected",
      header: "Tgl Kirim Diharapkan",
      sortKey: "expectedDate",
      cell: (o) => (
        <span className="whitespace-nowrap">{formatDate(o.expectedDate)}</span>
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
          <p className="max-w-2xl text-sm text-muted-foreground">
            {spec.description}
          </p>
        </div>
        {can("procurement.purchase_order.write") ? (
          <Button
            render={
              <Link
                to="/procurement/purchase-orders/new"
                search={{ orderType: spec.types[0] }}
              />
            }
          >
            <PlusIcon /> Buat {spec.title}
          </Button>
        ) : null}
      </div>
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
            placeholder="Cari nomor PO atau pemasok"
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
              status: v === ALL || !v ? undefined : (v as PurchaseOrderStatus),
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
        {spec.types.length > 1 ? (
          <Select
            value={search.orderType ?? ALL}
            onValueChange={(v) =>
              onSearchChange((s) => ({
                ...s,
                orderType:
                  v === ALL || !v ? undefined : (v as PurchaseOrderType),
                page: 1,
              }))
            }
          >
            <SelectTrigger className="w-40" aria-label="Filter jenis PO">
              <SelectValue>
                {(v: string) =>
                  v === ALL
                    ? "Semua jenis"
                    : ORDER_TYPE_LABELS[v as PurchaseOrderType]
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>Semua jenis</SelectItem>
              {spec.types.map((t) => (
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
