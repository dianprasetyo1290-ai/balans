import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { MinusIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { StatusBadge } from "@/components/common/status-badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
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
import { EntityHistory } from "@/features/audit/components/entity-history"
import { usePermissions } from "@/features/auth/api/auth-api"
import { locationsQuery } from "@/features/warehouse/api/warehouse-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  actOnStockCount,
  batchLabel,
  COUNT_TYPE_LABELS,
  COUNT_TYPES,
  createStockCounts,
  FREQUENCY_LABELS,
  locationLabel,
  MAX_PHOTO_BYTES,
  MONTH_LABELS,
  requestPhotoUpload,
  saveCountEntries,
  saveSchedule,
  schedulesQuery,
  stockCountQuery,
  stockCountsQuery,
  WEEKDAY_LABELS,
  type CountSearch,
  type StockCount,
  type StockCountAction,
  type StockCountFrequency,
  type StockCountSchedule,
  type StockCountStatus,
  type StockCountSummary,
  type StockCountType,
} from "../api/documents-api"
import { ApprovalPanel, Info } from "./document-parts"

const ALL = "all"

const STATUS_TEXT: Record<StockCountStatus, string> = {
  draft: "Draft",
  in_progress: "Diproses",
  pending_approval: "Menunggu Persetujuan",
  rejected: "Ditolak",
  posted: "Diposting",
  completed: "Selesai",
  cancelled: "Dibatalkan",
}

type ListTab = "counts" | "schedules"

/** Gudang › Stock Count: daftar hitung per lokasi dan jadwal (S-217, S-218). */
export function StockCountListPage({
  tab,
  search,
  onSearchChange,
}: {
  tab: ListTab
  search: CountSearch
  onSearchChange: (patch: Partial<CountSearch> & { tab?: ListTab }) => void
}) {
  const { can } = usePermissions()
  const [createOpen, setCreateOpen] = useState(false)
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Stock Count</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Hitung buta per lokasi: qty sistem disembunyikan sampai hasil
            dikirim. Selisih stock count gudang memerlukan persetujuan Head
            Logistik (lebih dari Rp2 jt juga Head Finance &amp; Accounting).
          </p>
        </div>
        {can("warehouse.stock_count.write") && tab === "counts" ? (
          <Button className="min-h-11" onClick={() => setCreateOpen(true)}>
            <PlusIcon /> Buat Stock Count
          </Button>
        ) : null}
      </div>
      <PageTabs
        label="Bagian Stock Count"
        value={tab}
        onValueChange={(t) => onSearchChange({ tab: t, page: 1 })}
        tabs={[
          { id: "counts", label: "Daftar" },
          { id: "schedules", label: "Jadwal" },
        ]}
      />
      {tab === "counts" ? (
        <CountList search={search} onSearchChange={onSearchChange} />
      ) : (
        <ScheduleList />
      )}
      {createOpen ? (
        <CreateCountDialog onClose={() => setCreateOpen(false)} />
      ) : null}
    </div>
  )
}

