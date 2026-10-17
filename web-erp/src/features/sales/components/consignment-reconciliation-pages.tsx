import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, PrinterIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { openFile } from "@/features/party/api/party-api"
import type { DetailTab } from "@/features/procurement/components/request-detail-page"
import {
  fieldErrors,
  Info,
  LineError,
  ReasonDialog,
} from "@/features/procurement/components/shared"
import { ApprovalPanel } from "@/features/warehouse/components/document-parts"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  BORNE_BY_LABELS,
  completedConsignmentCountsQuery,
  consignmentPartnersQuery,
  createReconciliation,
  RECONCILIATION_STATUSES,
  reconciliationAction,
  reconciliationQuery,
  reconciliationsQuery,
  requestConsignmentUpload,
  updateReconciliationEntries,
  VARIANCE_CAUSE_LABELS,
  varianceOf,
  type ConsignmentReconciliation,
  type ConsignmentReconciliationSummary,
  type ConsignmentVarianceCause,
  type ReconciliationAction,
  type ReconciliationListSearch,
} from "../api/consignment-api"
import { MAX_PROOF_FILE_BYTES } from "../api/sales-documents-api"

const ALL = "all"
const NONE = "none"

/** Penjualan › Rekonsiliasi Konsinyasi (S-304, S-305). */
export function ReconciliationListPage({
  search,
  onSearchChange,
}: {
  search: ReconciliationListSearch
  onSearchChange: (
    u: (s: ReconciliationListSearch) => ReconciliationListSearch
  ) => void
}) {
  const list = useQuery(reconciliationsQuery(search))
  const [creating, setCreating] = useState(false)
  const columns: DataTableColumn<ConsignmentReconciliationSummary>[] = [
    {
      id: "number",
      header: "Nomor KSN",
      cell: (r) => (
        <Link
          to="/sales/consignment-reconciliations/$id"
          params={{ id: r.id }}
          search={{}}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {r.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "partner",
      header: "Mitra",
      cell: (r) => (
        <div className="flex flex-col">
          <span>{r.partnerName}</span>
          <span className="text-xs text-muted-foreground">{r.partnerCode}</span>
        </div>
      ),
    },
    {
      id: "period",
      header: "Periode",
      cell: (r) =>
        r.periodStart
          ? `${formatDate(r.periodStart)} – ${formatDate(r.cutOffDate)}`
          : `Cut-off ${formatDate(r.cutOffDate)}`,
    },
    { id: "count", header: "Stock Count", cell: (r) => r.stockCountNumber },
    {
      id: "sales",
      header: "Penjualan",
      align: "right",
      cell: (r) => formatRupiah(r.salesAmount),
    },
    {
      id: "variance",
      header: "Selisih Ditagih",
      align: "right",
      cell: (r) => formatRupiah(r.billedVarianceAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (r) => <StatusBadge status={r.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Rekonsiliasi Konsinyasi
          </h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Rekonsiliasi bulanan per mitra: terjual = stok awal + kiriman −
            tarikan − sisa fisik (stock count Merchandiser), dibandingkan
            laporan penjualan mitra. BA bertanda tangan → approval Sales MT →
            Head F&A → invoice konsinyasi otomatis dan selisih diproses sesuai
            PKS.
          </p>
        </div>
        <Button onClick={() => setCreating(true)} className="min-h-11">
          <PlusIcon /> Buat Rekonsiliasi
        </Button>
      </div>
      <Select
        value={search.status ?? ALL}
        onValueChange={(v) =>
          onSearchChange((s) => ({
            ...s,
            status:
              v === ALL || !v
                ? undefined
                : (v as ReconciliationListSearch["status"]),
            page: 1,
          }))
        }
      >
        <SelectTrigger className="w-48" aria-label="Filter status">
          <SelectValue>
            {(v: string) => (v === ALL ? "Semua status" : statusLabel(v).label)}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Semua status</SelectItem>
          {RECONCILIATION_STATUSES.map((s) => (
            <SelectItem key={s} value={s}>
              {statusLabel(s).label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <DataTable
        columns={columns}
        rows={list.data?.items}
        getRowId={(r) => r.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada rekonsiliasi konsinyasi."
      />
      <Dialog open={creating} onOpenChange={setCreating}>
        {creating ? (
          <ReconciliationForm onClose={() => setCreating(false)} />
        ) : null}
      </Dialog>
    </div>
  )
}

function ReconciliationForm({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [partner, setPartner] = useState<PickerOption | null>(null)
  const [cutOffDate, setCutOffDate] = useState(todayBusinessDate())
  const [stockCount, setStockCount] = useState<PickerOption | null>(null)
  const [note, setNote] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createReconciliation(
        {
          partnerId: partner?.id ?? "",
          cutOffDate,
          stockCountId: stockCount?.id ?? "",
          note: note || undefined,
        },
        key
      ),
    onSuccess: async (r) => {
      toast.success("Draft rekonsiliasi dibuat")
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
      onClose()
      void navigate({
        to: "/sales/consignment-reconciliations/$id",
        params: { id: r.id },
        search: {},
      })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields)
        return
      }
      showErrorDialog(problem)
    },
  })
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Buat Rekonsiliasi Konsinyasi</DialogTitle>
        <DialogDescription>
          Pilih mitra, tanggal cut-off, dan stock count lokasi mitra yang sudah
          Selesai (paling jauh 7 hari dari cut-off).
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          const next: Record<string, string> = {}
          if (!partner) next.partnerId = "Pilih mitra konsinyasi"
          if (!stockCount) next.stockCountId = "Pilih stock count lokasi mitra"
          setErrors(next)
          if (Object.keys(next).length === 0) save.mutate()
        }}
      >
        <Field data-invalid={!!errors.partnerId}>
          <FieldLabel htmlFor="rk-partner">Mitra Konsinyasi</FieldLabel>
          <SearchPicker
            id="rk-partner"
            value={partner}
            onValueChange={setPartner}
            query={(q) => consignmentPartnersQuery(q)}
            toOption={(p) => ({ id: p.id, label: p.name, description: p.code })}
            placeholder="Pilih mitra"
            searchPlaceholder="Cari kode atau nama mitra"
            aria-invalid={!!errors.partnerId}
          />
          <FieldError errors={fieldErrors(errors.partnerId)} />
        </Field>
        <Field data-invalid={!!errors.cutOffDate}>
          <FieldLabel htmlFor="rk-cutoff">Tanggal Cut-off</FieldLabel>
          <DateInput
            id="rk-cutoff"
            value={cutOffDate}
            onValueChange={setCutOffDate}
          />
          <FieldError errors={fieldErrors(errors.cutOffDate)} />
        </Field>
        <Field data-invalid={!!errors.stockCountId}>
          <FieldLabel htmlFor="rk-count">Stock Count Lokasi Mitra</FieldLabel>
          <SearchPicker
            id="rk-count"
            value={stockCount}
            onValueChange={setStockCount}
            query={(q) => completedConsignmentCountsQuery(q)}
            toOption={(c) => ({
              id: c.id,
              label: c.number ?? "Stock Count",
              description: `${c.location.code} – ${c.location.name} · ${formatDate(c.businessDate)}`,
            })}
            placeholder="Pilih stock count"
            searchPlaceholder="Cari nomor SC"
            aria-invalid={!!errors.stockCountId}
          />
          <FieldError errors={fieldErrors(errors.stockCountId)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="rk-note">Catatan (opsional)</FieldLabel>
          <Textarea
            id="rk-note"
            value={note}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton type="submit" loading={save.isPending}>
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

const ACTION_TEXT: Record<
  Exclude<ReconciliationAction, "cancel">,
  { label: string; toast: string }
> = {
  start: { label: "Mulai Rekonsiliasi", toast: "Rekonsiliasi dimulai" },
  submit: { label: "Ajukan Persetujuan", toast: "Diajukan untuk persetujuan" },
  revise: { label: "Revisi", toast: "Rekonsiliasi dibuka kembali" },
}

type Entry = { reportedQty: string; cause: ConsignmentVarianceCause | null }

/** Detail Rekonsiliasi Konsinyasi: isi laporan mitra & penyebab selisih, unggah BA, ajukan, cetak BA. */
export function ReconciliationDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const q = useQuery(reconciliationQuery(id))
  return (
    <LoadingArea loading={q.isPending}>
      {q.data ? (
        <ReconciliationDetail
          key={`${q.data.id}-${q.data.version}`}
          rec={q.data}
          tab={tab}
          onTabChange={onTabChange}
        />
      ) : null}
    </LoadingArea>
  )
}

function ReconciliationDetail({
  rec,
  tab,
  onTabChange,
}: {
  rec: ConsignmentReconciliation
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const queryClient = useQueryClient()
  const editable = rec.allowedActions.includes("update_lines")
  const [entries, setEntries] = useState<Record<string, Entry>>(() =>
    Object.fromEntries(
      rec.lines.map((l) => [
        l.id,
        { reportedQty: l.reportedQty, cause: l.varianceCause ?? null },
      ])
    )
  )
  const [note, setNote] = useState(rec.note)
  const [baFileId, setBaFileId] = useState<string | null>(rec.baFileId ?? null)
  const [reportFileId, setReportFileId] = useState<string | null>(
    rec.partnerReportFileId ?? null
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [cancelling, setCancelling] = useState(false)
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["sales"] })
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      updateReconciliationEntries(
        rec.id,
        {
          version: rec.version,
          lines: rec.lines.map((l) => ({
            lineId: l.id,
            reportedQty: entries[l.id]?.reportedQty ?? l.reportedQty,
            varianceCause: entries[l.id]?.cause ?? null,
          })),
          setFiles: true,
          baFileId,
          partnerReportFileId: reportFileId,
          note,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Rekonsiliasi disimpan")
      setErrors({})
      await invalidate()
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields)
        return
      }
      showErrorDialog(problem)
    },
  })
  const act = useIdempotentMutation({
    mutationFn: (v: { action: ReconciliationAction; reason?: string }, key) =>
      reconciliationAction(rec.id, v.action, rec.version, key, v.reason),
    onSuccess: async (_, v) => {
      toast.success(
        v.action === "cancel"
          ? "Rekonsiliasi dibatalkan"
          : ACTION_TEXT[v.action].toast
      )
      setCancelling(false)
      await invalidate()
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields)
        toast.error(problem.detail ?? "Periksa isian rekonsiliasi")
        return
      }
      showErrorDialog(problem)
    },
  })
  const lineErr = (i: number, f: string) => errors[`lines.${i}.${f}`]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              Rekonsiliasi {rec.number ?? "Draft"}
            </h1>
            <StatusBadge status={rec.status} />
          </div>
          <p className="text-sm text-muted-foreground">
            {rec.partnerName} · cut-off {formatDate(rec.cutOffDate)}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {rec.allowedActions.includes("print") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/sales/consignment-reconciliations/$id/print"
                  params={{ id: rec.id }}
                />
              }
            >
              <PrinterIcon /> Cetak Berita Acara
            </Button>
          ) : null}
          {editable ? (
            <LoadingButton
              variant="outline"
              loading={save.isPending}
              onClick={() => save.mutate()}
            >
              Simpan
            </LoadingButton>
          ) : null}
          {(["start", "submit", "revise"] as const)
            .filter((a) => rec.allowedActions.includes(a))
            .map((a) => (
              <LoadingButton
                key={a}
                className="min-h-11"
                loading={act.isPending && act.variables?.action === a}
                onClick={() => act.mutate({ action: a })}
              >
                {ACTION_TEXT[a].label}
              </LoadingButton>
            ))}
          {rec.allowedActions.includes("cancel") ? (
            <Button variant="outline" onClick={() => setCancelling(true)}>
              Batalkan
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian rekonsiliasi"
        value={tab}
        onValueChange={onTabChange}
        tabs={[
          { id: "summary", label: "Ringkasan" },
          { id: "history", label: "Riwayat" },
        ]}
      />
      {tab === "summary" ? (
        <>
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info
              label="Periode"
              value={
                rec.periodStart
                  ? `${formatDate(rec.periodStart)} – ${formatDate(rec.cutOffDate)}`
                  : "Ditentukan saat mulai"
              }
            />
            <Info label="PKS" value={rec.agreementNumber || "—"} />
            <Info
              label="Termin"
              value={rec.agreementNumber ? `${rec.paymentTermDays} hari` : "—"}
            />
            <Info
              label="Lokasi Mitra"
              value={`${rec.location.code} – ${rec.location.name}`}
            />
            <Info label="Stock Count" value={rec.stockCountNumber} />
            <Info
              label="Ketentuan Selisih"
              value={`Hilang: ${BORNE_BY_LABELS[rec.varianceRules.lost]} · Rusak mitra: ${BORNE_BY_LABELS[rec.varianceRules.partnerDamage]} · Cacat produk: ${BORNE_BY_LABELS[rec.varianceRules.productDefect]} · Mendekati ED: ${BORNE_BY_LABELS[rec.varianceRules.nearExpiry]}`}
            />
            <Info label="Penjualan" value={formatRupiah(rec.salesAmount)} />
            <Info
              label="Selisih Ditagih"
              value={formatRupiah(rec.billedVarianceAmount)}
            />
            {rec.status === "posted" ? (
              <>
                <Info
                  label="Selisih Ditanggung Balans (HPP)"
                  value={formatRupiah(rec.absorbedVarianceCost)}
                />
                <Info
                  label="Invoice"
                  value={
                    rec.invoiceId ? (
                      <Link
                        to="/sales/invoices/$id"
                        params={{ id: rec.invoiceId }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {rec.invoiceNumber}
                      </Link>
                    ) : (
                      "—"
                    )
                  }
                />
                <Info
                  label="Invoice Selisih"
                  value={
                    rec.varianceInvoiceId ? (
                      <Link
                        to="/sales/invoices/$id"
                        params={{ id: rec.varianceInvoiceId }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {rec.varianceInvoiceNumber}
                      </Link>
                    ) : (
                      "—"
                    )
                  }
                />
              </>
            ) : null}
            {rec.decisionNote ? (
              <Info label="Catatan Persetujuan" value={rec.decisionNote} />
            ) : null}
            {rec.cancelReason ? (
              <Info label="Alasan dibatalkan" value={rec.cancelReason} />
            ) : null}
          </dl>
          {rec.lines.length > 0 ? (
            <div className="overflow-x-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Produk</TableHead>
                    <TableHead className="text-right">Stok Awal</TableHead>
                    <TableHead className="text-right">Kiriman</TableHead>
                    <TableHead className="text-right">Tarikan</TableHead>
                    <TableHead className="text-right">Sisa Fisik</TableHead>
                    <TableHead className="text-right">Terjual Sistem</TableHead>
                    <TableHead className="w-36">Laporan Mitra</TableHead>
                    <TableHead className="text-right">Selisih</TableHead>
                    <TableHead className="w-48">Penyebab</TableHead>
                    <TableHead className="text-right">Harga</TableHead>
                    <TableHead className="text-right">Tagihan</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rec.lines.map((l, i) => {
                    const e = entries[l.id] ?? {
                      reportedQty: l.reportedQty,
                      cause: null,
                    }
                    const variance = editable
                      ? varianceOf(l.soldQty, e.reportedQty)
                      : Number(l.varianceQty)
                    return (
                      <TableRow key={l.id}>
                        <TableCell className="min-w-48">
                          {l.product.sku} – {l.product.name}
                          {Number(l.soldQty) < 0 ? (
                            <LineError message="Sisa fisik melebihi saldo sistem" />
                          ) : null}
                          <LineError message={lineErr(i, "countedQty")} />
                        </TableCell>
                        <TableCell className="text-right">
                          {formatQuantity(l.openingQty)}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatQuantity(l.deliveredQty)}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatQuantity(l.returnedQty)}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatQuantity(l.countedQty)}
                        </TableCell>
                        <TableCell className="text-right font-medium">
                          {formatQuantity(l.soldQty)}
                        </TableCell>
                        <TableCell>
                          {editable ? (
                            <>
                              <QuantityInput
                                aria-label={`Laporan mitra ${l.product.sku}`}
                                value={e.reportedQty}
                                aria-invalid={!!lineErr(i, "reportedQty")}
                                onValueChange={(v) =>
                                  setEntries((s) => ({
                                    ...s,
                                    [l.id]: { ...e, reportedQty: v },
                                  }))
                                }
                              />
                              <LineError message={lineErr(i, "reportedQty")} />
                            </>
                          ) : (
                            formatQuantity(l.reportedQty)
                          )}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatQuantity(String(variance))}
                        </TableCell>
                        <TableCell>
                          {editable && variance > 0 ? (
                            <>
                              <Select
                                value={e.cause ?? NONE}
                                onValueChange={(v) =>
                                  setEntries((s) => ({
                                    ...s,
                                    [l.id]: {
                                      ...e,
                                      cause:
                                        v === NONE || !v
                                          ? null
                                          : (v as ConsignmentVarianceCause),
                                    },
                                  }))
                                }
                              >
                                <SelectTrigger
                                  className="w-full"
                                  aria-label={`Penyebab selisih ${l.product.sku}`}
                                  aria-invalid={!!lineErr(i, "varianceCause")}
                                >
                                  <SelectValue>
                                    {(v: string) =>
                                      v === NONE
                                        ? "Pilih penyebab"
                                        : VARIANCE_CAUSE_LABELS[
                                            v as ConsignmentVarianceCause
                                          ]
                                    }
                                  </SelectValue>
                                </SelectTrigger>
                                <SelectContent>
                                  {(
                                    Object.keys(
                                      VARIANCE_CAUSE_LABELS
                                    ) as ConsignmentVarianceCause[]
                                  ).map((c) => (
                                    <SelectItem key={c} value={c}>
                                      {VARIANCE_CAUSE_LABELS[c]}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                              <LineError
                                message={lineErr(i, "varianceCause")}
                              />
                            </>
                          ) : l.varianceCause ? (
                            <div className="flex flex-col">
                              <span>
                                {VARIANCE_CAUSE_LABELS[l.varianceCause]}
                              </span>
                              {l.borneBy ? (
                                <span className="text-xs text-muted-foreground">
                                  {BORNE_BY_LABELS[l.borneBy]}
                                </span>
                              ) : null}
                            </div>
                          ) : (
                            "—"
                          )}
                        </TableCell>
                        <TableCell
                          className="text-right"
                          title={l.priceDescription}
                        >
                          {formatRupiah(l.unitPrice)}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatRupiah(
                            (
                              Number(l.salesAmount) + Number(l.varianceAmount)
                            ).toFixed(2)
                          )}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell colSpan={10}>Total tagihan</TableCell>
                    <TableCell className="text-right">
                      {formatRupiah(
                        (
                          Number(rec.salesAmount) +
                          Number(rec.billedVarianceAmount)
                        ).toFixed(2)
                      )}
                    </TableCell>
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              Baris SKU dihitung saat rekonsiliasi dimulai.
            </p>
          )}
          {editable ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <Field data-invalid={!!errors.baFileId}>
                <FileUpload
                  label={
                    rec.baFileName
                      ? `Ganti BA bertanda tangan (${rec.baFileName})`
                      : "Unggah BA bertanda tangan mitra"
                  }
                  accept="image/jpeg,image/png,image/webp,application/pdf"
                  maxSizeBytes={MAX_PROOF_FILE_BYTES}
                  requestUpload={(f) =>
                    requestConsignmentUpload(f, "sales.consignment_ba")
                  }
                  onUploaded={(fid) => setBaFileId(fid)}
                  onRemoved={() => setBaFileId(rec.baFileId ?? null)}
                  onError={(m) => toast.error(m)}
                />
                <FieldError errors={fieldErrors(errors.baFileId)} />
              </Field>
              <FileUpload
                label={
                  rec.partnerReportFileName
                    ? `Ganti laporan mitra (${rec.partnerReportFileName})`
                    : "Unggah laporan penjualan mitra (opsional)"
                }
                accept="image/jpeg,image/png,image/webp,application/pdf"
                maxSizeBytes={MAX_PROOF_FILE_BYTES}
                requestUpload={(f) =>
                  requestConsignmentUpload(
                    f,
                    "sales.consignment_partner_report"
                  )
                }
                onUploaded={(fid) => setReportFileId(fid)}
                onRemoved={() =>
                  setReportFileId(rec.partnerReportFileId ?? null)
                }
                onError={(m) => toast.error(m)}
              />
              <Field className="sm:col-span-2">
                <FieldLabel htmlFor="rk-detail-note">Catatan</FieldLabel>
                <Textarea
                  id="rk-detail-note"
                  value={note}
                  maxLength={1000}
                  onChange={(e) => setNote(e.target.value)}
                />
              </Field>
            </div>
          ) : (
            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
              <Info
                label="BA bertanda tangan"
                value={
                  rec.baFileId ? (
                    <button
                      type="button"
                      className="text-primary underline-offset-4 hover:underline"
                      onClick={() => void openFile(rec.baFileId ?? "")}
                    >
                      {rec.baFileName}
                    </button>
                  ) : (
                    "—"
                  )
                }
              />
              <Info
                label="Laporan mitra"
                value={
                  rec.partnerReportFileId ? (
                    <button
                      type="button"
                      className="text-primary underline-offset-4 hover:underline"
                      onClick={() =>
                        void openFile(rec.partnerReportFileId ?? "")
                      }
                    >
                      {rec.partnerReportFileName}
                    </button>
                  ) : (
                    "—"
                  )
                }
              />
              {rec.note ? <Info label="Catatan" value={rec.note} /> : null}
            </dl>
          )}
          <ApprovalPanel
            subjectType="consignment_reconciliation"
            subjectId={rec.id}
          />
        </>
      ) : (
        <EntityHistory
          entityType="consignment_reconciliation"
          entityId={rec.id}
        />
      )}
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan rekonsiliasi"
        description="Rekonsiliasi dibatalkan; stock count bisa dipakai lagi."
        confirmLabel="Batalkan Rekonsiliasi"
        required
        destructive
        loading={act.isPending}
        onConfirm={(reason) => act.mutate({ action: "cancel", reason })}
      />
    </div>
  )
}

const PRINT_STYLES = `
@page { size: A4 landscape; margin: 10mm; }
@media print {
  body * { visibility: hidden !important; }
  #ksn-print-root, #ksn-print-root * { visibility: visible !important; }
  #ksn-print-root { position: absolute; inset: 0 auto auto 0; width: 100%; margin: 0; border: 0; box-shadow: none; }
}
`

/** Cetak Berita Acara Rekonsiliasi Konsinyasi dari browser (Desain Dokumen Cetak 2.2). */
export function ReconciliationPrintPage({ id }: { id: string }) {
  const q = useQuery(reconciliationQuery(id))
  const r = q.data
  return (
    <LoadingArea loading={q.isPending}>
      {r ? (
        <div className="flex flex-col gap-4">
          <style>{PRINT_STYLES}</style>
          <div className="flex justify-end print:hidden">
            <Button onClick={() => window.print()}>
              <PrinterIcon /> Cetak
            </Button>
          </div>
          <article
            id="ksn-print-root"
            className="mx-auto w-full max-w-5xl rounded-lg border bg-white p-6 text-sm text-black"
          >
            <header className="flex flex-wrap justify-between gap-4 border-b pb-4">
              <div>
                <p className="text-lg font-semibold">Balans Nutrition</p>
                <p>NON-PKP</p>
              </div>
              <div className="text-right">
                <h1 className="text-xl font-semibold">
                  Berita Acara Rekonsiliasi Konsinyasi
                </h1>
                <p>{r.number ?? "Draft"}</p>
              </div>
            </header>
            <dl className="grid gap-x-6 gap-y-1 py-4 sm:grid-cols-2">
              <Info
                label="Mitra"
                value={`${r.partnerName} (${r.partnerCode})`}
              />
              <Info label="Alamat" value={r.partnerAddress} />
              <Info
                label="Periode"
                value={
                  r.periodStart
                    ? `${formatDate(r.periodStart)} – ${formatDate(r.cutOffDate)}`
                    : formatDate(r.cutOffDate)
                }
              />
              <Info
                label="PKS"
                value={`${r.agreementNumber} · termin ${r.paymentTermDays} hari`}
              />
              <Info label="Stock Count" value={r.stockCountNumber} />
              <Info
                label="Lokasi"
                value={`${r.location.code} – ${r.location.name}`}
              />
            </dl>
            <table className="w-full border-collapse text-xs">
              <thead>
                <tr className="border-y">
                  {[
                    "SKU / Produk",
                    "Stok Awal",
                    "Kiriman",
                    "Tarikan",
                    "Sisa Fisik",
                    "Terjual Sistem",
                    "Laporan Mitra",
                    "Selisih",
                    "Penyebab",
                    "Ditanggung Oleh",
                    "Harga",
                    "Tagihan",
                  ].map((h) => (
                    <th key={h} className="p-1 text-left font-medium">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {r.lines.map((l) => (
                  <tr key={l.id} className="border-b">
                    <td className="p-1">
                      {l.product.sku} – {l.product.name}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.openingQty)}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.deliveredQty)}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.returnedQty)}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.countedQty)}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.soldQty)}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.reportedQty)}
                    </td>
                    <td className="p-1 text-right">
                      {formatQuantity(l.varianceQty)}
                    </td>
                    <td className="p-1">
                      {l.varianceCause
                        ? VARIANCE_CAUSE_LABELS[l.varianceCause]
                        : "—"}
                    </td>
                    <td className="p-1">
                      {l.borneBy ? BORNE_BY_LABELS[l.borneBy] : "—"}
                    </td>
                    <td className="p-1 text-right">
                      {formatRupiah(l.unitPrice)}
                    </td>
                    <td className="p-1 text-right">
                      {formatRupiah(
                        (
                          Number(l.salesAmount) + Number(l.varianceAmount)
                        ).toFixed(2)
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr>
                  <td colSpan={11} className="p-1 text-right font-medium">
                    Penjualan konsinyasi
                  </td>
                  <td className="p-1 text-right">
                    {formatRupiah(r.salesAmount)}
                  </td>
                </tr>
                <tr>
                  <td colSpan={11} className="p-1 text-right font-medium">
                    Selisih ditagih ke mitra
                  </td>
                  <td className="p-1 text-right">
                    {formatRupiah(r.billedVarianceAmount)}
                  </td>
                </tr>
                <tr className="border-t">
                  <td colSpan={11} className="p-1 text-right font-semibold">
                    Nilai tagihan
                  </td>
                  <td className="p-1 text-right font-semibold">
                    {formatRupiah(
                      (
                        Number(r.salesAmount) + Number(r.billedVarianceAmount)
                      ).toFixed(2)
                    )}
                  </td>
                </tr>
              </tfoot>
            </table>
            <p className="pt-4 text-xs text-muted-foreground">
              Terjual sistem = stok awal + kiriman − tarikan − sisa fisik.
              Selisih = terjual sistem − laporan mitra, diproses sesuai
              ketentuan PKS.
            </p>
            <div className="grid grid-cols-2 gap-8 pt-10 text-center">
              {["Mitra", "Sales MT Balans"].map((s) => (
                <div key={s} className="flex flex-col gap-16">
                  <span>{s}</span>
                  <span className="border-t pt-1">
                    Nama, tanda tangan & tanggal
                  </span>
                </div>
              ))}
            </div>
          </article>
        </div>
      ) : null}
    </LoadingArea>
  )
}
