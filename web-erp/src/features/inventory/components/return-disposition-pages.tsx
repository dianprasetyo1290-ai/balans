import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { QuantityInput } from "@/components/common/quantity-input"
import { StatusBadge } from "@/components/common/status-badge"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
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
  createReturnDisposition,
  DISPOSITION_LABELS,
  locationStockQuery,
  returnDispositionQuery,
  returnDispositionsQuery,
  returnReasonOptionsQuery,
  runReturnDispositionAction,
  updateReturnDisposition,
  type ReturnDisposition,
  type ReturnDispositionAction,
  type ReturnDispositionDocument,
  type ReturnDispositionDocumentInput,
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

const listRoute = getRouteApi("/_app/warehouse/return-dispositions/")
const detailRoute = getRouteApi("/_app/warehouse/return-dispositions/$id/")
const returnsRoute = getRouteApi("/_app/warehouse/returns")

const DEFAULT = "default"

/** Gudang › Gudang Retur: barang retur di lokasi Retur yang menunggu QC dan disposisi (WMS-07). */
export function ReturnsWarehousePage() {
  const search = returnsRoute.useSearch()
  const navigate = returnsRoute.useNavigate()
  const { can } = usePermissions()
  const stock = useQuery({
    ...locationStockQuery(search.locationId ?? ""),
    enabled: Boolean(search.locationId),
  })
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Gudang Retur
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Barang retur yang sudah masuk lokasi Retur. Lakukan QC dan usulkan
            disposisi per baris lewat Disposisi Retur.
          </p>
        </div>
        {can("warehouse.return_disposition.write") && search.locationId ? (
          <Button
            render={
              <Link
                to="/warehouse/return-dispositions/new"
                search={{ locationId: search.locationId }}
              />
            }
          >
            <PlusIcon /> Buat Disposisi Retur
          </Button>
        ) : null}
      </div>
      <div className="sm:max-w-sm">
        <LocationSelect
          id="returns-location"
          label="Lokasi Retur"
          value={search.locationId}
          onChange={(locationId) => void navigate({ search: { locationId } })}
          types={["returns"]}
        />
      </div>
      {search.locationId ? (
        <LoadingArea loading={stock.isPending}>
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[40rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Produk</TableHead>
                  <TableHead>Batch</TableHead>
                  <TableHead>Kedaluwarsa</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Qty</TableHead>
                  <TableHead className="text-right">Nilai</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(stock.data?.items ?? []).map((b) => (
                  <TableRow
                    key={`${b.product.id}-${b.batchId ?? ""}-${b.status}`}
                  >
                    <TableCell>
                      <span className="tabular-nums">{b.product.sku}</span>{" "}
                      {b.product.name}
                    </TableCell>
                    <TableCell>{b.batchNumber ?? "—"}</TableCell>
                    <TableCell>
                      {b.expiryDate ? formatDate(b.expiryDate) : "—"}
                    </TableCell>
                    <TableCell>
                      <StockStatusBadge status={b.status} />
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {b.qty}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(b.value)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {stock.data?.items.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Tidak ada barang di Gudang Retur ini.
            </p>
          ) : null}
        </LoadingArea>
      ) : (
        <p className="text-sm text-muted-foreground">
          Pilih lokasi Retur untuk melihat barang.
        </p>
      )}
    </div>
  )
}

/** Gudang › Disposisi Retur: daftar. */
export function ReturnDispositionListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const docs = useQuery(returnDispositionsQuery(search))
  const columns: DataTableColumn<ReturnDispositionDocument>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (d) => (
        <Link
          to="/warehouse/return-dispositions/$id"
          params={{ id: d.id }}
          search={{ tab: "detail" }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {d.number ?? "Draft"}
        </Link>
      ),
    },
    { id: "date", header: "Tanggal", cell: (d) => formatDate(d.businessDate) },
    { id: "location", header: "Gudang Retur", cell: (d) => d.location.code },
    {
      id: "summary",
      header: "Disposisi",
      cell: (d) =>
        Array.from(new Set(d.lines.map((l) => l.dispositionLabel))).join(", "),
    },
    {
      id: "disposal",
      header: "Nilai Pemusnahan",
      align: "right",
      cell: (d) => formatRupiah(d.disposalValue),
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
          <h1 className="text-2xl font-semibold tracking-tight">
            Disposisi Retur
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            QC dan usulan disposisi barang retur per baris (kembali ke stok,
            repacking, reprocess, sampel, pemusnahan), persetujuan Head
            Logistik, lalu eksekusi.
          </p>
        </div>
        {can("warehouse.return_disposition.write") ? (
          <Button
            render={
              <Link to="/warehouse/return-dispositions/new" search={{}} />
            }
          >
            <PlusIcon /> Buat Disposisi Retur
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
        emptyMessage="Belum ada Disposisi Retur."
      />
    </div>
  )
}

type Line = {
  productId: string
  label: string
  batchId?: string
  batchNumber?: string
  stockStatus: "available" | "quarantine"
  qty: string
  reasonId: string
  disposition: ReturnDisposition | ""
  qcNote: string
}

/** Form Buat/Ubah Disposisi Retur. */
export function ReturnDispositionFormPage({
  id,
  locationId,
}: {
  id?: string
  locationId?: string
}) {
  const doc = useQuery({
    ...returnDispositionQuery(id ?? ""),
    enabled: Boolean(id),
  })
  if (id && !doc.data) {
    return <LoadingArea loading>{null}</LoadingArea>
  }
  return <ReturnDispositionForm doc={doc.data} initialLocation={locationId} />
}

function ReturnDispositionForm({
  doc,
  initialLocation,
}: {
  doc?: ReturnDispositionDocument
  initialLocation?: string
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const reasons = useQuery(returnReasonOptionsQuery)
  const [locationId, setLocationId] = useState(
    doc?.location.id ?? initialLocation ?? ""
  )
  const [businessDate, setBusinessDate] = useState(
    doc?.businessDate ?? todayBusinessDate()
  )
  const [note, setNote] = useState(doc?.note ?? "")
  const [lines, setLines] = useState<Line[]>(
    doc?.lines.map((l) => ({
      productId: l.product.id,
      label: `${l.product.sku} ${l.product.name}`,
      batchId: l.batch?.id,
      batchNumber: l.batch?.number,
      stockStatus: l.stockStatus,
      qty: l.qty,
      reasonId: l.reason?.id ?? "",
      disposition: l.disposition,
      qcNote: l.qcNote,
    })) ?? []
  )
  const [picking, setPicking] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const reasonOptions = (reasons.data ?? [])
    .filter((r) => r.isActive)
    .map((r) => ({ value: r.id, label: `${r.code} · ${r.name}` }))
  const update = (i: number, patch: Partial<Line>) =>
    setLines((ls) => ls.map((x, j) => (j === i ? { ...x, ...patch } : x)))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: ReturnDispositionDocumentInput = {
        locationId,
        businessDate,
        note: note || undefined,
        lines: lines.map((l) => ({
          productId: l.productId,
          batchId: l.batchId,
          stockStatus: l.stockStatus,
          qty: l.qty || "0",
          reasonId: l.reasonId || undefined,
          disposition: l.disposition || undefined,
          qcNote: l.qcNote || undefined,
        })),
      }
      return doc
        ? updateReturnDisposition(
            doc.id,
            { ...body, version: doc.version },
            key
          )
        : createReturnDisposition(body, key)
    },
    onSuccess: async (d) => {
      toast.success("Disposisi Retur disimpan")
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
      void navigate({
        to: "/warehouse/return-dispositions/$id",
        params: { id: d.id },
        search: { tab: "detail" },
      })
    },
    onError: (err) => handleFormError(err, setErrors),
  })

  return (
    <form
      noValidate
      className="flex max-w-5xl flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        setErrors({})
        save.mutate()
      }}
    >
      <h1 className="text-2xl font-semibold tracking-tight">
        {doc
          ? `Ubah ${doc.number ?? "Draft Disposisi Retur"}`
          : "Buat Disposisi Retur"}
      </h1>
      <div className="grid gap-4 sm:grid-cols-3">
        <div>
          <LocationSelect
            id="dr-location"
            label="Gudang Retur"
            value={locationId}
            onChange={(v) => {
              setLocationId(v)
              setLines([])
            }}
            types={["returns"]}
            invalid={!!errors.locationId}
          />
          <FieldError errors={fieldErrors(errors, "locationId")} />
        </div>
        <Field data-invalid={!!errors.businessDate}>
          <FieldLabel htmlFor="dr-date">Tanggal</FieldLabel>
          <DateInput
            id="dr-date"
            value={businessDate}
            onValueChange={setBusinessDate}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="dr-note">Catatan</FieldLabel>
          <Textarea
            id="dr-note"
            value={note}
            maxLength={500}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      <section className="flex flex-col gap-2">
        <div className="flex items-center justify-between gap-2">
          <h2 className="font-medium">Barang Retur, QC &amp; Disposisi</h2>
          <Button
            type="button"
            variant="outline"
            disabled={!locationId}
            onClick={() => setPicking(true)}
          >
            <PlusIcon /> Tambah Barang
          </Button>
        </div>
        <FieldError errors={fieldErrors(errors, "lines")} />
        {lines.map((l, i) => (
          <div
            key={i}
            className="grid gap-3 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-[1fr_8rem_1fr_1fr_auto]"
          >
            <div className="text-sm">
              <div className="font-medium">{l.label}</div>
              <div className="flex items-center gap-2 text-muted-foreground">
                {l.batchNumber ? `Batch ${l.batchNumber}` : "Tanpa batch"}
                <StockStatusBadge status={l.stockStatus} />
              </div>
              <FieldError errors={fieldErrors(errors, `lines[${i}].batchId`)} />
            </div>
            <Field data-invalid={!!errors[`lines[${i}].qty`]}>
              <FieldLabel htmlFor={`dr-qty-${i}`}>Qty</FieldLabel>
              <QuantityInput
                id={`dr-qty-${i}`}
                value={l.qty}
                onValueChange={(v) => update(i, { qty: v })}
              />
              <FieldError errors={fieldErrors(errors, `lines[${i}].qty`)} />
            </Field>
            <div>
              <OptionSelect
                id={`dr-reason-${i}`}
                label="Alasan Retur"
                value={l.reasonId}
                onChange={(v) => update(i, { reasonId: v })}
                options={reasonOptions}
                invalid={!!errors[`lines[${i}].reasonId`]}
                placeholder="Pilih alasan"
              />
              <FieldError
                errors={fieldErrors(errors, `lines[${i}].reasonId`)}
              />
            </div>
            <div>
              <OptionSelect
                id={`dr-disposition-${i}`}
                label="Disposisi"
                value={l.disposition || DEFAULT}
                onChange={(v) =>
                  update(i, {
                    disposition: v === DEFAULT ? "" : (v as ReturnDisposition),
                  })
                }
                options={[
                  { value: DEFAULT, label: "Usulan alasan retur" },
                  ...Object.entries(DISPOSITION_LABELS).map(
                    ([value, label]) => ({ value, label })
                  ),
                ]}
                invalid={!!errors[`lines[${i}].disposition`]}
              />
              <FieldError
                errors={fieldErrors(errors, `lines[${i}].disposition`)}
              />
            </div>
            <Button
              type="button"
              variant="ghost"
              className="self-end"
              aria-label={`Hapus baris ${i + 1}`}
              onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}
            >
              <Trash2Icon />
            </Button>
            <Field className="sm:col-span-2 lg:col-span-5">
              <FieldLabel htmlFor={`dr-qc-${i}`}>
                Hasil QC (kondisi, segel, kedaluwarsa)
              </FieldLabel>
              <Input
                id={`dr-qc-${i}`}
                value={l.qcNote}
                maxLength={500}
                onChange={(e) => update(i, { qcNote: e.target.value })}
              />
            </Field>
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
        locationId={locationId}
        title="Pilih Barang dari Gudang Retur"
        onPick={(b) =>
          setLines((ls) => [
            ...ls,
            {
              productId: b.product.id,
              label: `${b.product.sku} ${b.product.name}`,
              batchId: b.batchId ?? undefined,
              batchNumber: b.batchNumber ?? undefined,
              stockStatus:
                b.status === "quarantine" ? "quarantine" : "available",
              qty: b.qty,
              reasonId: "",
              disposition: "",
              qcNote: "",
            },
          ])
        }
      />
    </form>
  )
}