function CountList({
  search,
  onSearchChange,
}: {
  search: CountSearch
  onSearchChange: (p: Partial<CountSearch>) => void
}) {
  const list = useQuery(stockCountsQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const columns: DataTableColumn<StockCountSummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (c) => (
        <Link
          to="/warehouse/stock-counts/$id"
          params={{ id: c.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {c.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (c) => (
        <span className="whitespace-nowrap">{formatDate(c.businessDate)}</span>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (c) => COUNT_TYPE_LABELS[c.countType],
    },
    {
      id: "location",
      header: "Lokasi",
      cell: (c) => locationLabel(c.location),
    },
    {
      id: "progress",
      header: "Dihitung",
      align: "right",
      cell: (c) => `${c.countedCount} dari ${c.lineCount}`,
    },
    {
      id: "variance",
      header: "Nilai Selisih",
      align: "right",
      cell: (c) => (c.varianceAmount ? formatRupiah(c.varianceAmount) : "—"),
    },
    { id: "assignee", header: "Petugas", cell: (c) => c.assignedToName || "—" },
    {
      id: "status",
      header: "Status",
      cell: (c) => <StatusBadge status={c.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-64"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange({ q: q || undefined, page: 1 })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari nomor"
            placeholder="Cari nomor SC"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            onSearchChange({
              status: v === ALL ? undefined : (v as StockCountStatus),
              page: 1,
            })
          }
        >
          <SelectTrigger className="w-52" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === ALL ? "Semua status" : STATUS_TEXT[v as StockCountStatus]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {(Object.keys(STATUS_TEXT) as StockCountStatus[]).map((s) => (
              <SelectItem key={s} value={s}>
                {STATUS_TEXT[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={search.countType ?? ALL}
          onValueChange={(v) =>
            onSearchChange({
              countType: v === ALL ? undefined : (v as StockCountType),
              page: 1,
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter jenis">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua jenis"
                  : COUNT_TYPE_LABELS[v as StockCountType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua jenis</SelectItem>
            {COUNT_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {COUNT_TYPE_LABELS[t]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        caption="Daftar Stock Count"
        columns={columns}
        rows={list.data?.items}
        getRowId={(c) => c.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange({ page })}
        emptyMessage="Belum ada stock count."
      />
    </div>
  )
}

const TYPE_LOCATIONS: Record<StockCountType, (t: string) => boolean> = {
  full: (t) =>
    !["consignment", "vending_buffer", "vending_machine"].includes(t),
  cycle: (t) =>
    !["consignment", "vending_buffer", "vending_machine"].includes(t),
  consignment: (t) => t === "consignment",
  vending: (t) => t === "vending_buffer" || t === "vending_machine",
}

function LocationChecklist({
  type,
  value,
  onChange,
}: {
  type: StockCountType
  value: string[]
  onChange: (v: string[]) => void
}) {
  const locations = useQuery(locationsQuery({ page: 1 }))
  const items = (locations.data?.items ?? []).filter(
    (l) => l.isActive && !l.deletedAt && TYPE_LOCATIONS[type](l.locationType)
  )
  return (
    <div className="flex max-h-56 flex-col gap-1 overflow-y-auto rounded-md border p-2">
      {items.length === 0 ? (
        <span className="text-sm text-muted-foreground">
          Tidak ada lokasi yang sesuai.
        </span>
      ) : null}
      {items.map((l) => (
        <label key={l.id} className="flex min-h-11 items-center gap-3 text-sm">
          <input
            type="checkbox"
            className="size-5"
            checked={value.includes(l.id)}
            onChange={(e) =>
              onChange(
                e.target.checked
                  ? [...value, l.id]
                  : value.filter((x) => x !== l.id)
              )
            }
          />
          {l.code} · {l.name}
        </label>
      ))}
    </div>
  )
}

function TypeSelect({
  id,
  value,
  onChange,
}: {
  id: string
  value: StockCountType
  onChange: (t: StockCountType) => void
}) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as StockCountType)}>
      <SelectTrigger id={id} className="min-h-11 w-full">
        <SelectValue>
          {(v: string) => COUNT_TYPE_LABELS[v as StockCountType]}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {COUNT_TYPES.map((t) => (
          <SelectItem key={t} value={t}>
            {COUNT_TYPE_LABELS[t]}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function CreateCountDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [type, setType] = useState<StockCountType>("cycle")
  const [locs, setLocs] = useState<string[]>([])
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createStockCounts({ countType: type, locationIds: locs }, key),
    onSuccess: async (res) => {
      toast.success(`${res.items.length} stock count dibuat`)
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onClose()
      if (res.items.length === 1) {
        void navigate({
          to: "/warehouse/stock-counts/$id",
          params: { id: res.items[0].id },
        })
      }
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setErrors(p.fields)
      } else {
        showErrorDialog(p)
      }
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Buat Stock Count</DialogTitle>
          <DialogDescription>
            Satu dokumen per lokasi. Snapshot saldo sistem diambil saat mulai
            hitung.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="sc-type">Jenis</FieldLabel>
          <TypeSelect
            id="sc-type"
            value={type}
            onChange={(t) => {
              setType(t)
              setLocs([])
            }}
          />
        </Field>
        <Field>
          <FieldLabel>Lokasi</FieldLabel>
          <LocationChecklist type={type} value={locs} onChange={setLocs} />
          {errors.locationIds ? (
            <FieldError>{errors.locationIds}</FieldError>
          ) : null}
        </Field>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            loading={save.isPending}
            className="min-h-11"
            onClick={() => save.mutate()}
          >
            Buat Stock Count
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function scheduleWhen(s: StockCountSchedule) {
  switch (s.frequency) {
    case "weekly":
      return `Setiap ${WEEKDAY_LABELS[(s.dayOfWeek ?? 1) - 1]}`
    case "monthly":
      return s.dayOfMonth === 31
        ? "Akhir bulan"
        : `Tanggal ${s.dayOfMonth} tiap bulan`
    default:
      return `${s.dayOfMonth} ${MONTH_LABELS[(s.monthOfYear ?? 1) - 1]} tiap tahun`
  }
}

function ScheduleList() {
  const { can } = usePermissions()
  const schedules = useQuery(schedulesQuery)
  const [editing, setEditing] = useState<StockCountSchedule | "new" | null>(
    null
  )
  return (
    <div className="flex flex-col gap-3">
      {can("warehouse.stock_count.write") ? (
        <Button
          variant="outline"
          className="min-h-11 self-start"
          onClick={() => setEditing("new")}
        >
          <PlusIcon /> Tambah Jadwal
        </Button>
      ) : null}
      <LoadingArea loading={schedules.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[44rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Nama</TableHead>
                <TableHead>Jenis</TableHead>
                <TableHead>Jadwal</TableHead>
                <TableHead>Lokasi</TableHead>
                <TableHead>Berikutnya</TableHead>
                <TableHead>Status</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(schedules.data?.items ?? []).map((s) => (
                <TableRow key={s.id}>
                  <TableCell className="font-medium">{s.name}</TableCell>
                  <TableCell>{COUNT_TYPE_LABELS[s.countType]}</TableCell>
                  <TableCell>{scheduleWhen(s)}</TableCell>
                  <TableCell>
                    {s.locationIds?.length
                      ? `${s.locationIds?.length} lokasi`
                      : "Semua lokasi sesuai jenis"}
                  </TableCell>
                  <TableCell>{formatDate(s.nextRunDate)}</TableCell>
                  <TableCell>{s.isActive ? "Aktif" : "Nonaktif"}</TableCell>
                  <TableCell>
                    {can("warehouse.stock_count.write") ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="min-h-11"
                        onClick={() => setEditing(s)}
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
      {editing ? (
        <ScheduleDialog
          schedule={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </div>
  )
}

function NumberSelect({
  id,
  value,
  onChange,
  options,
}: {
  id: string
  value: number
  onChange: (n: number) => void
  options: string[]
}) {
  return (
    <Select value={String(value)} onValueChange={(v) => onChange(Number(v))}>
      <SelectTrigger id={id} className="min-h-11 w-full">
        <SelectValue>{(v: string) => options[Number(v) - 1]}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {options.map((o, i) => (
          <SelectItem key={o} value={String(i + 1)}>
            {o}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

const DAYS = Array.from({ length: 31 }, (_, i) =>
  i === 30 ? "31 / akhir bulan" : String(i + 1)
)

function ScheduleDialog({
  schedule,
  onClose,
}: {
  schedule: StockCountSchedule | null
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [name, setName] = useState(schedule?.name ?? "")
  const [type, setType] = useState<StockCountType>(
    schedule?.countType ?? "cycle"
  )
  const [freq, setFreq] = useState<StockCountFrequency>(
    schedule?.frequency ?? "monthly"
  )
  const [dow, setDow] = useState(schedule?.dayOfWeek ?? 1)
  const [dom, setDom] = useState(schedule?.dayOfMonth ?? 31)
  const [moy, setMoy] = useState(schedule?.monthOfYear ?? 12)
  const [locs, setLocs] = useState<string[]>(schedule?.locationIds ?? [])
  const [active, setActive] = useState(schedule?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      saveSchedule(
        {
          name,
          countType: type,
          frequency: freq,
          dayOfWeek: freq === "weekly" ? dow : undefined,
          dayOfMonth: freq === "weekly" ? undefined : dom,
          monthOfYear: freq === "yearly" ? moy : undefined,
          locationIds: locs,
          isActive: active,
        },
        key,
        schedule?.id,
        schedule?.version
      ),
    onSuccess: async () => {
      toast.success("Jadwal disimpan")
      await queryClient.invalidateQueries({
        queryKey: ["warehouse", "stock-count-schedules"],
      })
      onClose()
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setErrors(p.fields)
      } else {
        showErrorDialog(p)
      }
    },
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {schedule ? "Ubah Jadwal" : "Tambah Jadwal"}
          </DialogTitle>
          <DialogDescription>
            Job harian membuat stock count pada tanggal jadwal dan mengirim
            pengingat ke petugas (Merchandiser mitra untuk lokasi konsinyasi).
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <Field>
            <FieldLabel htmlFor="sch-name">Nama</FieldLabel>
            <Input
              id="sch-name"
              className="min-h-11"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            {errors.name ? <FieldError>{errors.name}</FieldError> : null}
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="sch-type">Jenis</FieldLabel>
              <TypeSelect
                id="sch-type"
                value={type}
                onChange={(t) => {
                  setType(t)
                  setLocs([])
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="sch-freq">Frekuensi</FieldLabel>
              <Select
                value={freq}
                onValueChange={(v) => setFreq(v as StockCountFrequency)}
              >
                <SelectTrigger id="sch-freq" className="min-h-11 w-full">
                  <SelectValue>
                    {(v: string) => FREQUENCY_LABELS[v as StockCountFrequency]}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(FREQUENCY_LABELS) as StockCountFrequency[]).map(
                    (f) => (
                      <SelectItem key={f} value={f}>
                        {FREQUENCY_LABELS[f]}
                      </SelectItem>
                    )
                  )}
                </SelectContent>
              </Select>
            </Field>
            {freq === "weekly" ? (
              <Field>
                <FieldLabel htmlFor="sch-dow">Hari</FieldLabel>
                <NumberSelect
                  id="sch-dow"
                  value={dow}
                  onChange={setDow}
                  options={WEEKDAY_LABELS}
                />
              </Field>
            ) : (
              <Field>
                <FieldLabel htmlFor="sch-dom">Tanggal</FieldLabel>
                <NumberSelect
                  id="sch-dom"
                  value={dom}
                  onChange={setDom}
                  options={DAYS}
                />
              </Field>
            )}
            {freq === "yearly" ? (
              <Field>
                <FieldLabel htmlFor="sch-moy">Bulan</FieldLabel>
                <NumberSelect
                  id="sch-moy"
                  value={moy}
                  onChange={setMoy}
                  options={MONTH_LABELS}
                />
              </Field>
            ) : null}
          </div>
          <Field>
            <FieldLabel>
              Lokasi (kosong = semua lokasi aktif sesuai jenis)
            </FieldLabel>
            <LocationChecklist type={type} value={locs} onChange={setLocs} />
            {errors.warehouseId ? (
              <FieldError>{errors.warehouseId}</FieldError>
            ) : null}
          </Field>
          <label className="flex min-h-11 items-center gap-3 text-sm">
            <input
              type="checkbox"
              className="size-5"
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
            />
            Aktif
          </label>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            loading={save.isPending}
            className="min-h-11"
            onClick={() => save.mutate()}
          >
            Simpan Jadwal
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type DetailTab = "count" | "history"

const ACTION_LABELS: Partial<Record<StockCountAction, string>> = {
  start: "Mulai Hitung",
  recount: "Hitung Ulang",
  cancel: "Batalkan",
}

/** Detail Stock Count: form hitung HP saat Diproses, hasil & selisih setelah dikirim, Persetujuan, Riwayat. */
export function StockCountDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const count = useQuery(stockCountQuery(id))
  const c = count.data
  const act = useIdempotentMutation({
    mutationFn: (action: StockCountAction, key) =>
      actOnStockCount(id, action, c?.version ?? 0, key),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  if (count.isError) {
    return (
      <p className="text-sm text-destructive">
        Stock count tidak ditemukan atau tidak bisa dibuka.
      </p>
    )
  }
  return (
    <LoadingArea loading={count.isPending}>
      {c ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">
                {c.number ?? "Draft Stock Count"}
              </h1>
              <StatusBadge status={c.status} />
            </div>
            <div className="flex flex-wrap gap-2">
              {(["cancel", "recount", "start"] as StockCountAction[])
                .filter((a) => c.allowedActions.includes(a))
                .map((a) => (
                  <LoadingButton
                    key={a}
                    variant={a === "cancel" ? "outline" : "default"}
                    className="min-h-11"
                    loading={act.isPending && act.variables === a}
                    onClick={() => act.mutate(a)}
                  >
                    {ACTION_LABELS[a]}
                  </LoadingButton>
                ))}
            </div>
          </div>
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info label="Lokasi" value={locationLabel(c.location)} />
            <Info label="Jenis" value={COUNT_TYPE_LABELS[c.countType]} />
            <Info label="Tanggal" value={formatDate(c.businessDate)} />
            <Info label="Petugas" value={c.assignedToName} />
            {c.snapshotAt ? (
              <Info label="Snapshot" value={formatDateTime(c.snapshotAt)} />
            ) : null}
            {c.submittedAt ? (
              <Info
                label="Dikirim"
                value={`${c.submittedByName} · ${formatDateTime(c.submittedAt)}`}
              />
            ) : null}
            {c.varianceAmount ? (
              <Info
                label="Nilai selisih absolut"
                value={formatRupiah(c.varianceAmount)}
              />
            ) : null}
            {c.netVariance ? (
              <Info
                label="Selisih bersih"
                value={formatRupiah(c.netVariance)}
              />
            ) : null}
            {c.decisionNote ? (
              <Info label="Catatan keputusan" value={c.decisionNote} />
            ) : null}
            {c.journalEntryId ? (
              <Info
                label="Jurnal A-14"
                value={
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: c.journalEntryId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                }
              />
            ) : null}
          </dl>
          <PageTabs
            label="Bagian stock count"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "count", label: "Lembar Hitung" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "history" ? (
            <EntityHistory entityType="stock_count" entityId={c.id} />
          ) : c.allowedActions.includes("count") ? (
            <CountForm key={c.id} count={c} />
          ) : (
            <CountResult count={c} />
          )}
          <ApprovalPanel subjectType="stock_count" subjectId={c.id} />
        </div>
      ) : null}
    </LoadingArea>
  )
}

function CountResult({ count: c }: { count: StockCount }) {
  if (c.lines.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        Lembar hitung dibuat saat Mulai Hitung.
      </p>
    )
  }
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[48rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Produk</TableHead>
            <TableHead>Batch</TableHead>
            {c.revealed ? (
              <TableHead className="text-right">Sistem</TableHead>
            ) : null}
            <TableHead className="text-right">Hitung</TableHead>
            {c.revealed ? (
              <TableHead className="text-right">Selisih</TableHead>
            ) : null}
            {c.revealed ? (
              <TableHead className="text-right">Nilai Selisih</TableHead>
            ) : null}
          </TableRow>
        </TableHeader>
        <TableBody>
          {c.lines.map((l) => (
            <TableRow key={l.id}>
              <TableCell>
                <span className="font-medium">{l.product.sku}</span>{" "}
                {l.product.name}
                {l.isExtra ? (
                  <span className="ml-1 text-xs text-muted-foreground">
                    (temuan)
                  </span>
                ) : null}
              </TableCell>
              <TableCell>{batchLabel(l.batch)}</TableCell>
              {c.revealed ? (
                <TableCell className="text-right tabular-nums">
                  {l.systemQty}
                </TableCell>
              ) : null}
              <TableCell className="text-right tabular-nums">
                {l.countedQty ?? "—"}
              </TableCell>
              {c.revealed ? (
                <TableCell className="text-right tabular-nums">
                  {l.varianceQty}
                </TableCell>
              ) : null}
              {c.revealed ? (
                <TableCell className="text-right tabular-nums">
                  {l.varianceAmount ? formatRupiah(l.varianceAmount) : "—"}
                </TableCell>
              ) : null}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

type Draft = { counts: Record<string, string>; photos: string[] }

function readDraft(c: StockCount): Draft {
  const fallback: Draft = {
    counts: Object.fromEntries(
      c.lines.map((l) => [
        l.id,
        l.countedQty ? String(Number(l.countedQty)) : "",
      ])
    ),
    photos: c.photoFileIds,
  }
  try {
    const raw = sessionStorage.getItem(`stock-count:${c.id}`)
    if (raw) {
      const saved = JSON.parse(raw) as Draft
      return {
        counts: { ...fallback.counts, ...saved.counts },
        photos: saved.photos ?? fallback.photos,
      }
    }
  } catch {
    // sessionStorage tidak tersedia: pakai data server.
  }
  return fallback
}

/**
 * Form hitung HP (Desain UI/UX 4, TG 10.10): target sentuh ≥ 44 px, stepper, keyboard numerik, isian disimpan
 * di sessionStorage dan otomatis ke server (debounce), foto lokasi dikompres, tombol kirim sticky di bawah.
 */
function CountForm({ count: c }: { count: StockCount }) {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<Draft>(() => readDraft(c))
  const [filter, setFilter] = useState("")
  const [savedAt, setSavedAt] = useState<string | null>(null)
  const dirty = useRef(false)
  const autosave = useIdempotentMutation({
    mutationFn: (d: Draft, key) =>
      saveCountEntries(
        c.id,
        {
          lines: c.lines
            .filter(
              (l) => d.counts[l.id] !== undefined && d.counts[l.id] !== ""
            )
            .map((l) => ({ lineId: l.id, countedQty: d.counts[l.id] })),
          photoFileIds: d.photos,
        },
        key
      ),
    onSuccess: (res) => {
      dirty.current = false
      setSavedAt(new Date().toISOString())
      queryClient.setQueryData(stockCountQuery(c.id).queryKey, res)
    },
  })
  const { mutate } = autosave
  useEffect(() => {
    try {
      sessionStorage.setItem(`stock-count:${c.id}`, JSON.stringify(draft))
    } catch {
      // abaikan: autosave server tetap berjalan.
    }
    if (!dirty.current) {
      return
    }
    const timer = setTimeout(() => mutate(draft), 1200)
    return () => clearTimeout(timer)
  }, [c.id, draft, mutate])

  const update = (patch: Partial<Draft>) => {
    dirty.current = true
    setDraft((d) => ({ ...d, ...patch }))
  }
  const setCount = (lineId: string, v: string) =>
    update({ counts: { ...draft.counts, [lineId]: v } })
  const step = (lineId: string, delta: number) => {
    const n = Math.max(0, (Number(draft.counts[lineId]) || 0) + delta)
    setCount(lineId, String(n))
  }

  const submit = useIdempotentMutation({
    mutationFn: async (_: void, key) => {
      await saveCountEntries(
        c.id,
        {
          lines: c.lines
            .filter((l) => draft.counts[l.id] !== "")
            .map((l) => ({ lineId: l.id, countedQty: draft.counts[l.id] })),
          photoFileIds: draft.photos,
        },
        `${key}-save`
      )
      const fresh = await queryClient.fetchQuery({
        ...stockCountQuery(c.id),
        staleTime: 0,
      })
      return actOnStockCount(c.id, "submit", fresh.version, key)
    },
    onSuccess: async (res) => {
      try {
        sessionStorage.removeItem(`stock-count:${c.id}`)
      } catch {
        // abaikan
      }
      toast.success(
        res.status === "pending_approval"
          ? "Hasil dikirim; menunggu persetujuan selisih"
          : "Hasil hitung dikirim"
      )
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const visible = c.lines.filter((l) => {
    const q = filter.trim().toLowerCase()
    return (
      !q ||
      l.product.sku.toLowerCase().includes(q) ||
      l.product.name.toLowerCase().includes(q) ||
      (l.batch?.number ?? "").toLowerCase().includes(q)
    )
  })
  const counted = c.lines.filter((l) => draft.counts[l.id] !== "").length
  return (
    <div className="mx-auto flex w-full max-w-xl flex-col gap-3 pb-24">
      <Input
        aria-label="Cari SKU atau batch"
        placeholder="Cari / pindai SKU atau batch"
        className="min-h-11"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
      <ul className="flex flex-col divide-y rounded-lg border">
        {visible.map((l) => (
          <li key={l.id} className="flex flex-col gap-2 p-3">
            <div className="flex flex-col">
              <span className="font-medium">
                {l.product.sku} · {l.product.name}
              </span>
              <span className="text-xs text-muted-foreground">
                Batch {batchLabel(l.batch)}
                {l.batch?.expiryDate
                  ? ` · ED ${formatDate(l.batch.expiryDate)}`
                  : ""}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="size-11"
                aria-label={`Kurangi ${l.product.sku}`}
                onClick={() => step(l.id, -1)}
              >
                <MinusIcon />
              </Button>
              <Input
                aria-label={`Qty hitung ${l.product.sku} ${batchLabel(l.batch)}`}
                inputMode="decimal"
                className="min-h-11 flex-1 text-center text-lg tabular-nums"
                value={draft.counts[l.id] ?? ""}
                onChange={(e) =>
                  setCount(
                    l.id,
                    e.target.value.replace(/[^0-9.,]/g, "").replace(",", ".")
                  )
                }
              />
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="size-11"
                aria-label={`Tambah ${l.product.sku}`}
                onClick={() => step(l.id, 1)}
              >
                <PlusIcon />
              </Button>
            </div>
          </li>
        ))}
        {visible.length === 0 ? (
          <li className="p-3 text-sm text-muted-foreground">
            Tidak ada baris.
          </li>
        ) : null}
      </ul>
      <Field>
        <FieldLabel>
          Foto lokasi/rak ({draft.photos.length}/10)
          {c.countType === "consignment" || c.countType === "vending"
            ? " — wajib"
            : ""}
        </FieldLabel>
        <FileUpload
          label="Ambil Foto"
          accept="image/*"
          maxSizeBytes={MAX_PHOTO_BYTES}
          disabled={draft.photos.length >= 10}
          requestUpload={(f) => requestPhotoUpload("stock_count", f)}
          onUploaded={(fileId) => update({ photos: [...draft.photos, fileId] })}
          onError={(m) => toast.error(m)}
        />
      </Field>
      <div className="fixed inset-x-0 bottom-0 z-10 border-t bg-background p-3 sm:sticky">
        <div className="mx-auto flex max-w-xl flex-col gap-1">
          <span className="text-xs text-muted-foreground" aria-live="polite">
            {counted} dari {c.lines.length} baris dihitung ·{" "}
            {autosave.isPending
              ? "Menyimpan…"
              : savedAt
                ? `Tersimpan otomatis ${formatDateTime(savedAt)}`
                : "Isian tersimpan di perangkat"}
          </span>
          {c.allowedActions.includes("submit") ? (
            <LoadingButton
              className="min-h-11 w-full"
              loading={submit.isPending}
              onClick={() => submit.mutate()}
            >
              Kirim Hasil Hitung
            </LoadingButton>
          ) : null}
        </div>
      </div>
    </div>
  )
}
