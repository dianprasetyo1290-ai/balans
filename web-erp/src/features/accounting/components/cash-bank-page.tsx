import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { PageTabs } from "@/components/common/page-tabs"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
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
import { Textarea } from "@/components/ui/textarea"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { accountsQuery } from "../api/accounting-api"
import {
  bankMutationsQuery,
  cancelCashPayment,
  cashBankAccountQuery,
  cashBankAccountsQuery,
  cashPaymentsQuery,
  createCashBankAccount,
  createCashPayment,
  deleteCashBankAccount,
  DIRECTION_LABELS,
  postCashPayment,
  restoreCashBankAccount,
  updateCashBankAccount,
  updateCashPayment,
  type CashBankAccount,
  type CashPayment,
  type PaymentDirection,
} from "../api/finance-api"
import { Checkbox } from "./account-form-dialog"
import {
  bankOptions,
  fieldErrors,
  ReasonDialog,
  ReceiptUpload,
  SimpleSelect,
  useFinanceRefresh,
} from "./finance-shared"

const route = getRouteApi("/_app/finance/cash-bank")

type Tab = "accounts" | "payments" | "mutations"

/** Keuangan & Akuntansi › Kas & Bank: rekening, pembayaran masuk/keluar, mutasi per rekening (S-401). */
export function CashBankPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const tab = search.tab
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Kas & Bank</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Rekening kas dan bank terhubung ke akun COA 11xx. Pembayaran keluar
          membutuhkan konfirmasi ulang password/kode (re-autentikasi).
        </p>
      </div>
      <PageTabs<Tab>
        label="Bagian Kas & Bank"
        tabs={[
          { id: "accounts", label: "Rekening" },
          { id: "payments", label: "Pembayaran" },
          { id: "mutations", label: "Mutasi Rekening" },
        ]}
        value={tab}
        onValueChange={(t) =>
          void navigate({ search: (s) => ({ ...s, tab: t, page: 1 }) })
        }
      />
      {tab === "accounts" ? <AccountsTab /> : null}
      {tab === "payments" ? <PaymentsTab /> : null}
      {tab === "mutations" ? <MutationsTab /> : null}
    </div>
  )
}

// --- Rekening ---------------------------------------------------------------------------------

