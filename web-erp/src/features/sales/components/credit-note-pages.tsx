import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { PageTabs } from "@/components/common/page-tabs"
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
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApprovalSteps } from "@/features/approval/components/approval-steps"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { subjectApprovalsQuery } from "@/features/procurement/api/procurement-api"
import type { DetailTab } from "@/features/procurement/components/request-detail-page"
import {
  fieldErrors,
  Info,
  OptionSelect,
} from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { cashBankAccountsQuery } from "../api/sales-documents-api"
import {
  applyCreditNote,
  CREDIT_NOTE_STATUSES,
  creditNoteQuery,
  creditNotesQuery,
  payCreditNoteRefund,
  SETTLEMENT_LABELS,
  type CreditNote,
  type CreditNoteListSearch,
  type CreditNoteSummary,
} from "../api/sales-returns-api"

const columns: DataTableColumn<CreditNoteSummary>[] = [
  {
    id: "number",
    header: "Nomor CN",
    cell: (c) => (
      <Link
        to="/sales/credit-notes/$id"
        params={{ id: c.id }}
        search={{}}
        className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
      >
        {c.number}
      </Link>
    ),
  },
  {
    id: "date",
    header: "Tanggal",
    cell: (c) => (
      <span className="whitespace-nowrap">{formatDate(c.issueDate)}</span>
    ),
  },
  {
    id: "party",
    header: "Pelanggan / Mitra",
    cell: (c) => (
      <div className="flex flex-col">
        <span>{c.partyName}</span>
        <span className="text-xs text-muted-foreground">
          {c.salesReturnNumber}
          {c.invoiceNumber ? ` · ${c.invoiceNumber}` : ""}
        </span>
      </div>
    ),
  },
  {
    id: "settlement",
    header: "Jenis",
    cell: (c) => (
      <Badge variant="outline">{SETTLEMENT_LABELS[c.settlementMethod]}</Badge>
    ),
  },
  {
    id: "amount",
    header: "Nilai",
    align: "right",
    cell: (c) => formatRupiah(c.amount),
  },
  {
    id: "remaining",
    header: "Saldo",
    align: "right",
    cell: (c) => formatRupiah(c.remainingAmount),
  },
  {
    id: "status",
    header: "Status",
    cell: (c) => <StatusBadge status={c.status} />,
  },
]

/** Penjualan › Nota Kredit: saldo kredit pelanggan/mitra dari retur penjualan dan refund. */
export function CreditNoteListPage({
  search,
  onSearchChange,
}: {
  search: CreditNoteListSearch
  onSearchChange: (u: (s: CreditNoteListSearch) => CreditNoteListSearch) => void
}) {
  const list = useQuery(creditNotesQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Nota Kredit</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Nota kredit terbit saat retur penjualan diposting (jurnal A-13
          mengurangi piutang). Saldo dialokasikan ke invoice asal lalu invoice
          berikutnya; refund disetujui Head Finance & Accounting lalu dibayar
          F&A.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange((s) => ({ ...s, q: q || undefined, page: 1 }))
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari nota kredit"
            placeholder="Cari nomor CN, RJ, pelanggan/mitra"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? "all"}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              status:
                v && v !== "all"
                  ? (v as CreditNoteListSearch["status"])
                  : undefined,
              page: 1,
            }))
          }
        >
          <SelectTrigger className="w-full sm:w-56" aria-label="Status">
            <SelectValue>
              {(v: string) =>
                v === "all" ? "Semua status" : statusLabel(v).label
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua status</SelectItem>
            {CREDIT_NOTE_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {statusLabel(s).label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={list.data?.items}
        getRowId={(c) => c.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada nota kredit."
      />
    </div>
  )
}

const CN_FIELD_LABELS: Record<string, string> = {
  number: "Nomor",
  settlementMethod: "Jenis",
  status: "Status",
  amount: "Nilai",
  appliedAmount: "Dialokasikan",
  refundedAmount: "Direfund",
  refundAccountCode: "Akun refund",
  refundReference: "Referensi refund",
}

/** Detail Nota Kredit: alokasi ke invoice, persetujuan & pembayaran refund, Riwayat. */
export function CreditNoteDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const cn = useQuery(creditNoteQuery(id))
  const [dialog, setDialog] = useState<"apply" | "refund" | null>(null)
  return (
    <LoadingArea loading={cn.isPending}>
      {cn.data ? (
        <div className="flex flex-col gap-4">
          <CreditNoteHeader cn={cn.data} onAction={setDialog} />
          <PageTabs
            label="Bagian nota kredit"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "summary", label: "Ringkasan" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "summary" ? (
            <CreditNoteSummaryView cn={cn.data} />
          ) : (
            <EntityHistory
              entityType="credit_note"
              entityId={cn.data.id}
              fieldLabels={CN_FIELD_LABELS}
            />
          )}
          <Dialog
            open={dialog === "apply"}
            onOpenChange={(o) => setDialog(o ? "apply" : null)}
          >
            {dialog === "apply" ? (
              <ApplyForm cn={cn.data} onClose={() => setDialog(null)} />
            ) : null}
          </Dialog>
          <Dialog
            open={dialog === "refund"}
            onOpenChange={(o) => setDialog(o ? "refund" : null)}
          >
            {dialog === "refund" ? (
              <RefundForm cn={cn.data} onClose={() => setDialog(null)} />
            ) : null}
          </Dialog>
        </div>
      ) : null}
    </LoadingArea>
  )
}

