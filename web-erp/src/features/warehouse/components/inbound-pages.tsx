import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
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
import { Textarea } from "@/components/ui/textarea"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  batchLabel,
  inboundQuery,
  inboundsQuery,
  locationLabel,
  MAX_PHOTO_BYTES,
  recordQc,
  requestPhotoUpload,
  type Inbound,
  type InboundLine,
  type InboundStatus,
  type InboundSummary,
} from "../api/documents-api"
import { Info } from "./document-parts"

const listRoute = getRouteApi("/_app/warehouse/inbounds/")
const ALL = "all"

/** Gudang › Inbound: antrian barang Karantina per dokumen asal (S-211). */
export function InboundListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const list = useQuery(inboundsQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const columns: DataTableColumn<InboundSummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (i) => (
        <Link
          to="/warehouse/inbounds/$id"
          params={{ id: i.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {i.number}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (i) => (
        <span className="whitespace-nowrap">{formatDate(i.businessDate)}</span>
      ),
    },
    {
      id: "source",
      header: "Dokumen Asal",
      cell: (i) => (
        <div className="flex flex-col">
          <span>{i.sourceTypeLabel}</span>
          <span className="text-xs text-muted-foreground">
            {i.sourceLabel || "—"}
          </span>
        </div>
      ),
    },
    {
      id: "location",
      header: "Lokasi",
      cell: (i) => locationLabel(i.location),
    },
    {
      id: "pending",
      header: "Belum QC",
      align: "right",
      cell: (i) => `${i.pendingCount} dari ${i.lineCount}`,
    },
    {
      id: "status",
      header: "Status",
      cell: (i) => <StatusBadge status={i.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Inbound</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Semua barang masuk berstatus Karantina sampai lulus QC. Stok Karantina
          tidak bisa dialokasikan, dijual, atau ditransfer.
        </p>
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
            aria-label="Cari Inbound"
            placeholder="Cari nomor atau dokumen asal"
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
                status: v === ALL ? undefined : (v as InboundStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === "in_progress"
                  ? "Diproses"
                  : v === "completed"
                    ? "Selesai"
                    : "Semua status"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            <SelectItem value="in_progress">Diproses</SelectItem>
            <SelectItem value="completed">Selesai</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <DataTable
        caption="Daftar Inbound"
        columns={columns}
        rows={list.data?.items}
        getRowId={(i) => i.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Tidak ada barang di antrian Inbound."
      />
    </div>
  )
}

type DetailTab = "lines" | "history"

/** Detail Inbound: baris Karantina, QC per baris, catatan QC, dan Riwayat. */
export function InboundDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const inbound = useQuery(inboundQuery(id))
  const [qcLine, setQcLine] = useState<InboundLine | null>(null)
  const ib = inbound.data
  if (inbound.isError) {
    return (
      <p className="text-sm text-destructive">
        Inbound tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }
  return (
    <LoadingArea loading={inbound.isPending}>
      {ib ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {ib.number}
            </h1>
            <StatusBadge status={ib.status} />
          </div>
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info
              label="Dokumen Asal"
              value={`${ib.sourceTypeLabel} ${ib.sourceLabel}`}
            />
            <Info label="Tanggal" value={formatDate(ib.businessDate)} />
            <Info label="Lokasi Inbound" value={locationLabel(ib.location)} />
            <Info
              label="Barang ditolak"
              value={
                ib.rejectAction === "hold"
                  ? "Ditahan di Inbound (reject ke pemasok/maklon)"
                  : "Dipindah ke Gudang Retur"
              }
            />
          </dl>
          <PageTabs
            label="Bagian Inbound"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "lines", label: "Baris & QC" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "lines" ? (
            <InboundLines inbound={ib} onQc={setQcLine} />
          ) : (
            <EntityHistory entityType="inbound" entityId={ib.id} />
          )}
          {qcLine ? (
            <QcDialog
              inbound={ib}
              line={qcLine}
              onClose={() => setQcLine(null)}
            />
          ) : null}
        </div>
      ) : null}
    </LoadingArea>
  )
}

function InboundLines({
  inbound,
  onQc,
}: {
  inbound: Inbound
  onQc: (l: InboundLine) => void
}) {
  return (
    <div className="flex flex-col gap-4">
      <div className="overflow-x-auto rounded-lg border">
        <Table className="min-w-[56rem]">
          <TableHeader>
            <TableRow>
              <TableHead>Produk</TableHead>
              <TableHead>Batch</TableHead>
              <TableHead className="text-right">Qty</TableHead>
              <TableHead className="text-right">Lulus</TableHead>
              <TableHead className="text-right">Ditolak</TableHead>
              <TableHead>Lokasi Tujuan</TableHead>
              <TableHead>Status</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {inbound.lines.map((l) => (
              <TableRow key={l.id}>
                <TableCell>
                  <div className="flex flex-col">
                    <span className="font-medium">{l.product.sku}</span>
                    <span className="text-xs text-muted-foreground">
                      {l.product.name}
                    </span>
                  </div>
                </TableCell>
                <TableCell>{batchLabel(l.batch)}</TableCell>
                <TableCell className="text-right tabular-nums">
                  {l.qty}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {l.passedQty}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {l.rejectedQty}
                  {l.rejectReason ? (
                    <span className="block text-xs text-muted-foreground">
                      {l.rejectReason}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell>{locationLabel(l.destination)}</TableCell>
                <TableCell>
                  <StatusBadge status={l.status} />
                </TableCell>
                <TableCell>
                  {l.allowedActions.includes("qc") ? (
                    <Button
                      size="sm"
                      className="min-h-11"
                      onClick={() => onQc(l)}
                    >
                      QC
                    </Button>
                  ) : l.checkedAt ? (
                    <span className="text-xs whitespace-nowrap text-muted-foreground">
                      {l.checkedByName} · {formatDateTime(l.checkedAt)}
                    </span>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      {inbound.checks.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h2 className="font-medium">Catatan QC</h2>
          <ul className="flex flex-col gap-1 text-sm">
            {inbound.checks.map((c) => (
              <li key={c.id}>
                {c.result === "passed" ? "Lulus" : "Ditolak"} {c.qty} ·{" "}
                {c.checkedByName} · {formatDateTime(c.checkedAt)}
                {c.note ? ` — ${c.note}` : ""}
                {c.photoFileIds.length
                  ? ` · ${c.photoFileIds.length} foto`
                  : ""}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  )
}

const CHECKS = [
  { key: "batch", label: "Batch sesuai" },
  { key: "expiry", label: "Tanggal kedaluwarsa sesuai" },
  { key: "condition", label: "Kondisi barang baik" },
  { key: "seal", label: "Segel utuh" },
] as const

type CheckKey = (typeof CHECKS)[number]["key"]

function QcDialog({
  inbound,
  line,
  onClose,
}: {
  inbound: Inbound
  line: InboundLine
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [passed, setPassed] = useState(line.qty)
  const [rejected, setRejected] = useState("0")
  const [checks, setChecks] = useState<Record<CheckKey, boolean>>({
    batch: true,
    expiry: true,
    condition: true,
    seal: true,
  })
  const [reason, setReason] = useState("")
  const [note, setNote] = useState("")
  const [photos, setPhotos] = useState<string[]>([])
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      recordQc(
        inbound.id,
        line.id,
        {
          passedQty: passed || "0",
          rejectedQty: rejected || "0",
          checks,
          rejectReason: reason || undefined,
          note: note || undefined,
          photoFileIds: photos,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Hasil QC disimpan")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
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
            QC {line.product.sku} · {batchLabel(line.batch)}
          </DialogTitle>
          <DialogDescription>
            Qty {line.qty} {line.product.uom}. Lulus + ditolak harus sama dengan
            qty baris.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setErrors({})
            save.mutate()
          }}
        >
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-1 text-sm font-medium">Checklist QC</legend>
            {CHECKS.map((c) => (
              <label
                key={c.key}
                className="flex min-h-11 items-center gap-3 text-sm"
              >
                <input
                  type="checkbox"
                  className="size-5"
                  checked={checks[c.key]}
                  onChange={(e) =>
                    setChecks({ ...checks, [c.key]: e.target.checked })
                  }
                />
                {c.label}
              </label>
            ))}
            {errors.checks ? <FieldError>{errors.checks}</FieldError> : null}
          </fieldset>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="qc-passed">Qty lulus</FieldLabel>
              <QuantityInput
                id="qc-passed"
                value={passed}
                unit={line.product.uom}
                onValueChange={setPassed}
                className="min-h-11"
              />
              {errors.passedQty ? (
                <FieldError>{errors.passedQty}</FieldError>
              ) : null}
            </Field>
            <Field>
              <FieldLabel htmlFor="qc-rejected">Qty ditolak</FieldLabel>
              <QuantityInput
                id="qc-rejected"
                value={rejected}
                unit={line.product.uom}
                onValueChange={setRejected}
                className="min-h-11"
              />
              {errors.rejectedQty ? (
                <FieldError>{errors.rejectedQty}</FieldError>
              ) : null}
            </Field>
          </div>
          {Number(rejected) > 0 ? (
            <Field>
              <FieldLabel htmlFor="qc-reason">Alasan penolakan</FieldLabel>
              <Textarea
                id="qc-reason"
                value={reason}
                onChange={(e) => setReason(e.target.value)}
              />
              {errors.rejectReason ? (
                <FieldError>{errors.rejectReason}</FieldError>
              ) : null}
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="qc-note">Catatan</FieldLabel>
            <Textarea
              id="qc-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>Foto ({photos.length}/5)</FieldLabel>
            <FileUpload
              label="Ambil Foto"
              accept="image/*"
              maxSizeBytes={MAX_PHOTO_BYTES}
              disabled={photos.length >= 5}
              requestUpload={(f) => requestPhotoUpload("quality_check", f)}
              onUploaded={(fileId) => setPhotos((p) => [...p, fileId])}
              onError={(m) => toast.error(m)}
            />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              className="min-h-11"
            >
              Simpan Hasil QC
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
