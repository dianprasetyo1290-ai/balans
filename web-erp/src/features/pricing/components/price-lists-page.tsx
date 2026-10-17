import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"

import { LoadingArea } from "@/components/common/loading-area"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Checkbox } from "@/features/accounting/components/account-form-dialog"
import { usePermissions } from "@/features/auth/api/auth-api"
import { formatDate } from "@/lib/format/date"
import { cn } from "@/lib/utils"

import {
  PRICE_LIST_TYPE_LABELS,
  formatPercent,
  priceListsQuery,
  type PriceList,
  type PriceListSearch,
  type PriceListType,
} from "../api/pricing-api"
import { PriceListFormDialog } from "./price-list-form-dialog"
import { useChannelNames } from "./pricing-parts"

const ALL = "__all"

type PriceListsPageProps = {
  search: PriceListSearch
  onSearchChange: (patch: Partial<PriceListSearch>) => void
}

/** Cakupan daftar harga: channel/sub-channel, mitra, atau acuan persentase. */
export function scopeLabel(
  l: Pick<
    PriceList,
    | "priceListType"
    | "channelId"
    | "subChannelId"
    | "partnerName"
    | "resellerLevel"
  >,
  channelName: (id?: string | null) => string | null
) {
  switch (l.priceListType) {
    case "base":
      return "Semua channel"
    case "employee":
      return "Pelanggan berstatus karyawan aktif"
    case "partner":
      return l.partnerName ?? "—"
    case "reseller":
      return `Level ${l.resellerLevel}`
    default:
      return [channelName(l.channelId), channelName(l.subChannelId)]
        .filter(Boolean)
        .join(" › ")
  }
}

/**
 * Data Master › Daftar Harga (MD-04, S-116): Harga Dasar & HET serta daftar harga per channel, mitra, karyawan, dan
 * vending. Harga berlaku lewat versi yang disetujui Head Sales lalu Head Finance & Accounting.
 */
export function PriceListsPage({
  search,
  onSearchChange,
}: PriceListsPageProps) {
  const { can } = usePermissions()
  const lists = useQuery(priceListsQuery(search))
  const { name: channelName } = useChannelNames()
  const [q, setQ] = useState(search.q ?? "")
  const [creating, setCreating] = useState(false)
  const canWrite = can("pricing.price_list.write")

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Daftar Harga
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Harga per channel, mitra, karyawan, dan vending dengan riwayat
            versi. Versi baru berlaku setelah disetujui Head Sales dan Head
            Finance & Accounting; transaksi lama tetap memakai harga saat
            dibuat.
          </p>
        </div>
        {canWrite && !search.deleted ? (
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Tambah Daftar Harga
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange({ q: q || undefined })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari daftar harga"
            placeholder="Cari kode atau nama"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.priceListType ?? ALL}
          onValueChange={(v) =>
            onSearchChange({
              priceListType: !v || v === ALL ? undefined : (v as PriceListType),
            })
          }
        >
          <SelectTrigger className="w-full sm:w-56" aria-label="Jenis">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua jenis"
                  : PRICE_LIST_TYPE_LABELS[v as PriceListType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua jenis</SelectItem>
            {(Object.keys(PRICE_LIST_TYPE_LABELS) as PriceListType[]).map(
              (t) => (
                <SelectItem key={t} value={t}>
                  {PRICE_LIST_TYPE_LABELS[t]}
                </SelectItem>
              )
            )}
          </SelectContent>
        </Select>
        <Checkbox
          label="Tampilkan yang nonaktif"
          checked={!!search.includeInactive}
          onChange={(v) => onSearchChange({ includeInactive: v || undefined })}
        />
        {can("pricing.price_list.write") ? (
          <Checkbox
            label="Dihapus"
            checked={!!search.deleted}
            onChange={(v) => onSearchChange({ deleted: v || undefined })}
          />
        ) : null}
      </div>
      <LoadingArea loading={lists.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kode</TableHead>
                <TableHead>Nama</TableHead>
                <TableHead>Jenis</TableHead>
                <TableHead>Berlaku untuk</TableHead>
                <TableHead>Versi berlaku</TableHead>
                <TableHead>Versi berikutnya</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(lists.data ?? []).map((l) => (
                <TableRow
                  key={l.id}
                  className={cn(!l.isActive && "text-muted-foreground")}
                >
                  <TableCell className="font-medium whitespace-nowrap">
                    <Link
                      to="/master/price-lists/$id"
                      params={{ id: l.id }}
                      search={{}}
                      className="underline-offset-4 hover:underline"
                    >
                      {l.code}
                    </Link>
                  </TableCell>
                  <TableCell>{l.name}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {PRICE_LIST_TYPE_LABELS[l.priceListType]}
                    {l.basis === "percent" && l.currentVersion?.percent
                      ? ` · ${formatPercent(l.currentVersion.percent)} dari ${l.referencePriceListName ?? "acuan"}`
                      : ""}
                  </TableCell>
                  <TableCell>{scopeLabel(l, channelName)}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {l.currentVersion
                      ? `v${l.currentVersion.versionNo} · sejak ${formatDate(l.currentVersion.validFrom)}`
                      : "Belum ada"}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {l.nextVersion
                      ? `v${l.nextVersion.versionNo} · mulai ${formatDate(l.nextVersion.validFrom)}`
                      : "—"}
                    {l.openVersion ? (
                      <span className="ml-2 inline-flex">
                        <StatusBadge status={l.openVersion.status} />
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <RecordStatusBadge
                      isActive={l.isActive}
                      deletedAt={l.deletedAt}
                    />
                  </TableCell>
                </TableRow>
              ))}
              {lists.data && lists.data.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={7}
                    className="text-center text-muted-foreground"
                  >
                    Belum ada daftar harga yang cocok.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <PriceListFormDialog open={creating} onOpenChange={setCreating} />
    </div>
  )
}
