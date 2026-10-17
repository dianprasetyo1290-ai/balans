import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import {
  CheckCircle2Icon,
  MinusCircleIcon,
  PlusIcon,
  XCircleIcon,
} from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
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
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
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
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { accountsQuery } from "../api/accounting-api"
import {
  cutoverReconciliationQuery,
  deleteOpeningLine,
  OPENING_ITEM_TYPE_LABELS,
  openingBalanceQuery,
  openingItemsQuery,
  postOpeningBalance,
  saveCutoverDate,
  saveOpeningLine,
  type OpeningBalance,
  type OpeningItem,
  type OpeningItemType,
} from "../api/close-api"
import { AccountSelect } from "./account-select"

const route = getRouteApi("/_app/finance/opening-balances")

const TX = "dataexchange.transaction"

/**
 * Keuangan & Akuntansi › Saldo Awal (ACC-14, A-36, S-412): tanggal cut-over, Neraca Saldo Awal, piutang/utang
 * terbuka, impor template migrasi, dan pengecekan rekonsiliasi cut-over sebelum diposting.
 */
export function OpeningBalancePage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Saldo Awal</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Saldo per tanggal cut-over (akhir periode akuntansi) dari Template
          Migrasi Saldo Awal. Neraca Saldo Awal diposting sebagai satu jurnal
          A-36; selisih debit dan kredit dibukukan ke 3901 Saldo Awal Ekuitas.
        </p>
      </div>
      <div className="flex flex-wrap gap-2 text-sm">
        <ImportGroup
          label="Neraca Saldo Awal"
          kind="accounting.opening_balance"
        />
        <ImportGroup
          label="Piutang Terbuka"
          kind="accounting.opening_receivable"
        />
        <ImportGroup label="Utang Terbuka" kind="accounting.opening_payable" />
        <ImportGroup label="Saldo Stok" kind="inventory.opening_stock" />
        <ImportGroup label="Aset Tetap" kind="accounting.opening_fixed_asset" />
      </div>
      <PageTabs
        label="Saldo awal"
        tabs={[
          { id: "balances", label: "Neraca Saldo Awal" },
          { id: "items", label: "Piutang & Utang Terbuka" },
          { id: "reconciliation", label: "Rekonsiliasi Cut-over" },
        ]}
        value={search.tab}
        onValueChange={(tab) =>
          void navigate({ search: (s) => ({ ...s, tab, page: 1 }) })
        }
      />
      {search.tab === "balances" ? <Balances /> : null}
      {search.tab === "items" ? <Items /> : null}
      {search.tab === "reconciliation" ? <Reconciliation /> : null}
    </div>
  )
}

function ImportGroup({ label, kind }: { label: string; kind: string }) {
  return (
    <div className="flex items-center gap-1 rounded-lg border px-2 py-1">
      <span className="text-muted-foreground">{label}</span>
      <DataExchangeButtons kind={kind} resource={TX} />
    </div>
  )
}

