import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, PrinterIcon, Trash2Icon, XIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { usePermissions } from "@/features/auth/api/auth-api"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import {
  createDisposal,
  DISPOSAL_REASON_LABELS,
  disposalQuery,
  disposalsQuery,
  fileUrlQuery,
  requestDisposalPhotoUpload,
  runDisposalAction,
  updateDisposal,
  updateDisposalReport,
  type Disposal,
  type DisposalAction,
  type DisposalInput,
  type DisposalReason,
} from "../api/documents-api"
import {
  DocActionButtons,
  DocHeader,
  DocListFilters,
  DocMovementsTable,
  DocTabs,
  fieldErrors,
  handleFormError,
  Info,
  LocationSelect,
  OptionSelect,
  StockPickerDialog,
  stepText,
  type ActionSpec,
  type DocTab,
} from "./doc-shared"
import { StockStatusBadge } from "./stock-filters"

const listRoute = getRouteApi("/_app/inventory/disposals/")
const detailRoute = getRouteApi("/_app/inventory/disposals/$id/")

const MAX_PHOTO_BYTES = 10 << 20

/** Jenis lokasi asal per alasan pemusnahan (barang retur hanya dari Gudang Retur). */
const REASON_LOCATIONS: Record<DisposalReason, string[]> = {
  return: ["returns"],
  expired: ["finished_goods", "ecommerce", "petty_stock", "inbound", "event"],
  damaged: ["finished_goods", "ecommerce", "petty_stock", "inbound", "event"],
}

