import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { BarChart3Icon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { usePermissions } from "@/features/auth/api/auth-api"
import {
  fieldErrors,
  LineError,
  OptionSelect,
  ReasonDialog,
} from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  agingQuery,
  cancelPayment,
  cashBankAccountsQuery,
  createPayment,
  invoiceQuery,
  invoicesQuery,
  MAX_PROOF_FILE_BYTES,
  openInvoicesQuery,
  PAYMENT_METHOD_LABELS,
  paymentsQuery,
  requestProofUpload,
  type CustomerPayment,
  type CustomerPaymentMethod,
  type SalesInvoiceSummary,
  type SalesPartyType,
} from "../api/sales-documents-api"
import { autoAllocate, sumAmounts } from "../lib/sales-lib"
import { invoiceColumns } from "./billing-pages"

export type ReceivableTab = "invoices" | "payments"

export type ReceivableSearch = {
  tab?: ReceivableTab
  page: number
  sort?: string
  q?: string
  overdue?: boolean
  /** Buka form pembayaran untuk invoice ini. */
  pay?: string
}

/**
 * Keuangan & Akuntansi › Piutang Usaha: piutang per invoice (sisa, jatuh tempo, umur), pembayaran sebagian, satu
 * pembayaran dialokasikan ke banyak invoice (S-315, ACC-03, ACC-15).
 */
export function ReceivablesPage({
  search,
  onSearchChange,
}: {
  search: ReceivableSearch
  onSearchChange: (u: (s: ReceivableSearch) => ReceivableSearch) => void
}) {
  const { can } = usePermissions()
  const tab = search.tab ?? "invoices"
  const [paying, setPaying] = useState<SalesInvoiceSummary | "new" | null>(null)
  const preset = useQuery({
    ...invoiceQuery(search.pay ?? ""),
    enabled: !!search.pay,
  })
  const presetInvoice = search.pay && preset.data ? preset.data : null
  const dialogInvoice =
    paying === "new" ? null : (paying ?? presetInvoice ?? null)
  const dialogOpen = paying !== null || !!presetInvoice

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Piutang Usaha
          </h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Piutang per invoice dengan pembayaran sebagian dan alokasi satu
            pembayaran ke banyak invoice. Jurnal pembayaran: Dr Kas & Bank, Cr
            Piutang Usaha (A-PB).
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            render={<Link to="/reports/receivable-aging" search={{}} />}
          >
            <BarChart3Icon /> Laporan Piutang
          </Button>
          {can("accounting.settlement.read") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/finance/marketplace-settlements"
                  search={{ page: 1 }}
                />
              }
            >
              Settlement Marketplace
            </Button>
          ) : null}
          {can("accounting.payment.post") ? (
            <Button onClick={() => setPaying("new")}>
              <PlusIcon /> Catat Pembayaran
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Bagian piutang"
        value={tab}
        onValueChange={(t) =>
          onSearchChange(() => ({ tab: t as ReceivableTab, page: 1 }))
        }
        tabs={[
          { id: "invoices", label: "Invoice Terbuka" },
          { id: "payments", label: "Pembayaran" },
        ]}
      />
      {tab === "invoices" ? (
        <OpenInvoices
          search={search}
          onSearchChange={onSearchChange}
          onPay={can("accounting.payment.post") ? setPaying : undefined}
        />
      ) : (
        <Payments search={search} onSearchChange={onSearchChange} />
      )}
      <Dialog
        open={dialogOpen}
        onOpenChange={(o) => {
          if (!o) {
            setPaying(null)
            if (search.pay) onSearchChange((s) => ({ ...s, pay: undefined }))
          }
        }}
      >
        {dialogOpen ? (
          <PaymentForm
            key={dialogInvoice?.id ?? "new"}
            invoice={dialogInvoice}
            onClose={() => {
              setPaying(null)
              if (search.pay) onSearchChange((s) => ({ ...s, pay: undefined }))
            }}
          />
        ) : null}
      </Dialog>
    </div>
  )
}

