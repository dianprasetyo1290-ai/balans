import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { CopyIcon, PrinterIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import {
  fieldErrors,
  Info,
  OptionSelect,
} from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  cashBankAccountsQuery,
  confirmProformaPayment,
  INVOICE_STATUSES,
  invoiceQuery,
  invoiceShareText,
  invoicesQuery,
  MAX_PROOF_FILE_BYTES,
  ORDER_TYPE_LABELS,
  PAYMENT_METHOD_LABELS,
  proformaQuery,
  proformasQuery,
  PROFORMA_STATUSES,
  requestProofUpload,
  type CustomerPaymentMethod,
  type InvoiceListSearch,
  type ProformaInvoice,
  type ProformaInvoiceDetail,
  type ProformaListSearch,
  type SalesDocumentStatus,
  type SalesInvoice,
  type SalesInvoiceSummary,
} from "../api/sales-documents-api"
import { overdueLabel } from "../lib/sales-lib"

const ALL = "all"

function StatusFilter({
  value,
  statuses,
  onChange,
}: {
  value?: SalesDocumentStatus
  statuses: SalesDocumentStatus[]
  onChange: (s: SalesDocumentStatus | undefined) => void
}) {
  return (
    <Select
      value={value ?? ALL}
      onValueChange={(v) =>
        onChange(v === ALL || !v ? undefined : (v as SalesDocumentStatus))
      }
    >
      <SelectTrigger className="w-48" aria-label="Filter status">
        <SelectValue>
          {(v: string) => (v === ALL ? "Semua status" : statusLabel(v).label)}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={ALL}>Semua status</SelectItem>
        {statuses.map((s) => (
          <SelectItem key={s} value={s}>
            {statusLabel(s).label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function SearchBox({
  initial,
  placeholder,
  onSearch,
}: {
  initial?: string
  placeholder: string
  onSearch: (q: string | undefined) => void
}) {
  const [q, setQ] = useState(initial ?? "")
  return (
    <form
      className="relative w-full sm:w-72"
      onSubmit={(e) => {
        e.preventDefault()
        onSearch(q || undefined)
      }}
    >
      <SearchIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
      />
      <Input
        aria-label={placeholder}
        placeholder={placeholder}
        className="pl-8"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
    </form>
  )
}

// --- Proforma Invoice ---------------------------------------------------------------------------

/** Penjualan › Proforma Invoice (S-302). */
export function ProformaListPage({
  search,
  onSearchChange,
}: {
  search: ProformaListSearch
  onSearchChange: (u: (s: ProformaListSearch) => ProformaListSearch) => void
}) {
  const list = useQuery(proformasQuery(search))
  const columns: DataTableColumn<ProformaInvoice>[] = [
    {
      id: "number",
      header: "Nomor PI",
      cell: (p) => (
        <Link
          to="/sales/proforma-invoices/$id"
          params={{ id: p.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {p.number}
        </Link>
      ),
    },
    {
      id: "order",
      header: "Sales Order",
      cell: (p) => (
        <div className="flex flex-col">
          <span>{p.salesOrderNumber}</span>
          <span className="text-xs text-muted-foreground">
            {ORDER_TYPE_LABELS[p.orderType]}
          </span>
        </div>
      ),
    },
    { id: "party", header: "Pelanggan / Mitra", cell: (p) => p.partyName },
    {
      id: "date",
      header: "Terbit",
      cell: (p) => formatDate(p.issueDate),
    },
    {
      id: "valid",
      header: "Berlaku s.d.",
      cell: (p) => formatDate(p.validUntil),
    },
    {
      id: "amount",
      header: "Nominal",
      align: "right",
      cell: (p) => formatRupiah(p.amount),
    },
    {
      id: "status",
      header: "Status",
      cell: (p) => <StatusBadge status={p.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Proforma Invoice
        </h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Permintaan pembayaran pesanan bayar di muka, berlaku 14 hari. Proforma
          yang tidak dibayar otomatis Kedaluwarsa dan alokasi stoknya dilepas.
          F&A mengonfirmasi pembayaran → invoice NON-PKP Lunas.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <SearchBox
          initial={search.q}
          placeholder="Cari nomor PI, SO, pelanggan"
          onSearch={(q) => onSearchChange((s) => ({ ...s, q, page: 1 }))}
        />
        <StatusFilter
          value={search.status}
          statuses={PROFORMA_STATUSES}
          onChange={(status) =>
            onSearchChange((s) => ({ ...s, status, page: 1 }))
          }
        />
      </div>
      <DataTable
        columns={columns}
        rows={list.data?.items}
        getRowId={(p) => p.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada Proforma Invoice."
      />
    </div>
  )
}

/** Detail Proforma Invoice dengan konfirmasi pembayaran oleh F&A. */
export function ProformaDetailPage({ id }: { id: string }) {
  const pi = useQuery(proformaQuery(id))
  const [paying, setPaying] = useState(false)
  return (
    <LoadingArea loading={pi.isPending}>
      {pi.data ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-2xl font-semibold tracking-tight">
                  Proforma Invoice {pi.data.number}
                </h1>
                <StatusBadge status={pi.data.status} />
              </div>
              <p className="text-sm text-muted-foreground">
                {pi.data.partyName} ·{" "}
                <Link
                  to="/sales/orders/$id"
                  params={{ id: pi.data.salesOrderId }}
                  search={{}}
                  className="underline-offset-4 hover:underline"
                >
                  {pi.data.salesOrderNumber}
                </Link>
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                render={
                  <Link
                    to="/sales/proforma-invoices/$id/print"
                    params={{ id: pi.data.id }}
                  />
                }
              >
                <PrinterIcon /> Cetak Proforma
              </Button>
              {pi.data.allowedActions.includes("confirm_payment") ? (
                <Button onClick={() => setPaying(true)}>
                  Konfirmasi Pembayaran
                </Button>
              ) : null}
              {pi.data.invoiceId ? (
                <Button
                  variant="outline"
                  render={
                    <Link
                      to="/sales/invoices/$id"
                      params={{ id: pi.data.invoiceId }}
                    />
                  }
                >
                  Buka Invoice
                </Button>
              ) : null}
            </div>
          </div>
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info label="Terbit" value={formatDate(pi.data.issueDate)} />
            <Info label="Berlaku s.d." value={formatDate(pi.data.validUntil)} />
            <Info label="Nominal" value={formatRupiah(pi.data.amount)} />
            <Info
              label="Dibayar"
              value={pi.data.paidAt ? formatDateTime(pi.data.paidAt) : "—"}
            />
            {pi.data.salesOrder.paymentProofFileName ? (
              <Info
                label="Bukti bayar dari CS/Sales"
                value={pi.data.salesOrder.paymentProofFileName}
              />
            ) : null}
            {pi.data.cancelReason ? (
              <Info label="Alasan dibatalkan" value={pi.data.cancelReason} />
            ) : null}
          </dl>
          <DocumentLines
            lines={pi.data.salesOrder.lines.map((l) => ({
              id: l.id,
              description: `${l.product.sku} – ${l.product.name}`,
              qty: l.qty,
              unitPrice: l.unitPrice,
              discountAmount: l.discountAmount,
              lineTotalAmount: l.lineTotalAmount,
            }))}
            discount={(
              Number(pi.data.salesOrder.lineDiscountAmount) +
              Number(pi.data.salesOrder.documentDiscountAmount)
            ).toFixed(2)}
            shipping={pi.data.salesOrder.shippingAmount}
            total={pi.data.amount}
          />
          <ConfirmPaymentDialog
            open={paying}
            onOpenChange={setPaying}
            proforma={pi.data}
          />
        </div>
      ) : null}
    </LoadingArea>
  )
}

type DocLine = {
  id: string
  description: string
  qty: string
  unitPrice: string
  discountAmount: string
  lineTotalAmount: string
}

function DocumentLines({
  lines,
  discount,
  shipping,
  total,
}: {
  lines: DocLine[]
  discount: string
  shipping: string
  total: string
}) {
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[44rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Uraian</TableHead>
            <TableHead className="text-right">Qty</TableHead>
            <TableHead className="text-right">Harga</TableHead>
            <TableHead className="text-right">Diskon</TableHead>
            <TableHead className="text-right">Jumlah</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {lines.map((l) => (
            <TableRow key={l.id}>
              <TableCell>{l.description}</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatQuantity(l.qty)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRupiah(l.unitPrice)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {Number(l.discountAmount) > 0
                  ? `−${formatRupiah(l.discountAmount)}`
                  : "—"}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRupiah(l.lineTotalAmount)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
        <TableFooter>
          {Number(discount) > 0 ? (
            <SumRow label="Total diskon" value={`−${formatRupiah(discount)}`} />
          ) : null}
          {Number(shipping) > 0 ? (
            <SumRow label="Ongkos kirim" value={formatRupiah(shipping)} />
          ) : null}
          <SumRow
            label="Total (tanpa PPN — NON-PKP)"
            value={formatRupiah(total)}
          />
        </TableFooter>
      </Table>
    </div>
  )
}

function SumRow({ label, value }: { label: string; value: string }) {
  return (
    <TableRow>
      <TableCell colSpan={4} className="font-medium">
        {label}
      </TableCell>
      <TableCell className="text-right tabular-nums">{value}</TableCell>
    </TableRow>
  )
}

function ConfirmPaymentDialog({
  open,
  onOpenChange,
  proforma,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  proforma: ProformaInvoiceDetail
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ConfirmPaymentForm
          proforma={proforma}
          onClose={() => onOpenChange(false)}
        />
      ) : null}
    </Dialog>
  )
}

function ConfirmPaymentForm({
  proforma,
  onClose,
}: {
  proforma: ProformaInvoiceDetail
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const accounts = useQuery(cashBankAccountsQuery)
  const [paymentDate, setPaymentDate] = useState(todayBusinessDate())
  const [method, setMethod] = useState<CustomerPaymentMethod>("bank_transfer")
  const [accountCode, setAccountCode] = useState("")
  const [reference, setReference] = useState("")
  const [proofFileId, setProofFileId] = useState<string | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      confirmProformaPayment(
        proforma.id,
        proforma.version,
        { paymentDate, method, accountCode, reference, proofFileId },
        key
      ),
    onSuccess: async () => {
      toast.success("Pembayaran dikonfirmasi, invoice Lunas terbit")
      onClose()
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
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
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Konfirmasi Pembayaran {proforma.number}</DialogTitle>
        <DialogDescription>
          Invoice NON-PKP terbit Lunas sebesar {formatRupiah(proforma.amount)}{" "}
          dan pembayaran dicatat ke akun Kas & Bank (jurnal A-05 dan A-PB).
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          if (!accountCode) {
            setErrors({ accountCode: "Pilih akun Kas & Bank" })
            return
          }
          save.mutate()
        }}
      >
        <Field data-invalid={!!errors.paymentDate}>
          <FieldLabel htmlFor="pay-date">Tanggal Bayar</FieldLabel>
          <DateInput
            id="pay-date"
            value={paymentDate}
            onValueChange={setPaymentDate}
          />
          <FieldError errors={fieldErrors(errors.paymentDate)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="pay-method">Cara Bayar</FieldLabel>
          <Select
            value={method}
            onValueChange={(v) => setMethod(v as CustomerPaymentMethod)}
          >
            <SelectTrigger id="pay-method" className="w-full">
              <SelectValue>
                {(v: string) =>
                  PAYMENT_METHOD_LABELS[v as CustomerPaymentMethod]
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {(["bank_transfer", "cash"] as const).map((m) => (
                <SelectItem key={m} value={m}>
                  {PAYMENT_METHOD_LABELS[m]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field data-invalid={!!errors.accountCode}>
          <FieldLabel htmlFor="pay-account">Akun Kas & Bank</FieldLabel>
          <OptionSelect
            id="pay-account"
            value={accountCode}
            onValueChange={setAccountCode}
            options={accounts.data ?? []}
            placeholder="Pilih akun"
            showCode
            aria-invalid={!!errors.accountCode}
          />
          <FieldError errors={fieldErrors(errors.accountCode)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="pay-ref">Referensi transfer</FieldLabel>
          <Input
            id="pay-ref"
            value={reference}
            maxLength={200}
            onChange={(e) => setReference(e.target.value)}
          />
        </Field>
        <FileUpload
          label="Lampirkan bukti (opsional)"
          accept="image/jpeg,image/png,image/webp,application/pdf"
          maxSizeBytes={MAX_PROOF_FILE_BYTES}
          requestUpload={(f) =>
            requestProofUpload(f, "sales.customer_payment_proof")
          }
          onUploaded={(id) => setProofFileId(id)}
          onRemoved={() => setProofFileId(null)}
          onError={(m) => toast.error(m)}
        />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Memproses…"
          >
            Konfirmasi Pembayaran
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

// --- Invoice ------------------------------------------------------------------------------------

export function invoiceColumns(): DataTableColumn<SalesInvoiceSummary>[] {
  return [
    {
      id: "number",
      header: "Nomor Invoice",
      sortKey: "number",
      cell: (i) => (
        <Link
          to="/sales/invoices/$id"
          params={{ id: i.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {i.number}
        </Link>
      ),
    },
    {
      id: "party",
      header: "Pelanggan / Mitra",
      cell: (i) => (
        <div className="flex flex-col">
          <span>{i.partyName}</span>
          <span className="text-xs text-muted-foreground">
            {[i.partyCode, i.salesOrderNumber].filter(Boolean).join(" · ")}
          </span>
        </div>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      sortKey: "invoiceDate",
      cell: (i) => formatDate(i.invoiceDate),
    },
    {
      id: "due",
      header: "Jatuh Tempo",
      sortKey: "dueDate",
      cell: (i) => (
        <div className="flex flex-col">
          <span className="whitespace-nowrap">{formatDate(i.dueDate)}</span>
          {i.daysOverdue > 0 ? (
            <span className="text-xs text-destructive">
              {overdueLabel(i.daysOverdue)}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "total",
      header: "Total",
      sortKey: "totalAmount",
      align: "right",
      cell: (i) => formatRupiah(i.totalAmount),
    },
    {
      id: "outstanding",
      header: "Sisa Piutang",
      align: "right",
      cell: (i) => formatRupiah(i.outstandingAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (i) => <StatusBadge status={i.status} />,
    },
  ]
}

/** Penjualan › Invoice (S-302). */
export function InvoiceListPage({
  search,
  onSearchChange,
}: {
  search: InvoiceListSearch
  onSearchChange: (u: (s: InvoiceListSearch) => InvoiceListSearch) => void
}) {
  const list = useQuery(invoicesQuery(search))
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Invoice</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Invoice NON-PKP (tanpa PPN). Penjualan MT, B2B, WA/IG, dan karyawan
          diakui saat invoice terbit (jurnal A-05).
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <SearchBox
          initial={search.q}
          placeholder="Cari nomor invoice, SO, pelanggan"
          onSearch={(q) => onSearchChange((s) => ({ ...s, q, page: 1 }))}
        />
        <StatusFilter
          value={search.status}
          statuses={INVOICE_STATUSES}
          onChange={(status) =>
            onSearchChange((s) => ({ ...s, status, page: 1 }))
          }
        />
      </div>
      <DataTable
        columns={invoiceColumns()}
        rows={list.data?.items}
        getRowId={(i) => i.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          onSearchChange((s) => ({ ...s, sort, page: 1 }))
        }
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada invoice."
      />
    </div>
  )
}

/** Detail invoice: baris, riwayat pembayaran, cetak, dan teks WhatsApp. */
export function InvoiceDetailPage({ id }: { id: string }) {
  const inv = useQuery(invoiceQuery(id))
  const [sharing, setSharing] = useState(false)
  return (
    <LoadingArea loading={inv.isPending}>
      {inv.data ? (
        <InvoiceDetail
          invoice={inv.data}
          sharing={sharing}
          onSharingChange={setSharing}
        />
      ) : null}
    </LoadingArea>
  )
}

function InvoiceDetail({
  invoice,
  sharing,
  onSharingChange,
}: {
  invoice: SalesInvoice
  sharing: boolean
  onSharingChange: (open: boolean) => void
}) {
  const can = (a: SalesInvoice["allowedActions"][number]) =>
    invoice.allowedActions.includes(a)
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              Invoice {invoice.number}
            </h1>
            <StatusBadge status={invoice.status} />
            {invoice.orderType ? (
              <Badge variant="secondary">
                {ORDER_TYPE_LABELS[invoice.orderType]}
              </Badge>
            ) : null}
          </div>
          <p className="text-sm text-muted-foreground">
            {invoice.partyName} · {invoice.channelName} ›{" "}
            {invoice.subChannelName}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("share_whatsapp") ? (
            <Button variant="outline" onClick={() => onSharingChange(true)}>
              Kirim ke WhatsApp
            </Button>
          ) : null}
          {can("print") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/sales/invoices/$id/print"
                  params={{ id: invoice.id }}
                />
              }
            >
              <PrinterIcon /> Cetak Invoice
            </Button>
          ) : null}
          {can("record_payment") ? (
            <Button
              render={
                <Link
                  to="/finance/receivables"
                  search={{
                    tab: "invoices",
                    page: 1,
                    pay: invoice.id,
                  }}
                />
              }
            >
              Catat Pembayaran
            </Button>
          ) : null}
        </div>
      </div>
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <Info label="Tanggal" value={formatDate(invoice.invoiceDate)} />
        <Info
          label="Jatuh Tempo"
          value={`${formatDate(invoice.dueDate)}${invoice.daysOverdue > 0 ? ` (${overdueLabel(invoice.daysOverdue)})` : ""}`}
        />
        <Info
          label="Sales Order"
          value={
            invoice.salesOrderId ? (
              <Link
                to="/sales/orders/$id"
                params={{ id: invoice.salesOrderId }}
                search={{}}
                className="text-primary underline-offset-4 hover:underline"
              >
                {invoice.salesOrderNumber}
              </Link>
            ) : (
              "—"
            )
          }
        />
        <Info label="Dibayar" value={formatRupiah(invoice.paidAmount)} />
        <Info
          label="Sisa Piutang"
          value={formatRupiah(invoice.outstandingAmount)}
        />
        {invoice.cancelReason ? (
          <Info label="Alasan dibatalkan" value={invoice.cancelReason} />
        ) : null}
      </dl>
      <DocumentLines
        lines={invoice.lines}
        discount={invoice.discountAmount}
        shipping={invoice.shippingAmount}
        total={invoice.totalAmount}
      />
      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">Pembayaran</h2>
        {invoice.payments.length === 0 ? (
          <p className="text-sm text-muted-foreground">Belum ada pembayaran.</p>
        ) : (
          <ul className="flex flex-col gap-1 text-sm">
            {invoice.payments.map((p) => (
              <li
                key={p.paymentId}
                className="flex flex-wrap items-center gap-2"
              >
                <span className="font-medium">{p.paymentNumber}</span>
                <span>{formatDate(p.paymentDate)}</span>
                <span className="tabular-nums">{formatRupiah(p.amount)}</span>
                <StatusBadge status={p.status} />
              </li>
            ))}
          </ul>
        )}
      </section>
      <ShareDialog
        open={sharing}
        onOpenChange={onSharingChange}
        invoiceId={invoice.id}
      />
    </div>
  )
}

/** Teks invoice siap tempel ke WhatsApp beserta tautan cetak (SLS-04). */
function ShareDialog({
  open,
  onOpenChange,
  invoiceId,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  invoiceId: string
}) {
  const text = useQuery({
    queryKey: ["sales", "invoice-share", invoiceId],
    queryFn: () => invoiceShareText(invoiceId),
    enabled: open,
  })
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Kirim Invoice ke WhatsApp</DialogTitle>
          <DialogDescription>
            Salin teks lalu tempel di chat WhatsApp pelanggan.
          </DialogDescription>
        </DialogHeader>
        <LoadingArea loading={text.isPending} label="Menyusun teks invoice">
          <Textarea
            aria-label="Teks invoice"
            readOnly
            rows={14}
            value={text.data?.text ?? ""}
          />
        </LoadingArea>
        <DialogFooter>
          <Button
            disabled={!text.data}
            onClick={() => {
              void navigator.clipboard
                .writeText(text.data?.text ?? "")
                .then(() => toast.success("Teks invoice disalin"))
            }}
          >
            <CopyIcon /> Salin Teks
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
