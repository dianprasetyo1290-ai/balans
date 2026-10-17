import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { UploadIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
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
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { accountsQuery } from "../api/accounting-api"
import {
  autoMatchStatement,
  bankStatementQuery,
  bankStatementsQuery,
  cashBankAccountsQuery,
  confirmStatementLine,
  confirmStatementSuggestions,
  createStatementLineEntry,
  importBankStatement,
  lineCandidatesQuery,
  matchStatementLine,
  requestStatementUpload,
  unmatchStatementLine,
  type BankMatchCandidate,
  type BankStatement,
  type BankStatementDetail,
  type BankStatementLine,
} from "../api/finance-api"
import {
  bankOptions,
  fieldErrors,
  FinanceStatusBadge,
  Info,
  SimpleSelect,
  useFinanceRefresh,
} from "./finance-shared"

const listRoute = getRouteApi("/_app/finance/bank-reconciliation/")

const RULE_LABELS: Record<string, string> = {
  reference: "Referensi",
  amount_date: "Nominal + tanggal",
  manual: "Manual",
}

const KIND_LABELS: Record<string, string> = {
  journal_line: "Transaksi tercatat",
  document: "Dokumen terbuka",
  new_entry: "Jurnal baru",
}

/** Keuangan & Akuntansi › Rekonsiliasi Bank: daftar file mutasi yang diimpor (S-402). */
export function BankReconciliationListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const banks = useQuery(cashBankAccountsQuery())
  const statements = useQuery(
    bankStatementsQuery({
      page: search.page,
      bankAccountId: search.bankAccountId,
    })
  )
  const [importing, setImporting] = useState(false)

  const columns: DataTableColumn<BankStatement>[] = [
    {
      id: "file",
      header: "File Mutasi",
      cell: (s) => (
        <Link
          to="/finance/bank-reconciliation/$id"
          params={{ id: s.id }}
          className="font-medium text-primary underline-offset-4 hover:underline"
        >
          {s.fileName}
        </Link>
      ),
    },
    { id: "bank", header: "Rekening", cell: (s) => s.bankAccountName },
    {
      id: "period",
      header: "Periode",
      cell: (s) => (
        <span className="whitespace-nowrap">
          {formatDate(s.periodFrom)} – {formatDate(s.periodTo)}
        </span>
      ),
    },
    {
      id: "in",
      header: "Masuk",
      align: "right",
      cell: (s) => formatRupiah(s.totalIn),
    },
    {
      id: "out",
      header: "Keluar",
      align: "right",
      cell: (s) => formatRupiah(s.totalOut),
    },
    {
      id: "progress",
      header: "Cocok",
      cell: (s) => (
        <div className="flex flex-wrap gap-1">
          <Badge
            variant={s.matchedCount === s.lineCount ? "secondary" : "outline"}
          >
            {s.matchedCount}/{s.lineCount}
          </Badge>
          {s.suggestedCount > 0 ? (
            <Badge variant="outline">{s.suggestedCount} usulan</Badge>
          ) : null}
        </div>
      ),
    },
    {
      id: "created",
      header: "Diimpor",
      cell: (s) => formatDateTime(s.createdAt),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Rekonsiliasi Bank
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Impor file mutasi CSV/Excel dari bank. Sistem mengusulkan pasangan
            berdasarkan referensi, lalu nominal dan tanggal; usulan dikonfirmasi
            Finance & Accounting.
          </p>
        </div>
        {can("accounting.bank_reconciliation.import") ? (
          <Button onClick={() => setImporting(true)}>
            <UploadIcon /> Impor Mutasi Bank
          </Button>
        ) : null}
      </div>
      <SimpleSelect
        className="w-full sm:w-80"
        aria-label="Filter rekening"
        value={search.bankAccountId ?? "all"}
        onValueChange={(v) =>
          void navigate({
            search: (s) => ({
              ...s,
              bankAccountId: v === "all" ? undefined : v,
              page: 1,
            }),
          })
        }
        placeholder="Semua rekening"
        options={[
          { value: "all", label: "Semua rekening" },
          ...bankOptions(banks.data, true),
        ]}
      />
      <DataTable
        columns={columns}
        rows={statements.data?.items}
        getRowId={(s) => s.id}
        loading={statements.isPending}
        fetching={statements.isFetching}
        page={search.page}
        totalPages={statements.data?.totalPages ?? 0}
        totalItems={statements.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada file mutasi yang diimpor."
      />
      <Dialog open={importing} onOpenChange={setImporting}>
        {importing ? <ImportForm onClose={() => setImporting(false)} /> : null}
      </Dialog>
    </div>
  )
}

function ImportForm({ onClose }: { onClose: () => void }) {
  const navigate = useNavigate()
  const refresh = useFinanceRefresh()
  const banks = useQuery(cashBankAccountsQuery())
  const [bankAccountId, setBankAccountId] = useState("")
  const [fileId, setFileId] = useState<string>()
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      importBankStatement({ bankAccountId, fileId: fileId ?? "" }, key),
    onSuccess: async (d) => {
      toast.success(
        `${d.lineCount} baris diimpor, ${d.suggestedCount} usulan pencocokan`
      )
      onClose()
      await refresh()
      void navigate({
        to: "/finance/bank-reconciliation/$id",
        params: { id: d.id },
      })
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setErrors(p.fields)
        return
      }
      showErrorDialog(p)
    },
  })
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Impor Mutasi Bank</DialogTitle>
        <DialogDescription>
          File CSV atau Excel (.xlsx) dengan kolom Tanggal, Keterangan,
          Referensi, Debit/Kredit (atau Nominal + D/K), dan Saldo. Satu file
          hanya bisa diimpor sekali.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          const next: Record<string, string> = {}
          if (!bankAccountId) next.bankAccountId = "Rekening wajib dipilih"
          if (!fileId) next.fileId = "File mutasi wajib diunggah"
          setErrors(next)
          if (Object.keys(next).length === 0) save.mutate()
        }}
      >
        <Field data-invalid={!!errors.bankAccountId}>
          <FieldLabel htmlFor="st-bank">Rekening</FieldLabel>
          <SimpleSelect
            id="st-bank"
            value={bankAccountId}
            onValueChange={setBankAccountId}
            options={bankOptions(banks.data, true)}
            placeholder="Pilih rekening bank"
            aria-invalid={!!errors.bankAccountId}
          />
          <FieldError errors={fieldErrors(errors.bankAccountId)} />
        </Field>
        <Field data-invalid={!!errors.fileId}>
          <FieldLabel>File Mutasi</FieldLabel>
          <FileUpload
            label="Pilih File CSV/Excel"
            accept=".csv,text/csv,.xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            maxSizeBytes={10 * 1024 * 1024}
            requestUpload={requestStatementUpload}
            onUploaded={(id) => setFileId(id)}
            onRemoved={() => setFileId(undefined)}
            onError={(m) => toast.error(m)}
          />
          <FieldError errors={fieldErrors(errors.fileId)} />
        </Field>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Mengimpor…"
          >
            Impor Mutasi
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