/** Persediaan › Pemusnahan dengan Berita Acara (INV-12). */
export function DisposalListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const docs = useQuery(disposalsQuery(search))
  const columns: DataTableColumn<Disposal>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (d) => (
        <Link
          to="/inventory/disposals/$id"
          params={{ id: d.id }}
          search={{ tab: "detail" }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {d.number ?? "Draft"}
        </Link>
      ),
    },
    { id: "date", header: "Tanggal", cell: (d) => formatDate(d.disposalDate) },
    { id: "reason", header: "Alasan", cell: (d) => d.reasonLabel },
    {
      id: "lines",
      header: "Baris",
      align: "right",
      cell: (d) => d.lines.length,
    },
    {
      id: "value",
      header: "Nilai HPP",
      align: "right",
      cell: (d) =>
        formatRupiah(d.status === "posted" ? d.costAmount : d.estimatedCost),
    },
    {
      id: "status",
      header: "Status",
      cell: (d) => <StatusBadge status={d.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Pemusnahan</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Pemusnahan barang kedaluwarsa, rusak, atau retur dengan berita acara
            (metode, saksi, foto) dan persetujuan berjenjang.
          </p>
        </div>
        {can("inventory.disposal.write") ? (
          <Button render={<Link to="/inventory/disposals/new" />}>
            <PlusIcon /> Buat Pemusnahan
          </Button>
        ) : null}
      </div>
      <DocListFilters
        q={search.q}
        status={search.status}
        statuses={[
          "draft",
          "pending_approval",
          "approved",
          "rejected",
          "posted",
          "cancelled",
        ]}
        onSearch={(q) =>
          void navigate({ search: (s) => ({ ...s, q, page: 1 }) })
        }
        onStatus={(status) =>
          void navigate({ search: (s) => ({ ...s, status, page: 1 }) })
        }
      />
      <DataTable
        columns={columns}
        rows={docs.data?.items}
        getRowId={(d) => d.id}
        loading={docs.isPending}
        fetching={docs.isFetching}
        page={search.page}
        totalPages={docs.data?.totalPages ?? 0}
        totalItems={docs.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada Pemusnahan."
      />
    </div>
  )
}

type Line = {
  productId: string
  label: string
  locationId: string
  locationCode: string
  batchId?: string
  batchNumber?: string
  stockStatus: "available" | "quarantine"
  qty: string
}

type Photo = { id: string; name: string }

/** Isian berita acara: tanggal, metode, saksi, foto. */
function ReportFields({
  date,
  setDate,
  method,
  setMethod,
  witnesses,
  setWitnesses,
  photos,
  setPhotos,
  errors,
  setErrors,
}: {
  date: string
  setDate: (v: string) => void
  method: string
  setMethod: (v: string) => void
  witnesses: string
  setWitnesses: (v: string) => void
  photos: Photo[]
  setPhotos: (fn: (p: Photo[]) => Photo[]) => void
  errors: Record<string, string>
  setErrors: (fn: (e: Record<string, string>) => Record<string, string>) => void
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <Field data-invalid={!!errors.disposalDate}>
        <FieldLabel htmlFor="ds-date">Tanggal Pemusnahan</FieldLabel>
        <DateInput id="ds-date" value={date} onValueChange={setDate} />
        <FieldError errors={fieldErrors(errors, "disposalDate")} />
      </Field>
      <Field data-invalid={!!errors.method}>
        <FieldLabel htmlFor="ds-method">Metode</FieldLabel>
        <Textarea
          id="ds-method"
          value={method}
          maxLength={500}
          onChange={(e) => setMethod(e.target.value)}
        />
        <FieldError errors={fieldErrors(errors, "method")} />
      </Field>
      <Field data-invalid={!!errors.witnesses}>
        <FieldLabel htmlFor="ds-witnesses">Saksi</FieldLabel>
        <Textarea
          id="ds-witnesses"
          value={witnesses}
          onChange={(e) => setWitnesses(e.target.value)}
        />
        <FieldDescription>
          Satu nama per baris, maksimal 10 saksi.
        </FieldDescription>
        <FieldError errors={fieldErrors(errors, "witnesses")} />
      </Field>
      <Field data-invalid={!!errors.photoFileIds || !!errors.fileId}>
        <FieldLabel>Foto</FieldLabel>
        <ul className="flex flex-col gap-1 text-sm">
          {photos.map((p) => (
            <li key={p.id} className="flex items-center gap-2">
              <span className="truncate">{p.name}</span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={`Hapus foto ${p.name}`}
                onClick={() =>
                  setPhotos((ps) => ps.filter((x) => x.id !== p.id))
                }
              >
                <XIcon />
              </Button>
            </li>
          ))}
        </ul>
        {photos.length < 10 ? (
          <FileUpload
            key={photos.length}
            label="Unggah Foto"
            accept="image/jpeg,image/png,image/webp"
            maxSizeBytes={MAX_PHOTO_BYTES}
            requestUpload={requestDisposalPhotoUpload}
            onUploaded={(id, file) =>
              setPhotos((ps) => [...ps, { id, name: file.name }])
            }
            onError={(message) => setErrors((e) => ({ ...e, fileId: message }))}
          />
        ) : null}
        <FieldError errors={fieldErrors(errors, "photoFileIds")} />
        <FieldError errors={fieldErrors(errors, "fileId")} />
      </Field>
    </div>
  )
}

const splitWitnesses = (s: string) =>
  s
    .split("\n")
    .map((w) => w.trim())
    .filter(Boolean)

/** Form Buat/Ubah Pemusnahan. */
export function DisposalFormPage({ id }: { id?: string }) {
  const doc = useQuery({ ...disposalQuery(id ?? ""), enabled: Boolean(id) })
  if (id && !doc.data) {
    return <LoadingArea loading>{null}</LoadingArea>
  }
  return <DisposalForm doc={doc.data} />
}

function DisposalForm({ doc }: { doc?: Disposal }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [reason, setReason] = useState<DisposalReason>(doc?.reason ?? "expired")
  const [location, setLocation] = useState("")
  const [date, setDate] = useState(doc?.disposalDate ?? todayBusinessDate())
  const [method, setMethod] = useState(doc?.method ?? "")
  const [witnesses, setWitnesses] = useState(doc?.witnesses.join("\n") ?? "")
  const [photos, setPhotos] = useState<Photo[]>(doc?.photos ?? [])
  const [note, setNote] = useState(doc?.note ?? "")
  const [lines, setLines] = useState<Line[]>(
    doc?.lines.map((l) => ({
      productId: l.product.id,
      label: `${l.product.sku} ${l.product.name}`,
      locationId: l.location.id,
      locationCode: l.location.code,
      batchId: l.batch?.id,
      batchNumber: l.batch?.number,
      stockStatus: l.stockStatus,
      qty: l.qty,
    })) ?? []
  )
  const [picking, setPicking] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: DisposalInput = {
        reason,
        disposalDate: date,
        method: method || undefined,
        witnesses: splitWitnesses(witnesses),
        photoFileIds: photos.map((p) => p.id),
        note: note || undefined,
        lines: lines.map((l) => ({
          productId: l.productId,
          batchId: l.batchId,
          locationId: l.locationId,
          stockStatus: l.stockStatus,
          qty: l.qty || "0",
        })),
      }
      return doc
        ? updateDisposal(doc.id, { ...body, version: doc.version }, key)
        : createDisposal(body, key)
    },
    onSuccess: async (d) => {
      toast.success("Pemusnahan disimpan")
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
      void navigate({
        to: "/inventory/disposals/$id",
        params: { id: d.id },
        search: { tab: "detail" },
      })
    },
    onError: (err) => handleFormError(err, setErrors),
  })

  return (
    <form
      noValidate
      className="flex max-w-4xl flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        setErrors({})
        save.mutate()
      }}
    >
      <h1 className="text-2xl font-semibold tracking-tight">
        {doc ? `Ubah ${doc.number ?? "Draft Pemusnahan"}` : "Buat Pemusnahan"}
      </h1>
      <div className="grid gap-4 sm:grid-cols-2">
        <OptionSelect
          id="ds-reason"
          label="Alasan"
          value={reason}
          onChange={(v) => {
            setReason(v as DisposalReason)
            setLocation("")
          }}
          options={Object.entries(DISPOSAL_REASON_LABELS).map(
            ([value, label]) => ({ value, label })
          )}
          invalid={!!errors.reason}
        />
        <Field>
          <FieldLabel htmlFor="ds-note">Catatan</FieldLabel>
          <Textarea
            id="ds-note"
            value={note}
            maxLength={500}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      <ReportFields
        date={date}
        setDate={setDate}
        method={method}
        setMethod={setMethod}
        witnesses={witnesses}
        setWitnesses={setWitnesses}
        photos={photos}
        setPhotos={setPhotos}
        errors={errors}
        setErrors={setErrors}
      />
      <section className="flex flex-col gap-2">
        <h2 className="font-medium">Barang yang Dimusnahkan</h2>
        <FieldError errors={fieldErrors(errors, "lines")} />
        <div className="flex items-end gap-2">
          <div className="flex-1 sm:max-w-sm">
            <LocationSelect
              id="ds-location"
              label="Lokasi"
              value={location}
              onChange={setLocation}
              types={REASON_LOCATIONS[reason]}
            />
          </div>
          <Button
            type="button"
            variant="outline"
            disabled={!location}
            onClick={() => setPicking(true)}
          >
            <PlusIcon /> Tambah Barang
          </Button>
        </div>
        {lines.map((l, i) => (
          <div
            key={i}
            className="grid items-end gap-2 rounded-lg border p-3 sm:grid-cols-[1fr_10rem_auto]"
          >
            <div className="text-sm">
              <div className="font-medium">{l.label}</div>
              <div className="flex items-center gap-2 text-muted-foreground">
                {l.locationCode} ·{" "}
                {l.batchNumber ? `Batch ${l.batchNumber}` : "Tanpa batch"}
                <StockStatusBadge status={l.stockStatus} />
              </div>
              <FieldError
                errors={fieldErrors(errors, `lines[${i}].locationId`)}
              />
            </div>
            <Field data-invalid={!!errors[`lines[${i}].qty`]}>
              <FieldLabel htmlFor={`ds-qty-${i}`}>Qty</FieldLabel>
              <QuantityInput
                id={`ds-qty-${i}`}
                value={l.qty}
                onValueChange={(v) =>
                  setLines((ls) =>
                    ls.map((x, j) => (j === i ? { ...x, qty: v } : x))
                  )
                }
              />
              <FieldError errors={fieldErrors(errors, `lines[${i}].qty`)} />
            </Field>
            <Button
              type="button"
              variant="ghost"
              aria-label={`Hapus baris ${i + 1}`}
              onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}
            >
              <Trash2Icon />
            </Button>
          </div>
        ))}
      </section>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => window.history.back()}
        >
          Batal
        </Button>
        <LoadingButton
          type="submit"
          loading={save.isPending}
          loadingText="Menyimpan…"
        >
          Simpan Draft
        </LoadingButton>
      </div>
      <StockPickerDialog
        open={picking}
        onOpenChange={setPicking}
        locationId={location}
        onPick={(b) =>
          setLines((ls) => [
            ...ls,
            {
              productId: b.product.id,
              label: `${b.product.sku} ${b.product.name}`,
              locationId: b.location.id,
              locationCode: b.location.code,
              batchId: b.batchId ?? undefined,
              batchNumber: b.batchNumber ?? undefined,
              stockStatus:
                b.status === "quarantine" ? "quarantine" : "available",
              qty: b.qty,
            },
          ])
        }
      />
    </form>
  )
}

