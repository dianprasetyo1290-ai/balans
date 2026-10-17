import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import {
  ASSET_STATUS_LABELS,
  assetClassesQuery,
  fixedAssetsQuery,
  type FixedAsset,
  type FixedAssetClass,
  type FixedAssetStatus,
} from "../api/close-api"
import { AssetClassFormDialog } from "./asset-class-form-dialog"
import { AssetStatusBadge } from "./asset-status-badge"
import { FixedAssetFormDialog } from "./fixed-asset-form-dialog"

const route = getRouteApi("/_app/finance/fixed-assets/")

const ALL = "all"

/** Keuangan & Akuntansi › Aset Tetap: register aset per kelas & lokasi dan kelas aset (ACC-21, S-408). */
export function FixedAssetsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const [creating, setCreating] = useState(false)

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Aset Tetap</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Register aset termasuk mesin vending per lokasi. Penyusutan garis
            lurus dijalankan otomatis setiap tanggal 1 untuk bulan sebelumnya.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons
            kind="accounting.opening_fixed_asset"
            resource="dataexchange.transaction"
          />
          {can("accounting.fixed_asset.write") && search.tab === "assets" ? (
            <Button onClick={() => setCreating(true)}>
              <PlusIcon /> Tambah Aset Tetap
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Aset tetap"
        tabs={[
          { id: "assets", label: "Daftar Aset" },
          { id: "classes", label: "Kelas Aset" },
        ]}
        value={search.tab}
        onValueChange={(tab) =>
          void navigate({ search: (s) => ({ ...s, tab }) })
        }
      />
      {search.tab === "assets" ? <AssetList /> : <ClassList />}
      <FixedAssetFormDialog open={creating} onOpenChange={setCreating} />
    </div>
  )
}

function AssetList() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [q, setQ] = useState(search.q ?? "")
  const classes = useQuery(assetClassesQuery(true))
  const assets = useQuery(
    fixedAssetsQuery({
      page: search.page,
      q: search.q,
      status: search.status,
      classId: search.classId,
    })
  )
  const setSearch = (patch: Partial<typeof search>) =>
    void navigate({ search: (s) => ({ ...s, ...patch, page: 1 }) })

  const columns: DataTableColumn<FixedAsset>[] = [
    {
      id: "code",
      header: "Kode",
      cell: (a) => (
        <Link
          to="/finance/fixed-assets/$id"
          params={{ id: a.id }}
          search={{ tab: "detail" }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {a.code}
        </Link>
      ),
    },
    {
      id: "name",
      header: "Nama",
      cell: (a) => (
        <div className="flex flex-col">
          <span>{a.name}</span>
          <span className="text-xs text-muted-foreground">
            {a.className}
            {a.locationLabel ? ` · ${a.locationLabel}` : ""}
          </span>
        </div>
      ),
    },
    {
      id: "date",
      header: "Perolehan",
      cell: (a) => (
        <span className="whitespace-nowrap">
          {formatDate(a.acquisitionDate)}
        </span>
      ),
    },
    {
      id: "cost",
      header: "Harga Perolehan",
      align: "right",
      cell: (a) => formatRupiah(a.cost),
    },
    {
      id: "accumulated",
      header: "Akumulasi",
      align: "right",
      cell: (a) => formatRupiah(a.accumulated),
    },
    {
      id: "book",
      header: "Nilai Buku",
      align: "right",
      cell: (a) => formatRupiah(a.bookValue),
    },
    {
      id: "status",
      header: "Status",
      cell: (a) => <AssetStatusBadge status={a.status} />,
    },
  ]

  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            setSearch({ q: q || undefined })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari aset"
            placeholder="Cari kode, nama, atau lokasi"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            setSearch({
              status: v === ALL ? undefined : (v as FixedAssetStatus),
            })
          }
        >
          <SelectTrigger aria-label="Status" className="w-40">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua status"
                  : ASSET_STATUS_LABELS[v as FixedAssetStatus]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {(["draft", "active", "disposed"] as const).map((s) => (
              <SelectItem key={s} value={s}>
                {ASSET_STATUS_LABELS[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={search.classId ?? ALL}
          onValueChange={(v) =>
            setSearch({ classId: v === ALL ? undefined : String(v) })
          }
        >
          <SelectTrigger aria-label="Kelas aset" className="w-56">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua kelas"
                  : ((classes.data ?? []).find((c) => c.id === v)?.name ??
                    "Kelas")
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua kelas</SelectItem>
            {(classes.data ?? []).map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={assets.data?.items}
        getRowId={(a) => a.id}
        loading={assets.isPending}
        fetching={assets.isFetching}
        page={search.page}
        totalPages={assets.data?.totalPages ?? 0}
        totalItems={assets.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada aset tetap."
      />
    </>
  )
}

function ClassList() {
  const { can } = usePermissions()
  const classes = useQuery(assetClassesQuery(true))
  const [form, setForm] = useState<{ value?: FixedAssetClass } | null>(null)
  const canWrite = can("accounting.fixed_asset.write")
  return (
    <>
      {canWrite ? (
        <div>
          <Button variant="outline" onClick={() => setForm({})}>
            <PlusIcon /> Tambah Kelas Aset
          </Button>
        </div>
      ) : null}
      <LoadingArea loading={classes.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kelas</TableHead>
                <TableHead className="text-right">Umur Manfaat</TableHead>
                <TableHead>Akun Aset</TableHead>
                <TableHead>Akun Akumulasi</TableHead>
                <TableHead>Status</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(classes.data ?? []).map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="font-medium">{c.name}</TableCell>
                  <TableCell className="text-right whitespace-nowrap">
                    {c.usefulLifeMonths} bulan
                  </TableCell>
                  <TableCell>{c.assetAccountCode}</TableCell>
                  <TableCell>{c.accumulatedAccountCode}</TableCell>
                  <TableCell>{c.isActive ? "Aktif" : "Nonaktif"}</TableCell>
                  <TableCell className="text-right">
                    {canWrite ? (
                      <Button
                        variant="ghost"
                        className="min-h-11 sm:min-h-8"
                        onClick={() => setForm({ value: c })}
                      >
                        Ubah
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <AssetClassFormDialog
        open={!!form}
        onOpenChange={(o) => !o && setForm(null)}
        value={form?.value}
      />
    </>
  )
}