function Balances() {
  const queryClient = useQueryClient()
  const opening = useQuery(openingBalanceQuery)
  const [adding, setAdding] = useState(false)
  const [confirmPost, setConfirmPost] = useState(false)
  const set = (data: OpeningBalance) => {
    queryClient.setQueryData(openingBalanceQuery.queryKey, data)
    return queryClient.invalidateQueries({
      queryKey: ["accounting", "cutover-reconciliation"],
    })
  }
  const remove = useIdempotentMutation({
    mutationFn: (id: string, key) => deleteOpeningLine(id, key),
    onSuccess: set,
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const post = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      postOpeningBalance(opening.data?.version ?? 0, key),
    onSuccess: async (data) => {
      setConfirmPost(false)
      toast.success(`Saldo awal diposting: ${data.journalNumber ?? ""}`)
      await set(data)
    },
    onError: (err) => {
      setConfirmPost(false)
      showErrorDialog(parseProblem(err))
    },
  })
  const data = opening.data
  const canEdit = data?.allowedActions.includes("edit") ?? false
  const diff = Number(data?.difference ?? 0)

  return (
    <LoadingArea loading={opening.isPending}>
      {data ? (
        <div className="flex flex-col gap-4">
          <CutoverForm data={data} canEdit={canEdit} onSaved={set} />
          {data.status ? (
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <StatusBadge status={data.status} />
              {data.journalEntryId ? (
                <Link
                  to="/finance/journals/$id"
                  params={{ id: data.journalEntryId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  {data.journalNumber}
                </Link>
              ) : null}
              {data.postedAt ? (
                <span className="text-muted-foreground">
                  {formatDateTime(data.postedAt)}
                </span>
              ) : null}
            </div>
          ) : null}
          {data.id ? (
            <>
              <div className="flex flex-wrap gap-2">
                {canEdit ? (
                  <Button variant="outline" onClick={() => setAdding(true)}>
                    <PlusIcon /> Tambah Saldo Akun
                  </Button>
                ) : null}
                {data.allowedActions.includes("post") ? (
                  <Button onClick={() => setConfirmPost(true)}>
                    Posting Saldo Awal
                  </Button>
                ) : null}
              </div>
              <div className="overflow-x-auto rounded-lg border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Akun</TableHead>
                      <TableHead className="text-right">Debit</TableHead>
                      <TableHead className="text-right">Kredit</TableHead>
                      <TableHead>Keterangan</TableHead>
                      <TableHead />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {data.lines.map((l) => (
                      <TableRow key={l.id}>
                        <TableCell>
                          <span className="font-medium tabular-nums">
                            {l.accountCode}
                          </span>{" "}
                          {l.accountName}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {Number(l.debit) ? formatRupiah(l.debit) : ""}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {Number(l.credit) ? formatRupiah(l.credit) : ""}
                        </TableCell>
                        <TableCell className="text-sm">{l.note}</TableCell>
                        <TableCell className="text-right">
                          {canEdit ? (
                            <Button
                              variant="ghost"
                              className="min-h-11 sm:min-h-8"
                              disabled={remove.isPending}
                              onClick={() => remove.mutate(l.id)}
                            >
                              Hapus
                            </Button>
                          ) : null}
                        </TableCell>
                      </TableRow>
                    ))}
                    {data.lines.length === 0 ? (
                      <TableRow>
                        <TableCell
                          colSpan={5}
                          className="text-center text-muted-foreground"
                        >
                          Belum ada saldo akun. Impor sheet Neraca Saldo Awal
                          atau tambah per akun.
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                  <TableFooter>
                    <TableRow>
                      <TableCell className="font-semibold">Total</TableCell>
                      <TableCell className="text-right font-semibold tabular-nums">
                        {formatRupiah(data.totalDebit)}
                      </TableCell>
                      <TableCell className="text-right font-semibold tabular-nums">
                        {formatRupiah(data.totalCredit)}
                      </TableCell>
                      <TableCell colSpan={2} className="text-sm">
                        {diff === 0
                          ? "Balance"
                          : `Selisih ${formatRupiah(String(Math.abs(diff)))} akan dibukukan ke 3901 (${diff > 0 ? "kredit" : "debit"})`}
                      </TableCell>
                    </TableRow>
                  </TableFooter>
                </Table>
              </div>
            </>
          ) : null}
          <LineDialog open={adding} onOpenChange={setAdding} onSaved={set} />
          <ConfirmDialog
            open={confirmPost}
            onOpenChange={setConfirmPost}
            title="Posting saldo awal?"
            description={`Jurnal A-36 dibuat per ${formatDate(data.cutoverDate ?? "")}. Setelah diposting, saldo awal terkunci; koreksi lewat Jurnal Umum. Periksa tab Rekonsiliasi Cut-over lebih dulu.`}
            confirmLabel="Posting Saldo Awal"
            loading={post.isPending}
            onConfirm={() => post.mutate()}
          />
        </div>
      ) : null}
    </LoadingArea>
  )
}

function CutoverForm({
  data,
  canEdit,
  onSaved,
}: {
  data: OpeningBalance
  canEdit: boolean
  onSaved: (data: OpeningBalance) => Promise<unknown>
}) {
  const [date, setDate] = useState(data.cutoverDate ?? "")
  const [error, setError] = useState("")
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => saveCutoverDate(date, data.version ?? 0, key),
    onSuccess: async (d) => {
      setError("")
      toast.success("Tanggal cut-over disimpan")
      await onSaved(d)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields?.cutoverDate) {
        setError(problem.fields.cutoverDate)
        return
      }
      showErrorDialog(problem)
    },
  })
  return (
    <div className="flex flex-wrap items-end gap-3">
      <Field className="w-full sm:w-56" data-invalid={!!error}>
        <FieldLabel htmlFor="cutover-date">Tanggal Cut-over</FieldLabel>
        <DateInput
          id="cutover-date"
          value={date}
          disabled={!canEdit}
          onValueChange={setDate}
        />
        <FieldError errors={error ? [{ message: error }] : []} />
      </Field>
      {canEdit ? (
        <LoadingButton
          variant="outline"
          loading={save.isPending}
          loadingText="Menyimpan…"
          disabled={!date || date === data.cutoverDate}
          onClick={() => save.mutate()}
        >
          Simpan Tanggal
        </LoadingButton>
      ) : null}
    </div>
  )
}

type LineDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: (data: OpeningBalance) => Promise<unknown>
}