const DISPOSITION_ACTIONS: ActionSpec<ReturnDispositionAction>[] = [
  {
    action: "submit",
    label: "Ajukan",
    primary: true,
    confirmTitle: "Ajukan disposisi retur?",
    confirmDescription:
      "Dokumen mendapat nomor DRT dan diajukan ke Head Logistik; bila ada pemusnahan juga ke Head Finance & Accounting (> Rp10 jt + Direksi).",
  },
  {
    action: "revise",
    label: "Ubah Kembali",
    confirmTitle: "Kembalikan ke draft?",
    confirmDescription: "Dokumen yang ditolak bisa diubah dan diajukan ulang.",
  },
  {
    action: "post",
    label: "Eksekusi Disposisi",
    primary: true,
    confirmTitle: "Eksekusi disposisi?",
    confirmDescription:
      "Kembali ke stok → FG, reprocess → GM (jurnal A-26); repacking, sampel, dan pemusnahan dibuat sebagai dokumen lanjutan.",
  },
  {
    action: "cancel",
    label: "Batalkan",
    destructive: true,
    confirmTitle: "Batalkan disposisi?",
    confirmDescription: "Draft dibatalkan.",
  },
]

const TARGET_LINKS: Record<string, { to: string; label: string }> = {
  repacking: { to: "/inventory/repackings/$id", label: "Repacking" },
  disposal: { to: "/inventory/disposals/$id", label: "Pemusnahan" },
  internal_usage: {
    to: "/inventory/internal-usages/$id",
    label: "Pemakaian Internal",
  },
}