function CreditNoteHeader({
  cn,
  onAction,
}: {
  cn: CreditNote
  onAction: (a: "apply" | "refund") => void
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">
            Nota Kredit {cn.number}
          </h1>
          <StatusBadge status={cn.status} />
          <Badge variant="outline">
            {SETTLEMENT_LABELS[cn.settlementMethod]}
          </Badge>
        </div>
        <p className="text-sm text-muted-foreground">{cn.partyName}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        {cn.allowedActions.includes("apply") ? (
          <Button onClick={() => onAction("apply")}>
            Alokasikan ke Invoice
          </Button>
        ) : null}
        {cn.allowedActions.includes("pay_refund") ? (
          <Button onClick={() => onAction("refund")}>Bayar Refund</Button>
        ) : null}
      </div>
    </div>
  )
}

function CreditNoteSummaryView({ cn }: { cn: CreditNote }) {
  const refund = cn.settlementMethod === "refund" || cn.decisionNote !== ""
  return (
    <>
      <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
        <Info
          label="Retur Penjualan"
          value={
            <Link
              to="/sales/returns/$id"
              params={{ id: cn.salesReturnId }}
              search={{}}
              className="text-primary underline-offset-4 hover:underline"
            >
              {cn.salesReturnNumber}
            </Link>
          }
        />
        <Info
          label="Invoice Asal"
          value={
            cn.invoiceId ? (
              <Link
                to="/sales/invoices/$id"
                params={{ id: cn.invoiceId }}
                className="text-primary underline-offset-4 hover:underline"
              >
                {cn.invoiceNumber}
              </Link>
            ) : (
              "—"
            )
          }
        />
        <Info label="Tanggal" value={formatDate(cn.issueDate)} />
        <Info label="Nilai" value={formatRupiah(cn.amount)} />
        <Info label="Dialokasikan" value={formatRupiah(cn.appliedAmount)} />
        <Info label="Direfund" value={formatRupiah(cn.refundedAmount)} />
        <Info label="Saldo" value={formatRupiah(cn.remainingAmount)} />
        {cn.refundDate ? (
          <Info
            label="Refund Dibayar"
            value={`${formatDate(cn.refundDate)} · ${cn.refundAccountCode}${cn.refundReference ? ` · ${cn.refundReference}` : ""}`}
          />
        ) : null}
        {cn.refundJournalEntryId ? (
          <Info
            label="Jurnal Refund"
            value={
              <Link
                to="/finance/journals/$id"
                params={{ id: cn.refundJournalEntryId }}
                className="text-primary underline-offset-4 hover:underline"
              >
                Lihat jurnal
              </Link>
            }
          />
        ) : null}
        {cn.decisionNote ? (
          <Info label="Catatan Keputusan" value={cn.decisionNote} />
        ) : null}
      </dl>
      <section className="flex flex-col gap-2">
        <h2 className="text-sm font-semibold">Alokasi ke Invoice</h2>
        {cn.applications.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum dialokasikan ke invoice.
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[32rem]">
              <TableHeader>
                <TableRow>
                  <TableHead>Invoice</TableHead>
                  <TableHead>Tanggal</TableHead>
                  <TableHead className="text-right">Nominal</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {cn.applications.map((a) => (
                  <TableRow key={a.id}>
                    <TableCell>
                      <Link
                        to="/sales/invoices/$id"
                        params={{ id: a.invoiceId }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {a.invoiceNumber}
                      </Link>
                    </TableCell>
                    <TableCell>{formatDate(a.appliedDate)}</TableCell>
                    <TableCell className="text-right">
                      {formatRupiah(a.amount)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </section>
      {refund ? <RefundApprovalPanel creditNoteId={cn.id} /> : null}
    </>
  )
}

/** Panel persetujuan refund nota kredit (`sales_refund`, Head Finance & Accounting). */
function RefundApprovalPanel({ creditNoteId }: { creditNoteId: string }) {
  const approvals = useQuery(
    subjectApprovalsQuery("sales_refund", creditNoteId)
  )
  const requests = approvals.data?.items ?? []
  return (
    <section className="flex flex-col gap-3 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">Persetujuan Refund</h2>
      <LoadingArea loading={approvals.isPending} label="Memuat persetujuan">
        {requests.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum ada pengajuan persetujuan.
          </p>
        ) : (
          <ol className="flex flex-col gap-4">
            {requests.map((r, i) => (
              <li key={r.id} className="flex flex-col gap-2">
                {requests.length > 1 ? (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span>Pengajuan {requests.length - i}</span>
                    <StatusBadge status={r.status} />
                  </div>
                ) : null}
                <ApprovalSteps steps={r.steps} />
              </li>
            ))}
          </ol>
        )}
      </LoadingArea>
    </section>
  )
}

function useInvalidate() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: ["sales"] })
}

function ApplyForm({ cn, onClose }: { cn: CreditNote; onClose: () => void }) {
  const invalidate = useInvalidate()
  const [amounts, setAmounts] = useState<Record<string, string>>({})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const chosen = cn.openInvoices.filter(
    (i) => amounts[i.id] && Number(amounts[i.id]) > 0
  )
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      applyCreditNote(
        cn.id,
        cn.version,
        chosen.map((i) => ({ invoiceId: i.id, amount: amounts[i.id] })),
        key
      ),
    onSuccess: async () => {
      toast.success("Nota kredit dialokasikan")
      onClose()
      await invalidate()
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        const mapped: Record<string, string> = {}
        for (const [k, v] of Object.entries(problem.fields)) {
          const m = /^allocations\.(\d+)\./.exec(k)
          mapped[m && chosen[Number(m[1])] ? chosen[Number(m[1])].id : k] = v
        }
        setErrors(mapped)
        return
      }
      showErrorDialog(problem)
    },
  })
  return (
    <DialogContent className="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Alokasikan Nota Kredit</DialogTitle>
        <DialogDescription>
          Saldo {formatRupiah(cn.remainingAmount)} mengurangi sisa piutang
          invoice terbuka {cn.partyName}.
        </DialogDescription>
      </DialogHeader>
      {cn.openInvoices.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Tidak ada invoice terbuka untuk pihak ini.
        </p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[36rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Invoice</TableHead>
                <TableHead>Jatuh Tempo</TableHead>
                <TableHead className="text-right">Sisa Piutang</TableHead>
                <TableHead className="w-44 text-right">Alokasi</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {cn.openInvoices.map((i) => (
                <TableRow key={i.id} className="align-top">
                  <TableCell>{i.number}</TableCell>
                  <TableCell>{formatDate(i.dueDate)}</TableCell>
                  <TableCell className="text-right">
                    {formatRupiah(i.outstandingAmount)}
                  </TableCell>
                  <TableCell>
                    <MoneyInput
                      aria-label={`Alokasi ${i.number}`}
                      value={amounts[i.id] ?? ""}
                      onValueChange={(v) =>
                        setAmounts((a) => ({ ...a, [i.id]: v }))
                      }
                    />
                    {errors[i.id] ? (
                      <p className="mt-1 text-xs text-destructive">
                        {errors[i.id]}
                      </p>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {errors.allocations ? (
        <p className="text-sm text-destructive">{errors.allocations}</p>
      ) : null}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton
          loading={save.isPending}
          loadingText="Menyimpan…"
          disabled={chosen.length === 0}
          onClick={() => save.mutate()}
        >
          Alokasikan
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

function RefundForm({ cn, onClose }: { cn: CreditNote; onClose: () => void }) {
  const invalidate = useInvalidate()
  const accounts = useQuery(cashBankAccountsQuery)
  const [paymentDate, setPaymentDate] = useState(todayBusinessDate())
  const [accountCode, setAccountCode] = useState("")
  const [reference, setReference] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      payCreditNoteRefund(
        cn.id,
        {
          version: cn.version,
          paymentDate,
          accountCode,
          reference: reference || undefined,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Refund dibayar")
      onClose()
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
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Bayar Refund</DialogTitle>
        <DialogDescription>
          Refund {formatRupiah(cn.remainingAmount)} ke {cn.partyName}. Jurnal:
          Dr Piutang Usaha, Cr Kas/Bank. Butuh konfirmasi identitas.
        </DialogDescription>
      </DialogHeader>
      <Field data-invalid={!!errors.paymentDate}>
        <FieldLabel htmlFor="cn-date">Tanggal Bayar</FieldLabel>
        <DateInput
          id="cn-date"
          value={paymentDate}
          onValueChange={setPaymentDate}
        />
        <FieldError errors={fieldErrors(errors.paymentDate)} />
      </Field>
      <Field data-invalid={!!errors.accountCode}>
        <FieldLabel htmlFor="cn-account">Akun Kas & Bank</FieldLabel>
        <OptionSelect
          id="cn-account"
          value={accountCode}
          onValueChange={setAccountCode}
          options={(accounts.data ?? []).map((a) => ({
            id: a.code,
            code: a.code,
            name: a.name,
          }))}
          showCode
          placeholder="Pilih akun"
          aria-label="Akun Kas & Bank"
        />
        <FieldError errors={fieldErrors(errors.accountCode)} />
      </Field>
      <Field>
        <FieldLabel htmlFor="cn-ref">Referensi</FieldLabel>
        <Input
          id="cn-ref"
          maxLength={200}
          value={reference}
          onChange={(e) => setReference(e.target.value)}
        />
      </Field>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton
          loading={save.isPending}
          loadingText="Menyimpan…"
          disabled={!accountCode || !paymentDate}
          onClick={() => save.mutate()}
        >
          Bayar Refund
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
