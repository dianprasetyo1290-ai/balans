import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
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

import {
  ENTRY_TYPE_LABELS,
  journalsQuery,
  type JournalEntrySummary,
  type JournalEntryType,
  type JournalStatus,
} from "../api/accounting-api"

const route = getRouteApi("/_app/finance/journals/")

const ALL = "all"

/** Keuangan & Akuntansi › Jurnal Umum: daftar jurnal manual, otomatis, dan Balik Jurnal (S-127). */
export function JournalListPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const journals = useQuery(journalsQuery(search))
  const [q, setQ] = useState(search.q ?? "")

  const columns: DataTableColumn<JournalEntrySummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (e) => (
        <Link
          to="/finance/journals/$id"
          params={{ id: e.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {e.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (e) => (
        <span className="whitespace-nowrap">{formatDate(e.businessDate)}</span>
      ),
    },
    {
      id: "description",
      header: "Keterangan",
      cell: (e) => (
        <div className="flex max-w-md flex-col">
          <span className="truncate">{e.description}</span>
          {e.entryType === "automatic" && e.sourceLabel ? (
            <span className="text-xs text-muted-foreground">
              {e.postingRuleCode} · {e.sourceLabel}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (e) => (
        <Badge variant="secondary">{ENTRY_TYPE_LABELS[e.entryType]}</Badge>
      ),
    },
    {
      id: "total",
      header: "Total",
      align: "right",
      cell: (e) => formatRupiah(e.totalAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (e) => (
        <div className="flex flex-wrap gap-1">
          <StatusBadge status={e.status} />
          {e.reversedByNumber ? <Badge variant="outline">Dibalik</Badge> : null}
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Jurnal Umum</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Jurnal manual dan jurnal otomatis dari dokumen sumber. Jurnal yang
            sudah diposting tidak bisa diubah; koreksi dengan Balik Jurnal.
          </p>
        </div>
        {can("accounting.journal.write") ? (
          <Button render={<Link to="/finance/journals/new" />}>
            <PlusIcon /> Buat Jurnal Umum
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
            aria-label="Cari jurnal"
            placeholder="Cari nomor atau keterangan"
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
                status: v === ALL ? undefined : (v as JournalStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-40" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === "draft"
                  ? "Draft"
                  : v === "posted"
                    ? "Diposting"
                    : "Semua status"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            <SelectItem value="draft">Draft</SelectItem>
            <SelectItem value="posted">Diposting</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={search.entryType ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                entryType: v === ALL ? undefined : (v as JournalEntryType),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter jenis">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua jenis"
                  : ENTRY_TYPE_LABELS[v as JournalEntryType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua jenis</SelectItem>
            {(Object.keys(ENTRY_TYPE_LABELS) as JournalEntryType[]).map((t) => (
              <SelectItem key={t} value={t}>
                {ENTRY_TYPE_LABELS[t]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={journals.data?.items}
        getRowId={(e) => e.id}
        loading={journals.isPending}
        fetching={journals.isFetching}
        page={search.page}
        totalPages={journals.data?.totalPages ?? 0}
        totalItems={journals.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada jurnal."
      />
    </div>
  )
}