function AccountsTab() {
  const { can } = usePermissions()
  const [deleted, setDeleted] = useState(false)
  const accounts = useQuery(cashBankAccountsQuery(true, deleted))
  const [editing, setEditing] = useState<CashBankAccount | "new" | null>(null)
  const [confirm, setConfirm] = useState<{
    a: CashBankAccount
    kind: "delete" | "restore"
  } | null>(null)
  const refresh = useFinanceRefresh()
  const action = useIdempotentMutation({
    mutationFn: (c: { a: CashBankAccount; kind: "delete" | "restore" }, key) =>
      c.kind === "delete"
        ? deleteCashBankAccount(c.a.id, c.a.version, key)
        : restoreCashBankAccount(c.a.id, c.a.version, key),
    onSuccess: async (_, c) => {
      setConfirm(null)
      toast.success(
        c.kind === "delete" ? "Rekening dihapus" : "Rekening dipulihkan"
      )
      await refresh()
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<CashBankAccount>[] = [
    {
      id: "code",
      header: "Kode",
      cell: (a) => <span className="font-medium">{a.code}</span>,
    },
    {
      id: "name",
      header: "Rekening",
      cell: (a) => (
        <div className="flex flex-col">
          <span>{a.name}</span>
          <span className="text-xs text-muted-foreground">
            {a.kind === "cash"
              ? "Kas tunai"
              : `${a.bankName} ${a.accountNumberMasked} · a.n. ${a.accountHolder || "—"}`}
          </span>
        </div>
      ),
    },
    {
      id: "gl",
      header: "Akun COA",
      cell: (a) => `${a.accountCode} ${a.accountName}`,
    },
    {
      id: "status",
      header: "Status",
      cell: (a) => (
        <RecordStatusBadge isActive={a.isActive} deletedAt={a.deletedAt} />
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (a) => (
        <div className="flex justify-end gap-2">
          {a.allowedActions.includes("edit") ? (
            <Button variant="outline" size="sm" onClick={() => setEditing(a)}>
              Ubah
            </Button>
          ) : null}
          {a.allowedActions.includes("delete") ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setConfirm({ a, kind: "delete" })}
            >
              Hapus
            </Button>
          ) : null}
          {a.allowedActions.includes("restore") ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setConfirm({ a, kind: "restore" })}
            >
              Pulihkan
            </Button>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Checkbox
          label="Tampilkan yang dihapus"
          checked={deleted}
          onChange={setDeleted}
        />
        {can("accounting.payment.write") ? (
          <Button onClick={() => setEditing("new")}>
            <PlusIcon /> Tambah Rekening
          </Button>
        ) : null}
      </div>
      <DataTable
        columns={columns}
        rows={accounts.data}
        getRowId={(a) => a.id}
        loading={accounts.isPending}
        page={1}
        totalPages={1}
        totalItems={accounts.data?.length ?? 0}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="Belum ada rekening."
      />
      <BankAccountDialog value={editing} onClose={() => setEditing(null)} />
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={
          confirm?.kind === "delete" ? "Hapus Rekening" : "Pulihkan Rekening"
        }
        description={
          confirm?.kind === "delete"
            ? `Rekening ${confirm.a.code} tidak bisa dipakai transaksi baru. Dokumen lama tetap menampilkan rekening ini.`
            : `Rekening ${confirm?.a.code ?? ""} bisa dipakai kembali.`
        }
        confirmLabel={
          confirm?.kind === "delete" ? "Hapus Rekening" : "Pulihkan Rekening"
        }
        destructive={confirm?.kind === "delete"}
        loading={action.isPending}
        onConfirm={() => confirm && action.mutate(confirm)}
      />
    </div>
  )
}

function BankAccountDialog({
  value,
  onClose,
}: {
  value: CashBankAccount | "new" | null
  onClose: () => void
}) {
  const open = value !== null
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      {value === "new" ? <BankAccountForm onClose={onClose} /> : null}
      {value && value !== "new" ? (
        <BankAccountEdit id={value.id} onClose={onClose} />
      ) : null}
    </Dialog>
  )
}

function BankAccountEdit({ id, onClose }: { id: string; onClose: () => void }) {
  const detail = useQuery(cashBankAccountQuery(id))
  if (!detail.data) {
    return (
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Ubah Rekening</DialogTitle>
          <DialogDescription>Memuat…</DialogDescription>
        </DialogHeader>
      </DialogContent>
    )
  }
  return <BankAccountForm value={detail.data} onClose={onClose} />
}

function BankAccountForm({
  value,
  onClose,
}: {
  value?: CashBankAccount
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const gls = useQuery(accountsQuery(false))
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [kind, setKind] = useState<"bank" | "cash">(value?.kind ?? "bank")
  const [bankName, setBankName] = useState(value?.bankName ?? "")
  const [accountNumber, setAccountNumber] = useState(value?.accountNumber ?? "")
  const [accountHolder, setAccountHolder] = useState(value?.accountHolder ?? "")
  const [accountId, setAccountId] = useState(value?.accountId ?? "")
  const [isActive, setIsActive] = useState(value?.isActive ?? true)
  const [description, setDescription] = useState(value?.description ?? "")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const glOptions = (gls.data ?? [])
    .filter((a) => a.code.startsWith("11") && a.isPostable && a.code !== "1102")
    .map((a) => ({ value: a.id, label: `${a.code} ${a.name}` }))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        name,
        kind,
        bankName,
        accountNumber,
        accountHolder,
        accountId,
        isActive,
        description,
      }
      return value
        ? updateCashBankAccount(
            value.id,
            { ...body, version: value.version },
            key
          )
        : createCashBankAccount({ ...body, code }, key)
    },
    onSuccess: async () => {
      toast.success(value ? "Rekening diperbarui" : "Rekening ditambahkan")
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
        <DialogTitle>{value ? "Ubah Rekening" : "Tambah Rekening"}</DialogTitle>
        <DialogDescription>
          Nomor rekening disimpan terenkripsi. Satu akun COA hanya untuk satu
          rekening agar mutasi rekening sama dengan Buku Besar.
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
        <div className="grid gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="ba-code">Kode</FieldLabel>
            <Input
              id="ba-code"
              value={code}
              disabled={!!value}
              placeholder="BCA-OPS"
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <FieldError errors={fieldErrors(errors.code)} />
          </Field>
          <Field data-invalid={!!errors.kind}>
            <FieldLabel htmlFor="ba-kind">Jenis</FieldLabel>
            <SimpleSelect
              id="ba-kind"
              value={kind}
              onValueChange={(v) => setKind(v as "bank" | "cash")}
              placeholder="Pilih jenis"
              options={[
                { value: "bank", label: "Rekening bank" },
                { value: "cash", label: "Kas tunai" },
              ]}
            />
          </Field>
        </div>
        <Field data-invalid={!!errors.name}>
          <FieldLabel htmlFor="ba-name">Nama</FieldLabel>
          <Input
            id="ba-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <FieldError errors={fieldErrors(errors.name)} />
        </Field>
        {kind === "bank" ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.bankName}>
              <FieldLabel htmlFor="ba-bank">Nama Bank</FieldLabel>
              <Input
                id="ba-bank"
                value={bankName}
                onChange={(e) => setBankName(e.target.value)}
              />
              <FieldError errors={fieldErrors(errors.bankName)} />
            </Field>
            <Field data-invalid={!!errors.accountNumber}>
              <FieldLabel htmlFor="ba-number">Nomor Rekening</FieldLabel>
              <Input
                id="ba-number"
                inputMode="numeric"
                value={accountNumber}
                onChange={(e) => setAccountNumber(e.target.value)}
              />
              <FieldError errors={fieldErrors(errors.accountNumber)} />
            </Field>
            <Field className="sm:col-span-2">
              <FieldLabel htmlFor="ba-holder">Atas Nama</FieldLabel>
              <Input
                id="ba-holder"
                value={accountHolder}
                onChange={(e) => setAccountHolder(e.target.value)}
              />
            </Field>
          </div>
        ) : null}
        <Field data-invalid={!!errors.accountId}>
          <FieldLabel htmlFor="ba-gl">Akun COA</FieldLabel>
          <SimpleSelect
            id="ba-gl"
            value={accountId}
            onValueChange={setAccountId}
            options={
              value && !glOptions.some((o) => o.value === value.accountId)
                ? [
                    ...glOptions,
                    {
                      value: value.accountId,
                      label: `${value.accountCode} ${value.accountName}`,
                    },
                  ]
                : glOptions
            }
            placeholder="Pilih akun Kas & Bank"
            aria-invalid={!!errors.accountId}
          />
          <FieldError errors={fieldErrors(errors.accountId)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ba-desc">Keterangan</FieldLabel>
          <Textarea
            id="ba-desc"
            value={description}
            maxLength={500}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <Checkbox label="Aktif" checked={isActive} onChange={setIsActive} />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Simpan Rekening
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

// --- Pembayaran -------------------------------------------------------------------------------

function PaymentsTab() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const payments = useQuery(
    cashPaymentsQuery({
      page: search.page,
      q: search.q,
      direction: search.direction,
      bankAccountId: search.accountId,
    })
  )
  const [editing, setEditing] = useState<CashPayment | "new" | null>(null)
  const [posting, setPosting] = useState<CashPayment | null>(null)
  const [cancelling, setCancelling] = useState<CashPayment | null>(null)
  const refresh = useFinanceRefresh()

  const post = useIdempotentMutation({
    mutationFn: (p: CashPayment, key) => postCashPayment(p.id, p.version, key),
    onSuccess: async (p) => {
      setPosting(null)
      toast.success(`Pembayaran ${p.number ?? ""} diposting`)
      await refresh()
    },
    onError: (err) => {
      setPosting(null)
      showErrorDialog(parseProblem(err))
    },
  })
  const cancel = useIdempotentMutation({
    mutationFn: ({ p, reason }: { p: CashPayment; reason: string }, key) =>
      cancelCashPayment(p.id, p.version, reason, key),
    onSuccess: async () => {
      setCancelling(null)
      toast.success("Pembayaran dibatalkan")
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const columns: DataTableColumn<CashPayment>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (p) => (
        <span className="font-medium whitespace-nowrap">
          {p.number ?? "Draft"}
        </span>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (p) => (
        <span className="whitespace-nowrap">{formatDate(p.paymentDate)}</span>
      ),
    },
    {
      id: "direction",
      header: "Arah",
      cell: (p) => (
        <Badge variant={p.direction === "incoming" ? "secondary" : "outline"}>
          {DIRECTION_LABELS[p.direction]}
        </Badge>
      ),
    },
    { id: "bank", header: "Rekening", cell: (p) => p.bankAccountName },
    {
      id: "desc",
      header: "Keterangan",
      cell: (p) => (
        <div className="flex max-w-sm flex-col">
          <span className="truncate">
            {p.description || p.partyName || "—"}
          </span>
          <span className="text-xs text-muted-foreground">
            {p.counterAccountCode} {p.counterAccountName}
            {p.sourceLabel ? ` · ${p.sourceLabel}` : ""}
            {p.reference ? ` · Ref ${p.reference}` : ""}
          </span>
        </div>
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
      cell: (p) => (
        <div className="flex flex-wrap gap-1">
          <StatusBadge status={p.status} />
          {p.reconciled ? (
            <Badge variant="outline">Direkonsiliasi</Badge>
          ) : null}
        </div>
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (p) => (
        <div className="flex justify-end gap-2">
          {p.allowedActions.includes("edit") ? (
            <Button size="sm" variant="outline" onClick={() => setEditing(p)}>
              Ubah
            </Button>
          ) : null}
          {p.allowedActions.includes("post") ? (
            <Button size="sm" onClick={() => setPosting(p)}>
              Posting
            </Button>
          ) : null}
          {p.allowedActions.includes("cancel") ? (
            <Button size="sm" variant="ghost" onClick={() => setCancelling(p)}>
              Batalkan
            </Button>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-3">
          <form
            onSubmit={(e) => {
              e.preventDefault()
              const q = new FormData(e.currentTarget).get("q")
              void navigate({
                search: (s) => ({
                  ...s,
                  q: String(q ?? "") || undefined,
                  page: 1,
                }),
              })
            }}
          >
            <Input
              name="q"
              aria-label="Cari pembayaran"
              placeholder="Cari nomor, pihak, referensi"
              defaultValue={search.q}
              className="w-64"
            />
          </form>
          <SimpleSelect
            className="w-40"
            aria-label="Filter arah"
            value={search.direction ?? "all"}
            onValueChange={(v) =>
              void navigate({
                search: (s) => ({
                  ...s,
                  direction: v === "all" ? undefined : (v as PaymentDirection),
                  page: 1,
                }),
              })
            }
            placeholder="Semua arah"
            options={[
              { value: "all", label: "Semua arah" },
              { value: "incoming", label: "Masuk" },
              { value: "outgoing", label: "Keluar" },
            ]}
          />
        </div>
        {can("accounting.payment.write") ? (
          <Button onClick={() => setEditing("new")}>
            <PlusIcon /> Catat Pembayaran
          </Button>
        ) : null}
      </div>
      <DataTable
        columns={columns}
        rows={payments.data?.items}
        getRowId={(p) => p.id}
        loading={payments.isPending}
        fetching={payments.isFetching}
        page={search.page}
        totalPages={payments.data?.totalPages ?? 0}
        totalItems={payments.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada pembayaran."
      />
      <Dialog
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
      >
        {editing !== null ? (
          <PaymentForm
            value={editing === "new" ? undefined : editing}
            onClose={() => setEditing(null)}
          />
        ) : null}
      </Dialog>
      <ConfirmDialog
        open={!!posting}
        onOpenChange={(o) => !o && setPosting(null)}
        title="Posting Pembayaran"
        description={
          posting
            ? `Pembayaran ${DIRECTION_LABELS[posting.direction].toLowerCase()} ${formatRupiah(posting.amount)} diberi nomor BYR dan dijurnal (A-PB).${posting.direction === "outgoing" ? " Anda akan diminta konfirmasi ulang password/kode." : ""}`
            : ""
        }
        confirmLabel="Posting Pembayaran"
        loading={post.isPending}
        onConfirm={() => posting && post.mutate(posting)}
      />
      <ReasonDialog
        open={!!cancelling}
        onOpenChange={(o) => !o && setCancelling(null)}
        title="Batalkan Pembayaran"
        description="Pembayaran yang sudah diposting dibatalkan dengan jurnal pembalik."
        confirmLabel="Batalkan Pembayaran"
        loading={cancel.isPending}
        onConfirm={(reason) =>
          cancelling && cancel.mutate({ p: cancelling, reason })
        }
      />
    </div>
  )
}

function PaymentForm({
  value,
  onClose,
}: {
  value?: CashPayment
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const banks = useQuery(cashBankAccountsQuery())
  const gls = useQuery(accountsQuery(false))
  const [direction, setDirection] = useState<PaymentDirection>(
    value?.direction ?? "outgoing"
  )
  const [bankAccountId, setBankAccountId] = useState(value?.bankAccountId ?? "")
  const [paymentDate, setPaymentDate] = useState(
    value?.paymentDate ?? todayBusinessDate()
  )
  const [amount, setAmount] = useState(value?.amount ?? "")
  const [counterAccountId, setCounterAccountId] = useState(
    value?.counterAccountId ?? ""
  )
  const [partyName, setPartyName] = useState(value?.partyName ?? "")
  const [reference, setReference] = useState(value?.reference ?? "")
  const [description, setDescription] = useState(value?.description ?? "")
  const [proofFileId, setProofFileId] = useState<string | undefined>(
    value?.proofFileId
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const bankGLs = new Set((banks.data ?? []).map((b) => b.accountId))
  const counterOptions = (gls.data ?? [])
    .filter((a) => a.isPostable && a.allowManualPosting && !bankGLs.has(a.id))
    .map((a) => ({ value: a.id, label: `${a.code} ${a.name}` }))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        direction,
        bankAccountId,
        paymentDate,
        amount,
        counterAccountId,
        partyName,
        reference,
        description,
        proofFileId,
      }
      return value
        ? updateCashPayment(value.id, { ...body, version: value.version }, key)
        : createCashPayment(body, key)
    },
    onSuccess: async () => {
      toast.success("Draft pembayaran disimpan")
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
        <DialogTitle>
          {value ? "Ubah Pembayaran" : "Catat Pembayaran"}
        </DialogTitle>
        <DialogDescription>
          Penerimaan atau pengeluaran di luar piutang/utang (mis. setoran modal,
          biaya bank). Pelunasan invoice, tagihan pemasok, dan pengajuan biaya
          dicatat dari dokumennya.
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
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="pay-dir">Arah</FieldLabel>
            <SimpleSelect
              id="pay-dir"
              value={direction}
              onValueChange={(v) => setDirection(v as PaymentDirection)}
              placeholder="Pilih arah"
              options={[
                { value: "incoming", label: "Masuk (penerimaan)" },
                { value: "outgoing", label: "Keluar (pengeluaran)" },
              ]}
            />
          </Field>
          <Field data-invalid={!!errors.paymentDate}>
            <FieldLabel htmlFor="pay-date">Tanggal</FieldLabel>
            <DateInput
              id="pay-date"
              value={paymentDate}
              onValueChange={setPaymentDate}
            />
            <FieldError errors={fieldErrors(errors.paymentDate)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.bankAccountId}>
          <FieldLabel htmlFor="pay-bank">Rekening</FieldLabel>
          <SimpleSelect
            id="pay-bank"
            value={bankAccountId}
            onValueChange={setBankAccountId}
            options={bankOptions(banks.data)}
            placeholder="Pilih rekening"
            aria-invalid={!!errors.bankAccountId}
          />
          <FieldError errors={fieldErrors(errors.bankAccountId)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.amount}>
            <FieldLabel htmlFor="pay-amount">Nominal</FieldLabel>
            <MoneyInput
              id="pay-amount"
              value={amount}
              onValueChange={setAmount}
              aria-invalid={!!errors.amount}
            />
            <FieldError errors={fieldErrors(errors.amount)} />
          </Field>
          <Field data-invalid={!!errors.counterAccountId}>
            <FieldLabel htmlFor="pay-counter">Akun Lawan</FieldLabel>
            <SimpleSelect
              id="pay-counter"
              value={counterAccountId}
              onValueChange={setCounterAccountId}
              options={counterOptions}
              placeholder="Pilih akun"
              aria-invalid={!!errors.counterAccountId}
            />
            <FieldError errors={fieldErrors(errors.counterAccountId)} />
          </Field>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="pay-party">Pihak</FieldLabel>
            <Input
              id="pay-party"
              value={partyName}
              onChange={(e) => setPartyName(e.target.value)}
            />
          </Field>
          <Field data-invalid={!!errors.reference}>
            <FieldLabel htmlFor="pay-ref">Referensi Bank</FieldLabel>
            <Input
              id="pay-ref"
              value={reference}
              maxLength={100}
              onChange={(e) => setReference(e.target.value)}
            />
            <FieldError errors={fieldErrors(errors.reference)} />
          </Field>
        </div>
        <Field>
          <FieldLabel htmlFor="pay-desc">Keterangan</FieldLabel>
          <Textarea
            id="pay-desc"
            value={description}
            maxLength={500}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel>Bukti</FieldLabel>
          <ReceiptUpload
            ownerType="payment"
            onUploaded={setProofFileId}
            onRemoved={() => setProofFileId(undefined)}
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
            Simpan Draft
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

// --- Mutasi -----------------------------------------------------------------------------------

function MutationsTab() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const banks = useQuery(cashBankAccountsQuery())
  const today = todayBusinessDate()
  const accountId = search.accountId ?? banks.data?.[0]?.id ?? ""
  const dateFrom = search.dateFrom ?? `${today.slice(0, 8)}01`
  const dateTo = search.dateTo ?? today
  const mutations = useQuery({
    ...bankMutationsQuery({
      id: accountId,
      dateFrom,
      dateTo,
      page: search.page,
    }),
    enabled: !!accountId,
  })
  type Row = NonNullable<typeof mutations.data>["items"][number]
  const columns: DataTableColumn<Row>[] = [
    {
      id: "date",
      header: "Tanggal",
      cell: (m) => (
        <span className="whitespace-nowrap">{formatDate(m.businessDate)}</span>
      ),
    },
    {
      id: "number",
      header: "Jurnal",
      cell: (m) => <span className="whitespace-nowrap">{m.journalNumber}</span>,
    },
    {
      id: "desc",
      header: "Keterangan",
      cell: (m) => (
        <div className="flex max-w-md flex-col">
          <span className="truncate">{m.description}</span>
          {m.sourceLabel ? (
            <span className="text-xs text-muted-foreground">
              {m.sourceLabel}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "in",
      header: "Masuk",
      align: "right",
      cell: (m) => (m.amount.startsWith("-") ? "" : formatRupiah(m.amount)),
    },
    {
      id: "out",
      header: "Keluar",
      align: "right",
      cell: (m) =>
        m.amount.startsWith("-") ? formatRupiah(m.amount.slice(1)) : "",
    },
    {
      id: "balance",
      header: "Saldo",
      align: "right",
      cell: (m) => formatRupiah(m.balance),
    },
    {
      id: "rec",
      header: "Rekonsiliasi",
      cell: (m) =>
        m.reconciled ? (
          <Badge variant="secondary">Cocok</Badge>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
  ]
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-end gap-3">
        <Field className="w-full sm:w-80">
          <FieldLabel htmlFor="mut-bank">Rekening</FieldLabel>
          <SimpleSelect
            id="mut-bank"
            value={accountId}
            onValueChange={(v) =>
              void navigate({
                search: (s) => ({ ...s, accountId: v, page: 1 }),
              })
            }
            options={bankOptions(banks.data)}
            placeholder="Pilih rekening"
          />
        </Field>
        <Field className="w-44">
          <FieldLabel htmlFor="mut-from">Dari</FieldLabel>
          <DateInput
            id="mut-from"
            value={dateFrom}
            onValueChange={(v) =>
              void navigate({ search: (s) => ({ ...s, dateFrom: v, page: 1 }) })
            }
          />
        </Field>
        <Field className="w-44">
          <FieldLabel htmlFor="mut-to">Sampai</FieldLabel>
          <DateInput
            id="mut-to"
            value={dateTo}
            onValueChange={(v) =>
              void navigate({ search: (s) => ({ ...s, dateTo: v, page: 1 }) })
            }
          />
        </Field>
      </div>
      {mutations.data ? (
        <p className="text-sm text-muted-foreground">
          Saldo awal {formatDate(dateFrom)}:{" "}
          <span className="font-medium text-foreground">
            {formatRupiah(mutations.data.openingBalance)}
          </span>
        </p>
      ) : null}
      <DataTable
        columns={columns}
        rows={accountId ? mutations.data?.items : []}
        getRowId={(m) => m.journalLineId}
        loading={!!accountId && mutations.isPending}
        fetching={mutations.isFetching}
        page={search.page}
        totalPages={mutations.data?.totalPages ?? 0}
        totalItems={mutations.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage={
          accountId
            ? "Tidak ada mutasi pada rentang tanggal ini."
            : "Pilih rekening."
        }
      />
    </div>
  )
}
