import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { StatusBadge } from "@/components/common/status-badge"
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
  REQUEST_STATUSES,
  requestsQuery,
  type PurchaseRequestStatus,
  type PurchaseRequestSummary,
} from "../api/procurement-api"

const route = getRouteApi("/_app/procurement/purchase-requests/")

const ALL = "all"

/** Pengadaan › Purchase Request: daftar PR dengan filter di URL (S-201, Desain UI/UX 3.2). */
export function RequestListPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const requests = useQuery(requestsQuery(search))
  const [q, setQ] = useState(search.q ?? "")

  const columns: DataTableColumn<PurchaseRequestSummary>[] = [
    {
      id: "number",
      header: "Nomor PR",
      sortKey: "number",
      cell: (r) => (
        <Link
          to="/procurement/purchase-requests/$id"
          params={{ id: r.id }}
          search={{}}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {r.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      sortKey: "businessDate",
      cell: (r) => (
        <span className="whitespace-nowrap">{formatDate(r.businessDate)}</span>
      ),
    },
    { id: "division", header: "Divisi", cell: (r) => r.divisionName || "—" },
    {
      id: "requestedBy",
      header: "Peminta",
      cell: (r) => (
        <div className="flex max-w-xs flex-col">
          <span>{r.requestedByName || "—"}</span>
          {r.note ? (
            <span className="truncate text-xs text-muted-foreground">
              {r.note}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "needed",
      header: "Dibutuhkan",
      cell: (r) => (r.neededDate ? formatDate(r.neededDate) : "—"),
    },
    {
      id: "total",
      header: "Estimasi Total",
      sortKey: "estimatedTotal",
      align: "right",
      cell: (r) => formatRupiah(r.estimatedTotal),
    },
    {
      id: "status",
      header: "Status",
      cell: (r) => <StatusBadge status={r.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Purchase Request
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Permintaan pembelian dari divisi. Diajukan ke Head divisi peminta;
            PR yang disetujui dijadikan Purchase Order sebagian atau seluruh
            baris.
          </p>
        </div>
        {can("procurement.purchase_request.write") ? (
          <Button render={<Link to="/procurement/purchase-requests/new" />}>
            <PlusIcon /> Buat Purchase Request
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            void navigate({
              search: (s) => ({ ...s, q: q || undefined, page: 1 }),
            })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari Purchase Request"
            placeholder="Cari nomor atau catatan"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                status:
                  v === ALL || !v ? undefined : (v as PurchaseRequestStatus),
                page: 1,
              }),
            })
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
            {REQUEST_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {statusLabel(s).label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={search.mine ? "mine" : ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                mine: v === "mine" ? true : undefined,
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter peminta">
            <SelectValue>
              {(v: string) =>
                v === "mine" ? "Diajukan saya" : "Semua peminta"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua peminta</SelectItem>
            <SelectItem value="mine">Diajukan saya</SelectItem>
          </SelectContent>
        </Select>
        <DateRange
          from={search.dateFrom}
          to={search.dateTo}
          onChange={(dateFrom, dateTo) =>
            void navigate({
              search: (s) => ({ ...s, dateFrom, dateTo, page: 1 }),
            })
          }
        />
      </div>
      <DataTable
        columns={columns}
        rows={requests.data?.items}
        getRowId={(r) => r.id}
        loading={requests.isPending}
        fetching={requests.isFetching}
        page={search.page}
        totalPages={requests.data?.totalPages ?? 0}
        totalItems={requests.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          void navigate({ search: (s) => ({ ...s, sort, page: 1 }) })
        }
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada Purchase Request."
      />
    </div>
  )
}

/** Filter rentang tanggal dokumen (disimpan di URL oleh halaman pemakai). */
export function DateRange({
  from,
  to,
  onChange,
}: {
  from?: string
  to?: string
  onChange: (from: string | undefined, to: string | undefined) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="w-40">
        <DateInput
          value={from ?? ""}
          placeholder="Dari tanggal"
          onValueChange={(v) => onChange(v || undefined, to)}
        />
      </div>
      <span className="text-sm text-muted-foreground">s.d.</span>
      <div className="w-40">
        <DateInput
          value={to ?? ""}
          placeholder="Sampai tanggal"
          onValueChange={(v) => onChange(from, v || undefined)}
        />
      </div>
      {from || to ? (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onChange(undefined, undefined)}
        >
          Hapus tanggal
        </Button>
      ) : null}
    </div>
  )
}