function OpenInvoices({
  search,
  onSearchChange,
  onPay,
}: {
  search: ReceivableSearch
  onSearchChange: (u: (s: ReceivableSearch) => ReceivableSearch) => void
  onPay?: (inv: SalesInvoiceSummary) => void
}) {
  const [q, setQ] = useState(search.q ?? "")
  const list = useQuery(
    invoicesQuery(
      {
        page: search.page,
        q: search.q,
        overdue: search.overdue,
        sort: search.sort,
      },
      true
    )
  )
  const columns: DataTableColumn<SalesInvoiceSummary>[] = [
    ...invoiceColumns(),
    ...(onPay
      ? [
          {
            id: "pay",
            header: "",
            cell: (i: SalesInvoiceSummary) => (
              <Button size="sm" variant="outline" onClick={() => onPay(i)}>
                Bayar
              </Button>
            ),
          },
        ]
      : []),
  ]
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange((s) => ({ ...s, q: q || undefined, page: 1 }))
          }}
        >
          <Input
            aria-label="Cari invoice"
            placeholder="Cari nomor invoice atau pelanggan"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <label className="flex min-h-11 items-center gap-2 text-sm sm:min-h-9">
          <input
            type="checkbox"
            className="size-4"
            checked={!!search.overdue}
            onChange={(e) =>
              onSearchChange((s) => ({
                ...s,
                overdue: e.target.checked || undefined,
                page: 1,
              }))
            }
          />
          Hanya lewat jatuh tempo
        </label>
      </div>
      <DataTable
        columns={columns}
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
        emptyMessage="Tidak ada piutang terbuka."
      />
    </div>
  )
}