// --- Detail -----------------------------------------------------------------------------------

/** Detail file mutasi: baris, usulan, konfirmasi, pencocokan manual, jurnal baru. */
export function BankStatementDetailPage({ id }: { id: string }) {
  const statement = useQuery(bankStatementQuery(id))
  return (
    <LoadingArea loading={statement.isPending}>
      {statement.data ? <StatementDetail statement={statement.data} /> : null}
    </LoadingArea>
  )
}

function StatementDetail({ statement }: { statement: BankStatementDetail }) {
  const refresh = useFinanceRefresh()
  const [matching, setMatching] = useState<BankStatementLine | null>(null)
  const [entry, setEntry] = useState<BankStatementLine | null>(null)
  const canPost = statement.lines.some((l) => l.allowedActions.length > 0)

  const bulk = useIdempotentMutation<number, Error, "auto" | "confirm">({
    mutationFn: async (kind, key) =>
      kind === "auto"
        ? (await autoMatchStatement(statement.id, key)).suggestedCount
        : (await confirmStatementSuggestions(statement.id, key)).confirmed,
    onSuccess: async (n, kind) => {
      toast.success(
        kind === "auto" ? `${n} usulan pencocokan` : `${n} usulan dikonfirmasi`
      )
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const lineAction = useIdempotentMutation({
    mutationFn: (
      { line, kind }: { line: BankStatementLine; kind: "confirm" | "unmatch" },
      key
    ) =>
      kind === "confirm"
        ? confirmStatementLine(line.id, line.version, key)
        : unmatchStatementLine(line.id, line.version, key),
    onSuccess: async (_, v) => {
      toast.success(
        v.kind === "confirm" ? "Usulan dikonfirmasi" : "Pencocokan dilepas"
      )
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const columns: DataTableColumn<BankStatementLine>[] = [
    { id: "no", header: "#", cell: (l) => l.lineNo },
    {
      id: "date",
      header: "Tanggal",
      cell: (l) => (
        <span className="whitespace-nowrap">
          {formatDate(l.transactionDate)}
        </span>
      ),
    },
    {
      id: "desc",
      header: "Keterangan",
      cell: (l) => (
        <div className="flex max-w-xs flex-col">
          <span className="truncate">{l.description || "—"}</span>
          {l.reference ? (
            <span className="text-xs text-muted-foreground">
              Ref {l.reference}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "amount",
      header: "Nominal",
      align: "right",
      cell: (l) => (
        <span
          className={l.amount.startsWith("-") ? "text-destructive" : undefined}
        >
          {formatRupiah(l.amount)}
        </span>
      ),
    },
    {
      id: "match",
      header: "Pasangan",
      cell: (l) =>
        l.status === "unmatched" ? (
          <span className="text-muted-foreground">Belum ada pasangan</span>
        ) : (
          <div className="flex max-w-xs flex-col">
            <span className="truncate">
              {l.candidateLabel || l.journalNumber}
            </span>
            <span className="text-xs text-muted-foreground">
              {l.matchKind ? KIND_LABELS[l.matchKind] : ""}
              {l.matchRule ? ` · ${RULE_LABELS[l.matchRule]}` : ""}
            </span>
          </div>
        ),
    },
    {
      id: "status",
      header: "Status",
      cell: (l) => <FinanceStatusBadge status={l.status} />,
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (l) => (
        <div className="flex justify-end gap-2">
          {l.allowedActions.includes("confirm") ? (
            <Button
              size="sm"
              onClick={() => lineAction.mutate({ line: l, kind: "confirm" })}
            >
              Konfirmasi
            </Button>
          ) : null}
          {l.allowedActions.includes("match") ? (
            <Button size="sm" variant="outline" onClick={() => setMatching(l)}>
              Cocokkan
            </Button>
          ) : null}
          {l.allowedActions.includes("create_entry") ? (
            <Button size="sm" variant="ghost" onClick={() => setEntry(l)}>
              Buat Jurnal
            </Button>
          ) : null}
          {l.allowedActions.includes("unmatch") ? (
            <Button
              size="sm"
              variant="ghost"
              onClick={() => lineAction.mutate({ line: l, kind: "unmatch" })}
            >
              Lepas
            </Button>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {statement.fileName}
          </h1>
          <p className="text-sm text-muted-foreground">
            {statement.bankAccountName}
          </p>
        </div>
        {canPost ? (
          <div className="flex flex-wrap gap-2">
            <LoadingButton
              variant="outline"
              loading={bulk.isPending && bulk.variables === "auto"}
              onClick={() => bulk.mutate("auto")}
            >
              Cocokkan Otomatis
            </LoadingButton>
            {statement.suggestedCount > 0 ? (
              <LoadingButton
                loading={bulk.isPending && bulk.variables === "confirm"}
                onClick={() => bulk.mutate("confirm")}
              >
                Konfirmasi Semua Usulan
              </LoadingButton>
            ) : null}
          </div>
        ) : null}
      </div>
      <dl className="grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-4">
        <Info
          label="Periode"
          value={`${formatDate(statement.periodFrom)} – ${formatDate(statement.periodTo)}`}
        />
        <Info label="Total masuk" value={formatRupiah(statement.totalIn)} />
        <Info label="Total keluar" value={formatRupiah(statement.totalOut)} />
        <Info
          label="Direkonsiliasi"
          value={`${statement.matchedCount} dari ${statement.lineCount} baris`}
        />
      </dl>
      <DataTable
        columns={columns}
        rows={statement.lines}
        getRowId={(l) => l.id}
        loading={false}
        page={1}
        totalPages={1}
        totalItems={statement.lines.length}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="File tidak berisi baris."
      />
      <Dialog open={!!matching} onOpenChange={(o) => !o && setMatching(null)}>
        {matching ? (
          <MatchForm line={matching} onClose={() => setMatching(null)} />
        ) : null}
      </Dialog>
      <Dialog open={!!entry} onOpenChange={(o) => !o && setEntry(null)}>
        {entry ? (
          <EntryForm line={entry} onClose={() => setEntry(null)} />
        ) : null}
      </Dialog>
    </div>
  )
}

function MatchForm({
  line,
  onClose,
}: {
  line: BankStatementLine
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const candidates = useQuery(lineCandidatesQuery(line.id))
  const save = useIdempotentMutation({
    mutationFn: (c: BankMatchCandidate, key) =>
      matchStatementLine(
        line.id,
        c.kind === "journal_line"
          ? { version: line.version, journalLineId: c.journalLineId }
          : {
              version: line.version,
              source: c.source,
              documentId: c.documentId,
            },
        key
      ),
    onSuccess: async () => {
      toast.success("Baris mutasi dicocokkan")
      onClose()
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <DialogContent className="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Cocokkan Manual</DialogTitle>
        <DialogDescription>
          Mutasi {formatDate(line.transactionDate)} ·{" "}
          {formatRupiah(line.amount)}. Pilih transaksi tercatat (nominal harus
          sama) atau dokumen terbuka; dokumen dibayar otomatis dengan pembayaran
          BYR.
        </DialogDescription>
      </DialogHeader>
      <LoadingArea loading={candidates.isPending}>
        <ul className="flex max-h-96 flex-col divide-y overflow-y-auto rounded-md border">
          {(candidates.data ?? []).map((c) => (
            <li
              key={`${c.kind}-${c.journalLineId ?? c.documentId}`}
              className="flex items-center justify-between gap-3 p-3 text-sm"
            >
              <div className="flex min-w-0 flex-col">
                <span className="truncate font-medium">{c.label}</span>
                <span className="text-xs text-muted-foreground">
                  {KIND_LABELS[c.kind]} · {formatDate(c.date)}
                </span>
              </div>
              <div className="flex items-center gap-3">
                <span className="whitespace-nowrap tabular-nums">
                  {formatRupiah(c.amount)}
                </span>
                <Button
                  size="sm"
                  disabled={save.isPending}
                  onClick={() => save.mutate(c)}
                >
                  Pilih
                </Button>
              </div>
            </li>
          ))}
          {candidates.data?.length === 0 ? (
            <li className="p-3 text-sm text-muted-foreground">
              Tidak ada kandidat searah. Gunakan Buat Jurnal.
            </li>
          ) : null}
        </ul>
      </LoadingArea>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Tutup
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}

function EntryForm({
  line,
  onClose,
}: {
  line: BankStatementLine
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const gls = useQuery(accountsQuery(false))
  const banks = useQuery(cashBankAccountsQuery())
  const [counterAccountId, setCounterAccountId] = useState("")
  const [description, setDescription] = useState(line.description)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const bankGLs = new Set((banks.data ?? []).map((b) => b.accountId))
  const options = (gls.data ?? [])
    .filter((a) => a.isPostable && a.allowManualPosting && !bankGLs.has(a.id))
    .map((a) => ({ value: a.id, label: `${a.code} ${a.name}` }))
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createStatementLineEntry(
        line.id,
        { version: line.version, counterAccountId, description },
        key
      ),
    onSuccess: async () => {
      toast.success("Jurnal dibuat dan baris direkonsiliasi")
      onClose()
      await refresh()
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setErrors(p.fields)
        return
      }
      showErrorDialog(p)
    },
  })
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Buat Jurnal dari Mutasi</DialogTitle>
        <DialogDescription>
          Untuk mutasi tanpa pasangan seperti biaya administrasi, bunga, atau
          pajak bunga. Sistem membuat pembayaran{" "}
          {line.amount.startsWith("-") ? "keluar" : "masuk"}{" "}
          {formatRupiah(line.amount.replace("-", ""))} yang langsung diposting.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!counterAccountId) {
            setErrors({ counterAccountId: "Akun lawan wajib dipilih" })
            return
          }
          save.mutate()
        }}
      >
        <Field data-invalid={!!errors.counterAccountId}>
          <FieldLabel htmlFor="en-account">Akun Lawan</FieldLabel>
          <SimpleSelect
            id="en-account"
            value={counterAccountId}
            onValueChange={setCounterAccountId}
            options={options}
            placeholder="Mis. 7202 Beban Bank"
            aria-invalid={!!errors.counterAccountId}
          />
          <FieldError errors={fieldErrors(errors.counterAccountId)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="en-desc">Keterangan</FieldLabel>
          <Input
            id="en-desc"
            value={description}
            maxLength={500}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Buat Jurnal
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