function LineDialog(props: LineDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <LineForm {...props} /> : null}
    </Dialog>
  )
}

function LineForm({ onOpenChange, onSaved }: LineDialogProps) {
  const accounts = useQuery(accountsQuery(false))
  const balanceSheet = (accounts.data ?? []).filter(
    (a) => a.isPostable && a.group <= 3
  )
  const [accountId, setAccountId] = useState("")
  const [debit, setDebit] = useState("")
  const [credit, setCredit] = useState("")
  const [note, setNote] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      saveOpeningLine(
        { accountId, debit: debit || "0", credit: credit || "0", note },
        key
      ),
    onSuccess: async (d) => {
      toast.success("Saldo akun disimpan")
      await onSaved(d)
      onOpenChange(false)
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
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Tambah Saldo Akun</DialogTitle>
        <DialogDescription>
          Hanya akun neraca (1xxx–3xxx). Akun yang sudah ada di daftar diganti.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.accountId}>
            <FieldLabel htmlFor="opening-account">Akun</FieldLabel>
            <AccountSelect
              id="opening-account"
              value={accountId}
              onValueChange={setAccountId}
              accounts={balanceSheet}
            />
            <FieldError errors={err("accountId")} />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.debit}>
              <FieldLabel htmlFor="opening-debit">Debit</FieldLabel>
              <MoneyInput
                id="opening-debit"
                value={debit}
                onValueChange={setDebit}
              />
              <FieldError errors={err("debit")} />
            </Field>
            <Field data-invalid={!!errors.credit}>
              <FieldLabel htmlFor="opening-credit">Kredit</FieldLabel>
              <MoneyInput
                id="opening-credit"
                value={credit}
                onValueChange={setCredit}
              />
              <FieldError errors={err("credit")} />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="opening-note">Keterangan</FieldLabel>
            <Input
              id="opening-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </Field>
        </FieldGroup>
        <DialogFooter className="mt-6">
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

const ALL = "all"