function Payments({
  search,
  onSearchChange,
}: {
  search: ReceivableSearch
  onSearchChange: (u: (s: ReceivableSearch) => ReceivableSearch) => void
}) {
  const queryClient = useQueryClient()
  const list = useQuery(paymentsQuery({ page: search.page, q: search.q }))
  const [cancelling, setCancelling] = useState<CustomerPayment | null>(null)
  const cancel = useIdempotentMutation({
    mutationFn: (reason: string, key) =>
      cancelPayment(
        cancelling?.id ?? "",
        cancelling?.version ?? 0,
        reason,
        key
      ),
    onSuccess: async () => {
      setCancelling(null)
      toast.success("Pembayaran dibatalkan, jurnal dibalik")
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const columns: DataTableColumn<CustomerPayment>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (p) => <span className="font-medium">{p.number}</span>,
    },
    { id: "date", header: "Tanggal", cell: (p) => formatDate(p.paymentDate) },
    { id: "party", header: "Pelanggan / Mitra", cell: (p) => p.partyName },
    {
      id: "method",
      header: "Cara / Akun",
      cell: (p) => (
        <div className="flex flex-col">
          <span>{PAYMENT_METHOD_LABELS[p.method]}</span>
          <span className="text-xs text-muted-foreground">
            {p.accountCode}
            {p.reference ? ` · ${p.reference}` : ""}
          </span>
        </div>
      ),
    },
    {
      id: "allocations",
      header: "Dialokasikan ke",
      cell: (p) => (
        <ul className="text-xs">
          {p.allocations.map((a) => (
            <li key={a.invoiceId}>
              <Link
                to="/sales/invoices/$id"
                params={{ id: a.invoiceId }}
                className="text-primary underline-offset-4 hover:underline"
              >
                {a.invoiceNumber}
              </Link>{" "}
              {formatRupiah(a.amount)}
            </li>
          ))}
        </ul>
      ),
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
    {
      id: "actions",
      header: "",
      cell: (p) =>
        p.allowedActions.includes("cancel") ? (
          <Button size="sm" variant="outline" onClick={() => setCancelling(p)}>
            Batalkan
          </Button>
        ) : null,
    },
  ]
  return (
    <>
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
        emptyMessage="Belum ada pembayaran pelanggan."
      />
      <ReasonDialog
        open={cancelling !== null}
        onOpenChange={(o) => !o && setCancelling(null)}
        title={`Batalkan pembayaran ${cancelling?.number ?? ""}?`}
        description="Jurnal pembayaran dibalik dan sisa piutang invoice dikembalikan."
        confirmLabel="Batalkan Pembayaran"
        required
        destructive
        loading={cancel.isPending}
        onConfirm={(r) => cancel.mutate(r)}
      />
    </>
  )
}

type PartyRef = { type: SalesPartyType; id: string; name: string }

/** Form pembayaran: pilih pihak dari invoice, alokasikan ke satu atau banyak invoice terbuka. */
function PaymentForm({
  invoice,
  onClose,
}: {
  invoice: SalesInvoiceSummary | null
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const accounts = useQuery(cashBankAccountsQuery)
  const [party, setParty] = useState<PartyRef | null>(
    invoice
      ? {
          type: invoice.partyType,
          id: invoice.partyId,
          name: invoice.partyName,
        }
      : null
  )
  const [partyQ, setPartyQ] = useState("")
  const candidates = useQuery({
    ...invoicesQuery({ page: 1, q: partyQ || undefined }, true),
    enabled: !party,
  })
  const open = useQuery({
    ...openInvoicesQuery(party?.type ?? "customer", party?.id ?? ""),
    enabled: !!party,
  })
  const [amount, setAmount] = useState(invoice ? invoice.outstandingAmount : "")
  const [alloc, setAlloc] = useState<Record<string, string>>(
    invoice ? { [invoice.id]: invoice.outstandingAmount } : {}
  )
  const [paymentDate, setPaymentDate] = useState(todayBusinessDate())
  const [method, setMethod] = useState<CustomerPaymentMethod>("bank_transfer")
  const [accountCode, setAccountCode] = useState("")
  const [reference, setReference] = useState("")
  const [note, setNote] = useState("")
  const [proofFileId, setProofFileId] = useState<string | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const invoices = open.data ?? []
  const allocations = invoices
    .filter((i) => Number(alloc[i.id] ?? 0) > 0)
    .map((i) => ({ invoiceId: i.id, amount: alloc[i.id] }))
  const allocated = sumAmounts(allocations.map((a) => a.amount))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createPayment(
        {
          partyType: party?.type ?? "customer",
          partyId: party?.id ?? "",
          paymentDate,
          amount,
          method,
          accountCode,
          reference: reference || undefined,
          note: note || undefined,
          proofFileId,
          allocations,
        },
        key
      ),
    onSuccess: async (p) => {
      toast.success(`Pembayaran ${p.number} diposting`)
      onClose()
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        const mapped: Record<string, string> = {}
        for (const [k, v] of Object.entries(problem.fields)) {
          const m = /^allocations\.(\d+)\.(.+)$/.exec(k)
          mapped[m ? `alloc.${allocations[Number(m[1])]?.invoiceId}` : k] = v
        }
        setErrors(mapped)
        return
      }
      showErrorDialog(problem)
    },
  })

  return (
    <DialogContent className="sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle>Catat Pembayaran Pelanggan</DialogTitle>
        <DialogDescription>
          Pembayaran sebagian diperbolehkan. Jumlah alokasi harus sama dengan
          nominal pembayaran.
        </DialogDescription>
      </DialogHeader>
      {!party ? (
        <div className="flex flex-col gap-2">
          <Input
            aria-label="Cari invoice atau pelanggan"
            placeholder="Cari nama pelanggan/mitra atau nomor invoice"
            value={partyQ}
            onChange={(e) => setPartyQ(e.target.value)}
          />
          <LoadingArea loading={candidates.isPending}>
            <ul className="flex max-h-72 flex-col overflow-y-auto rounded-md border">
              {(candidates.data?.items ?? []).map((i) => (
                <li key={i.id}>
                  <button
                    type="button"
                    className="flex min-h-11 w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-accent"
                    onClick={() => {
                      setParty({
                        type: i.partyType,
                        id: i.partyId,
                        name: i.partyName,
                      })
                      setAlloc({})
                      setAmount("")
                    }}
                  >
                    <span>
                      {i.partyName}{" "}
                      <span className="text-muted-foreground">{i.number}</span>
                    </span>
                    <span className="tabular-nums">
                      {formatRupiah(i.outstandingAmount)}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </LoadingArea>
        </div>
      ) : (
        <form
          noValidate
          className="flex flex-col gap-3"
          onSubmit={(e) => {
            e.preventDefault()
            const next: Record<string, string> = {}
            if (!accountCode) next.accountCode = "Pilih akun Kas & Bank"
            if (!amount || Number(amount) <= 0)
              next.amount = "Nominal harus lebih dari nol"
            if (allocations.length === 0)
              next.allocations = "Alokasikan ke minimal satu invoice"
            else if (allocated !== Number(amount || 0).toFixed(2))
              next.amount = `Jumlah alokasi ${formatRupiah(allocated)} harus sama dengan nominal`
            setErrors(next)
            if (Object.keys(next).length === 0) save.mutate()
          }}
        >
          <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
            <span>
              Pihak: <span className="font-medium">{party.name}</span>
            </span>
            {!invoice ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setParty(null)}
              >
                Ganti
              </Button>
            ) : null}
          </div>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field data-invalid={!!errors.paymentDate}>
              <FieldLabel htmlFor="cp-date">Tanggal Bayar</FieldLabel>
              <DateInput
                id="cp-date"
                value={paymentDate}
                onValueChange={setPaymentDate}
              />
              <FieldError errors={fieldErrors(errors.paymentDate)} />
            </Field>
            <Field data-invalid={!!errors.amount}>
              <FieldLabel htmlFor="cp-amount">Nominal Diterima</FieldLabel>
              <MoneyInput
                id="cp-amount"
                value={amount}
                onValueChange={setAmount}
              />
              <FieldError errors={fieldErrors(errors.amount)} />
            </Field>
            <Field>
              <FieldLabel htmlFor="cp-method">Cara Bayar</FieldLabel>
              <Select
                value={method}
                onValueChange={(v) => setMethod(v as CustomerPaymentMethod)}
              >
                <SelectTrigger id="cp-method" className="w-full">
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
              <FieldLabel htmlFor="cp-account">Akun Kas & Bank</FieldLabel>
              <OptionSelect
                id="cp-account"
                value={accountCode}
                onValueChange={setAccountCode}
                options={accounts.data ?? []}
                placeholder="Pilih akun"
                showCode
                aria-invalid={!!errors.accountCode}
              />
              <FieldError errors={fieldErrors(errors.accountCode)} />
            </Field>
            <Field className="sm:col-span-2">
              <FieldLabel htmlFor="cp-ref">Referensi</FieldLabel>
              <Input
                id="cp-ref"
                value={reference}
                maxLength={200}
                onChange={(e) => setReference(e.target.value)}
              />
            </Field>
          </div>
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold">Alokasi ke Invoice</h3>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setAlloc(autoAllocate(amount, invoices))}
            >
              Alokasikan otomatis
            </Button>
          </div>
          <LoadingArea loading={open.isPending}>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[36rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead>Invoice</TableHead>
                    <TableHead>Jatuh Tempo</TableHead>
                    <TableHead className="text-right">Sisa</TableHead>
                    <TableHead className="w-44 text-right">Dibayar</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {invoices.map((i) => (
                    <TableRow key={i.id}>
                      <TableCell>{i.number}</TableCell>
                      <TableCell>{formatDate(i.dueDate)}</TableCell>
                      <TableCell className="text-right tabular-nums">
                        {formatRupiah(i.outstandingAmount)}
                      </TableCell>
                      <TableCell>
                        <MoneyInput
                          aria-label={`Alokasi ${i.number}`}
                          value={alloc[i.id] ?? ""}
                          onValueChange={(v) =>
                            setAlloc((a) => ({ ...a, [i.id]: v }))
                          }
                        />
                        <LineError message={errors[`alloc.${i.id}`]} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
                <TableFooter>
                  <TableRow>
                    <TableCell colSpan={3} className="font-medium">
                      Total dialokasikan
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(allocated)}
                    </TableCell>
                  </TableRow>
                </TableFooter>
              </Table>
            </div>
          </LoadingArea>
          <LineError message={errors.allocations} />
          <Field>
            <FieldLabel htmlFor="cp-note">Catatan</FieldLabel>
            <Textarea
              id="cp-note"
              value={note}
              maxLength={1000}
              onChange={(e) => setNote(e.target.value)}
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
              loadingText="Memposting…"
            >
              Posting Pembayaran
            </LoadingButton>
          </DialogFooter>
        </form>
      )}
    </DialogContent>
  )
}

// --- Laporan Piutang (umur piutang) -------------------------------------------------------------

export type AgingSearch = { asOf?: string; partyType?: SalesPartyType }

/** Laporan › Laporan Piutang: umur piutang per pelanggan/mitra (ACC-03, RPT-11). */
export function ReceivableAgingPage({
  search,
  onSearchChange,
}: {
  search: AgingSearch
  onSearchChange: (u: (s: AgingSearch) => AgingSearch) => void
}) {
  const asOf = search.asOf ?? todayBusinessDate()
  const report = useQuery(agingQuery(asOf, search.partyType))
  const cols = [
    ["current", "Belum Jatuh Tempo"],
    ["days1To30", "1–30 hari"],
    ["days31To60", "31–60 hari"],
    ["days61To90", "61–90 hari"],
    ["over90", "> 90 hari"],
    ["total", "Total"],
  ] as const
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Laporan Piutang
        </h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Umur piutang invoice terbuka per pelanggan/mitra berdasarkan hari
          lewat jatuh tempo pada tanggal posisi.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <div className="w-44">
          <DateInput
            value={asOf}
            onValueChange={(v) =>
              onSearchChange((s) => ({ ...s, asOf: v || undefined }))
            }
          />
        </div>
        <Select
          value={search.partyType ?? "all"}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              partyType: v === "all" || !v ? undefined : (v as SalesPartyType),
            }))
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter pihak">
            <SelectValue>
              {(v: string) =>
                v === "customer"
                  ? "Pelanggan"
                  : v === "partner"
                    ? "Mitra"
                    : "Semua pihak"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua pihak</SelectItem>
            <SelectItem value="customer">Pelanggan</SelectItem>
            <SelectItem value="partner">Mitra</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <LoadingArea loading={report.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[56rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Pelanggan / Mitra</TableHead>
                <TableHead className="text-right">Invoice</TableHead>
                {cols.map(([k, label]) => (
                  <TableHead key={k} className="text-right">
                    {label}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {(report.data?.rows ?? []).length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-muted-foreground">
                    Tidak ada piutang terbuka.
                  </TableCell>
                </TableRow>
              ) : null}
              {(report.data?.rows ?? []).map((r) => (
                <TableRow key={`${r.partyType}:${r.partyId}`}>
                  <TableCell>{r.partyName}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {r.invoices}
                  </TableCell>
                  {cols.map(([k]) => (
                    <TableCell key={k} className="text-right tabular-nums">
                      {formatRupiah(r[k])}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
            {report.data ? (
              <TableFooter>
                <TableRow>
                  <TableCell className="font-medium">Total</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {report.data.totals.invoices}
                  </TableCell>
                  {cols.map(([k]) => (
                    <TableCell key={k} className="text-right tabular-nums">
                      {formatRupiah(report.data.totals[k])}
                    </TableCell>
                  ))}
                </TableRow>
              </TableFooter>
            ) : null}
          </Table>
        </div>
      </LoadingArea>
    </div>
  )
}