const DISPOSAL_ACTIONS: ActionSpec<DisposalAction>[] = [
  {
    action: "submit",
    label: "Ajukan",
    primary: true,
    confirmTitle: "Ajukan pemusnahan?",
    confirmDescription:
      "Dokumen mendapat nomor MUS dan diajukan ke Head Logistik dan Head Finance & Accounting (nilai > Rp10 jt juga Direksi).",
  },
  {
    action: "revise",
    label: "Ubah Kembali",
    confirmTitle: "Kembalikan ke draft?",
    confirmDescription: "Dokumen yang ditolak bisa diubah dan diajukan ulang.",
  },
  {
    action: "post",
    label: "Posting",
    primary: true,
    confirmTitle: "Posting pemusnahan?",
    confirmDescription:
      "Barang keluar dari stok dan dijurnal (A-26 barang retur, A-27 kedaluwarsa/rusak). Berita acara harus lengkap.",
  },
  {
    action: "cancel",
    label: "Batalkan",
    destructive: true,
    confirmTitle: "Batalkan pemusnahan?",
    confirmDescription: "Dokumen dibatalkan dan tidak bisa diproses lagi.",
  },
]

/** Detail Pemusnahan dengan berita acara. */
export function DisposalDetailPage({ id }: { id: string }) {
  const doc = useQuery(disposalQuery(id))
  return (
    <LoadingArea loading={doc.isPending}>
      {doc.data ? <DisposalDetail doc={doc.data} /> : null}
    </LoadingArea>
  )
}