function Items() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const items = useQuery(openingItemsQuery(search.page, search.itemType))
  const columns: DataTableColumn<OpeningItem>[] = [
    {
      id: "type",
      header: "Jenis",
      cell: (i) => OPENING_ITEM_TYPE_LABELS[i.itemType],
    },
    { id: "party", header: "Pihak", cell: (i) => i.partyCode },
    {
      id: "doc",
      header: "Dokumen Lama",
      cell: (i) => (
        <div className="flex flex-col">
          <span className="font-medium">{i.documentNumber}</span>
          <span className="text-xs text-muted-foreground">
            {formatDate(i.documentDate)} · jatuh tempo {formatDate(i.dueDate)}
          </span>
        </div>
      ),
    },
    { id: "account", header: "Akun", cell: (i) => i.accountCode },
    {
      id: "amount",
      header: "Nilai",
      align: "right",
      cell: (i) => formatRupiah(i.amount),
    },
    {
      id: "open",
      header: "Sisa per Cut-over",
      align: "right",
      cell: (i) => formatRupiah(i.openAmount),
    },
    {
      id: "outstanding",
      header: "Sisa Saat Ini",
      align: "right",
      cell: (i) => formatRupiah(i.outstanding),
    },
  ]
  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <Select
          value={search.itemType ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                page: 1,
                itemType: v === ALL ? undefined : (v as OpeningItemType),
              }),
            })
          }
        >
          <SelectTrigger aria-label="Jenis saldo" className="w-44">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Piutang & utang"
                  : OPENING_ITEM_TYPE_LABELS[v as OpeningItemType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Piutang & utang</SelectItem>
            <SelectItem value="receivable">Piutang</SelectItem>
            <SelectItem value="payable">Utang</SelectItem>
          </SelectContent>
        </Select>
        {items.data ? (
          <span className="text-sm text-muted-foreground">
            Total sisa: {formatRupiah(items.data.outstandingTotal)}
          </span>
        ) : null}
      </div>
      <DataTable
        columns={columns}
        rows={items.data?.items}
        getRowId={(i) => i.id}
        loading={items.isPending}
        fetching={items.isFetching}
        page={search.page}
        totalPages={items.data?.totalPages ?? 0}
        totalItems={items.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada piutang/utang terbuka. Impor sheet Piutang Terbuka dan Utang Terbuka."
      />
    </>
  )
}

function Reconciliation() {
  const rec = useQuery(cutoverReconciliationQuery)
  const data = rec.data
  return (
    <LoadingArea loading={rec.isPending}>
      {data ? (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-muted-foreground">
            {data.cutoverDate
              ? `Per ${formatDate(data.cutoverDate)}. Saldo GL diambil dari ${data.ledgerSource === "posted" ? "jurnal terposting" : "Neraca Saldo Awal yang belum diposting"}. Toleransi 0.`
              : "Isi tanggal cut-over di tab Neraca Saldo Awal."}
          </p>
          {data.checks.length > 0 ? (
            <p className="text-sm font-medium">
              {data.passed
                ? "Semua pengecekan lolos."
                : "Masih ada pengecekan yang belum lolos."}
            </p>
          ) : null}
          <div className="overflow-x-auto rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10" />
                  <TableHead>Pengecekan</TableHead>
                  <TableHead>Akun</TableHead>
                  <TableHead className="text-right">Sub-ledger</TableHead>
                  <TableHead className="text-right">GL</TableHead>
                  <TableHead className="text-right">Selisih</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.checks.map((c, i) => (
                  <TableRow key={i}>
                    <TableCell>
                      {c.status === "ok" ? (
                        <CheckCircle2Icon
                          aria-label="Lolos"
                          className="size-4 text-emerald-600"
                        />
                      ) : c.status === "failed" ? (
                        <XCircleIcon
                          aria-label="Tidak lolos"
                          className="size-4 text-destructive"
                        />
                      ) : (
                        <MinusCircleIcon
                          aria-label="Dilewati"
                          className="size-4 text-muted-foreground"
                        />
                      )}
                    </TableCell>
                    <TableCell className="whitespace-normal">
                      {c.label}
                      {c.note ? (
                        <span className="block text-xs text-muted-foreground">
                          {c.note}
                        </span>
                      ) : null}
                    </TableCell>
                    <TableCell>{c.accountCode}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(c.subledger)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(c.ledger)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(c.difference)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      ) : null}
    </LoadingArea>
  )
}