/** Detail Disposisi Retur. */
export function ReturnDispositionDetailPage({ id }: { id: string }) {
  const doc = useQuery(returnDispositionQuery(id))
  return (
    <LoadingArea loading={doc.isPending}>
      {doc.data ? <ReturnDispositionDetail doc={doc.data} /> : null}
    </LoadingArea>
  )
}

function ReturnDispositionDetail({
  doc: d,
}: {
  doc: ReturnDispositionDocument
}) {
  const search = detailRoute.useSearch()
  const navigate = detailRoute.useNavigate()
  return (
    <div className="flex flex-col gap-4">
      <DocHeader
        title={d.number ?? "Draft Disposisi Retur"}
        status={d.status}
        subtitle={`${d.location.code} · ${d.location.name}`}
      >
        {d.allowedActions.includes("edit") ? (
          <Button
            variant="outline"
            render={
              <Link
                to="/warehouse/return-dispositions/$id/edit"
                params={{ id: d.id }}
              />
            }
          >
            Ubah
          </Button>
        ) : null}
        <DocActionButtons
          specs={DISPOSITION_ACTIONS}
          allowed={d.allowedActions}
          run={(a, key) => runReturnDispositionAction(d.id, a, d.version, key)}
        />
      </DocHeader>
      <DocTabs
        tab={search.tab}
        onTabChange={(tab: DocTab) => void navigate({ search: { tab } })}
        withApproval
        entityType="return_disposition"
        entityId={d.id}
      >
        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <Info label="Tanggal" value={formatDate(d.businessDate)} />
          <Info label="Dibuat" value={d.createdBy.name} />
          <Info label="Diajukan" value={stepText(d.submitted)} />
          <Info label="Dieksekusi" value={stepText(d.posted)} />
          <Info
            label="Nilai Pemusnahan"
            value={formatRupiah(d.disposalValue)}
          />
          <Info
            label="Nilai Restock/Reprocess"
            value={formatRupiah(d.costAmount)}
          />
          {d.journalId ? (
            <Info
              label="Jurnal"
              value={
                <Link
                  to="/finance/journals/$id"
                  params={{ id: d.journalId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat jurnal A-26
                </Link>
              }
            />
          ) : null}
          {d.note ? <Info label="Catatan" value={d.note} /> : null}
        </dl>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[60rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead>Batch</TableHead>
                <TableHead className="text-right">Qty</TableHead>
                <TableHead>Alasan</TableHead>
                <TableHead>Hasil QC</TableHead>
                <TableHead>Disposisi</TableHead>
                <TableHead className="text-right">Estimasi HPP</TableHead>
                <TableHead>Dokumen Lanjutan</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {d.lines.map((l) => {
                const target = l.targetType
                  ? TARGET_LINKS[l.targetType]
                  : undefined
                return (
                  <TableRow key={l.id}>
                    <TableCell>
                      <span className="tabular-nums">{l.product.sku}</span>{" "}
                      {l.product.name}
                    </TableCell>
                    <TableCell>{l.batch?.number ?? "—"}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {l.qty}
                    </TableCell>
                    <TableCell>{l.reason ? l.reason.name : "—"}</TableCell>
                    <TableCell className="max-w-xs truncate">
                      {l.qcNote || "—"}
                    </TableCell>
                    <TableCell>{l.dispositionLabel}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.estimatedCost)}
                    </TableCell>
                    <TableCell>
                      {target && l.targetId ? (
                        <Link
                          to={target.to}
                          params={{ id: l.targetId }}
                          search={{ tab: "detail" }}
                          className="text-primary underline-offset-4 hover:underline"
                        >
                          {target.label}
                        </Link>
                      ) : (
                        "—"
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
        <h2 className="font-medium">Mutasi Stok</h2>
        <DocMovementsTable movements={d.movements} />
      </DocTabs>
    </div>
  )
}