function DisposalDetail({ doc: d }: { doc: Disposal }) {
  const search = detailRoute.useSearch()
  const navigate = detailRoute.useNavigate()
  const [reporting, setReporting] = useState(false)
  return (
    <div className="flex flex-col gap-4">
      <DocHeader
        title={d.number ?? "Draft Pemusnahan"}
        status={d.status}
        subtitle={d.reasonLabel}
      >
        <Button
          variant="outline"
          render={
            <Link to="/inventory/disposals/$id/print" params={{ id: d.id }} />
          }
        >
          <PrinterIcon /> Berita Acara
        </Button>
        {d.allowedActions.includes("edit") ? (
          <Button
            variant="outline"
            render={
              <Link to="/inventory/disposals/$id/edit" params={{ id: d.id }} />
            }
          >
            Ubah
          </Button>
        ) : null}
        {d.allowedActions.includes("report") ? (
          <Button variant="outline" onClick={() => setReporting(true)}>
            Isi Berita Acara
          </Button>
        ) : null}
        <DocActionButtons
          specs={DISPOSAL_ACTIONS}
          allowed={d.allowedActions}
          run={(a, key) => runDisposalAction(d.id, a, d.version, key)}
        />
      </DocHeader>
      <DocTabs
        tab={search.tab}
        onTabChange={(tab: DocTab) => void navigate({ search: { tab } })}
        withApproval={!d.returnDispositionId}
        entityType="disposal"
        entityId={d.id}
      >
        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <Info label="Tanggal Pemusnahan" value={formatDate(d.disposalDate)} />
          <Info label="Metode" value={d.method} />
          <Info label="Saksi" value={d.witnesses.join(", ")} />
          <Info
            label="Foto"
            value={d.photos.length ? `${d.photos.length} foto` : ""}
          />
          <Info label="Estimasi HPP" value={formatRupiah(d.estimatedCost)} />
          <Info label="HPP Dimusnahkan" value={formatRupiah(d.costAmount)} />
          <Info label="Diajukan" value={stepText(d.submitted)} />
          <Info label="Diposting" value={stepText(d.posted)} />
          {d.journalId ? (
            <Info
              label="Jurnal"
              value={
                <Link
                  to="/finance/journals/$id"
                  params={{ id: d.journalId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat jurnal {d.postingRuleCode}
                </Link>
              }
            />
          ) : null}
          {d.note ? <Info label="Catatan" value={d.note} /> : null}
        </dl>
        <DisposalLinesTable doc={d} />
        <h2 className="font-medium">Mutasi Stok</h2>
        <DocMovementsTable movements={d.movements} />
      </DocTabs>
      <Dialog open={reporting} onOpenChange={setReporting}>
        {reporting ? (
          <ReportDialog doc={d} onOpenChange={setReporting} />
        ) : null}
      </Dialog>
    </div>
  )
}

function DisposalLinesTable({ doc }: { doc: Disposal }) {
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[44rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Produk</TableHead>
            <TableHead>Batch</TableHead>
            <TableHead>Lokasi</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Qty</TableHead>
            <TableHead className="text-right">Nilai HPP</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {doc.lines.map((l) => (
            <TableRow key={l.id}>
              <TableCell>
                <span className="tabular-nums">{l.product.sku}</span>{" "}
                {l.product.name}
              </TableCell>
              <TableCell>
                {l.batch
                  ? `${l.batch.number}${l.batch.expiryDate ? ` · ${formatDate(l.batch.expiryDate)}` : ""}`
                  : "FEFO"}
              </TableCell>
              <TableCell>{l.location.code}</TableCell>
              <TableCell>
                <StockStatusBadge status={l.stockStatus} />
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {l.qty} {l.product.uom}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRupiah(l.costAmount)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

function ReportDialog({
  doc,
  onOpenChange,
}: {
  doc: Disposal
  onOpenChange: (o: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [date, setDate] = useState(doc.disposalDate)
  const [method, setMethod] = useState(doc.method)
  const [witnesses, setWitnesses] = useState(doc.witnesses.join("\n"))
  const [photos, setPhotos] = useState<Photo[]>(doc.photos)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      updateDisposalReport(
        doc.id,
        {
          version: doc.version,
          disposalDate: date,
          method,
          witnesses: splitWitnesses(witnesses),
          photoFileIds: photos.map((p) => p.id),
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Berita acara disimpan")
      onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
    },
    onError: (err) => handleFormError(err, setErrors),
  })
  return (
    <DialogContent className="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Berita Acara Pemusnahan</DialogTitle>
        <DialogDescription>
          Metode, minimal satu saksi, dan minimal satu foto wajib diisi sebelum
          posting.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <ReportFields
          date={date}
          setDate={setDate}
          method={method}
          setMethod={setMethod}
          witnesses={witnesses}
          setWitnesses={setWitnesses}
          photos={photos}
          setPhotos={setPhotos}
          errors={errors}
          setErrors={setErrors}
        />
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

/** Cetak Berita Acara Pemusnahan dari browser (Desain Dokumen Cetak 2.2). */
export function DisposalPrintPage({ id }: { id: string }) {
  const doc = useQuery(disposalQuery(id))
  return (
    <LoadingArea loading={doc.isPending}>
      {doc.data ? <DisposalPrint doc={doc.data} /> : null}
    </LoadingArea>
  )
}

function DisposalPrint({ doc: d }: { doc: Disposal }) {
  const photos = useQueries({
    queries: d.photos.map((p) => fileUrlQuery(p.id)),
  })
  const location = d.lines[0]?.location
  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2 print:hidden">
        <Button onClick={() => window.print()}>
          <PrinterIcon /> Cetak
        </Button>
        <Button
          variant="outline"
          render={
            <Link
              to="/inventory/disposals/$id"
              params={{ id: d.id }}
              search={{ tab: "detail" }}
            />
          }
        >
          Kembali
        </Button>
      </div>
      <article
        className="mx-auto w-full max-w-3xl bg-background text-sm print:max-w-none"
        aria-label="Berita Acara Pemusnahan"
      >
        <header className="border-b pb-3 text-center">
          <p className="font-semibold">Balans Nutrition</p>
          <h1 className="text-xl font-bold tracking-tight">
            BERITA ACARA PEMUSNAHAN
          </h1>
          <p>Nomor: {d.number ?? "(belum bernomor)"}</p>
        </header>
        <dl className="my-4 grid grid-cols-[10rem_1fr] gap-y-1">
          <dt>Tanggal</dt>
          <dd>: {formatDate(d.disposalDate)}</dd>
          <dt>Alasan</dt>
          <dd>: {d.reasonLabel}</dd>
          <dt>Lokasi</dt>
          <dd>: {location ? `${location.code} · ${location.name}` : "—"}</dd>
          <dt>Metode</dt>
          <dd>: {d.method || "—"}</dd>
          <dt>Saksi</dt>
          <dd>: {d.witnesses.join(", ") || "—"}</dd>
        </dl>
        <DisposalLinesTable doc={d} />
        <p className="mt-2 text-right font-medium">
          Total nilai HPP:{" "}
          {formatRupiah(d.status === "posted" ? d.costAmount : d.estimatedCost)}
        </p>
        {photos.length > 0 ? (
          <section className="mt-4 break-inside-avoid">
            <h2 className="mb-2 font-medium">Lampiran Foto</h2>
            <div className="grid grid-cols-2 gap-2">
              {photos.map((p, i) =>
                p.data ? (
                  <img
                    key={d.photos[i].id}
                    src={p.data.url}
                    alt={d.photos[i].name}
                    className="max-h-64 w-full rounded border object-contain"
                  />
                ) : null
              )}
            </div>
          </section>
        ) : null}
        <section className="mt-10 grid grid-cols-2 gap-8 text-center sm:grid-cols-4">
          {["Pelaksana", "Saksi", "Head Logistik", "Finance & Accounting"].map(
            (role) => (
              <div key={role} className="flex flex-col gap-16">
                <span>{role}</span>
                <span className="border-t pt-1">
                  (&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;)
                </span>
              </div>
            )
          )}
        </section>
      </article>
    </div>
  )
}
