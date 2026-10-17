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
import { UserPicker } from "@/features/approval/components/user-picker"
import type { ApprovalUserOption } from "@/features/approval/api/approval-api"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  cancelPettyCash,
  cashBankAccountsQuery,
  createPettyCash,
  createPettyCashFund,
  expenseClaimOptionsQuery,
  pettyCashAction,
  pettyCashFundsQuery,
  pettyCashTransactionsQuery,
  postPettyCash,
  updatePettyCashFund,
  type PettyCashFund,
  type PettyCashTransaction,
  type PettyCashType,
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

const route = getRouteApi("/_app/finance/petty-cash")

/** Keuangan & Akuntansi › Kas Kecil: dana imprest, pengeluaran (A-17), pengisian kembali (A-16) (S-405). */
export function PettyCashPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const funds = useQuery(pettyCashFundsQuery(true))
  const txns = useQuery(
    pettyCashTransactionsQuery({ page: search.page, fundId: search.fundId })
  )
  const [fundForm, setFundForm] = useState<PettyCashFund | "new" | null>(null)
  const [txnForm, setTxnForm] = useState<PettyCashType | null>(null)
  const [posting, setPosting] = useState<PettyCashTransaction | null>(null)
  const [confirm, setConfirm] = useState<{
    t: PettyCashTransaction
    kind: "submit" | "revise"
  } | null>(null)
  const [cancelling, setCancelling] = useState<PettyCashTransaction | null>(
    null
  )
  const refresh = useFinanceRefresh()

  const action = useIdempotentMutation({
    mutationFn: (
      c: { t: PettyCashTransaction; kind: "submit" | "revise" },
      key
    ) => pettyCashAction(c.t.id, c.kind, c.t.version, key),
    onSuccess: async (_, c) => {
      setConfirm(null)
      toast.success(
        c.kind === "submit"
          ? "Pengisian kembali diajukan ke Head F&A"
          : "Kembali menjadi draft"
      )
      await refresh()
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })
  const cancel = useIdempotentMutation({
    mutationFn: (
      { t, reason }: { t: PettyCashTransaction; reason: string },
      key
    ) => cancelPettyCash(t.id, t.version, reason, key),
    onSuccess: async () => {
      setCancelling(null)
      toast.success("Transaksi kas kecil dibatalkan")
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const fundColumns: DataTableColumn<PettyCashFund>[] = [
    {
      id: "fund",
      header: "Dana",
      cell: (f) => (
        <button
          type="button"
          className="flex flex-col text-left"
          onClick={() =>
            void navigate({
              search: (s) => ({
                ...s,
                fundId: s.fundId === f.id ? undefined : f.id,
                page: 1,
              }),
            })
          }
        >
          <span className="font-medium text-primary">{f.code}</span>
          <span className="text-xs text-muted-foreground">{f.name}</span>
        </button>
      ),
    },
    { id: "holder", header: "Pemegang", cell: (f) => f.holderName || "—" },
    { id: "division", header: "Divisi", cell: (f) => f.divisionName },
    {
      id: "ceiling",
      header: "Plafon",
      align: "right",
      cell: (f) => formatRupiah(f.ceilingAmount),
    },
    {
      id: "balance",
      header: "Saldo",
      align: "right",
      cell: (f) => (
        <span className="font-medium">{formatRupiah(f.balanceAmount)}</span>
      ),
    },
    {
      id: "shortfall",
      header: "Perlu Diisi",
      align: "right",
      cell: (f) => formatRupiah(f.shortfallAmount),
    },
    {
      id: "status",
      header: "",
      align: "right",
      cell: (f) => (
        <div className="flex justify-end gap-2">
          {!f.isActive ? <Badge variant="outline">Nonaktif</Badge> : null}
          {search.fundId === f.id ? (
            <Badge variant="secondary">Dipilih</Badge>
          ) : null}
          {can("accounting.petty_cash.write") ? (
            <Button size="sm" variant="outline" onClick={() => setFundForm(f)}>
              Ubah
            </Button>
          ) : null}
        </div>
      ),
    },
  ]

  const txnColumns: DataTableColumn<PettyCashTransaction>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (t) => (
        <span className="font-medium whitespace-nowrap">
          {t.number ?? "Draft"}
        </span>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (t) => (
        <span className="whitespace-nowrap">{formatDate(t.businessDate)}</span>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (t) => (
        <Badge variant={t.type === "expense" ? "outline" : "secondary"}>
          {t.type === "expense" ? "Pengeluaran" : "Pengisian"}
        </Badge>
      ),
    },
    {
      id: "desc",
      header: "Keterangan",
      cell: (t) => (
        <div className="flex max-w-xs flex-col">
          <span className="truncate">
            {t.description || t.expenseTypeName || t.fundName}
          </span>
          <span className="text-xs text-muted-foreground">
            {t.fundCode}
            {t.expenseTypeName ? ` · ${t.expenseTypeName}` : ""}
            {t.bankAccountName ? ` · dari ${t.bankAccountName}` : ""}
            {t.receiptFileName ? " · bukti terlampir" : ""}
          </span>
        </div>
      ),
    },
    {
      id: "amount",
      header: "Nominal",
      align: "right",
      cell: (t) => formatRupiah(t.amount),
    },
    {
      id: "status",
      header: "Status",
      cell: (t) => <StatusBadge status={t.status} />,
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (t) => (
        <div className="flex justify-end gap-2">
          {t.allowedActions.includes("submit") ? (
            <Button size="sm" onClick={() => setConfirm({ t, kind: "submit" })}>
              Ajukan
            </Button>
          ) : null}
          {t.allowedActions.includes("revise") ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setConfirm({ t, kind: "revise" })}
            >
              Revisi
            </Button>
          ) : null}
          {t.allowedActions.includes("post") ? (
            <Button size="sm" onClick={() => setPosting(t)}>
              Posting
            </Button>
          ) : null}
          {t.allowedActions.includes("cancel") ? (
            <Button size="sm" variant="ghost" onClick={() => setCancelling(t)}>
              Batalkan
            </Button>
          ) : null}
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Kas Kecil</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Sistem imprest: pengeluaran dengan bukti maksimal{" "}
            {formatRupiah(funds.data?.maxPerTransaction ?? "0")} per transaksi;
            pengisian kembali disetujui Head Finance & Accounting.
          </p>
        </div>
        {can("accounting.petty_cash.write") ? (
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={() => setFundForm("new")}>
              <PlusIcon /> Tambah Dana
            </Button>
            <Button
              variant="outline"
              onClick={() => setTxnForm("replenishment")}
            >
              Isi Kembali
            </Button>
            <Button onClick={() => setTxnForm("expense")}>
              <PlusIcon /> Catat Pengeluaran
            </Button>
          </div>
        ) : null}
      </div>
      <DataTable
        caption="Dana kas kecil"
        columns={fundColumns}
        rows={funds.data?.items}
        getRowId={(f) => f.id}
        loading={funds.isPending}
        page={1}
        totalPages={1}
        totalItems={funds.data?.items.length ?? 0}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="Belum ada dana kas kecil."
      />
      <h2 className="text-lg font-semibold">Transaksi</h2>
      <DataTable
        columns={txnColumns}
        rows={txns.data?.items}
        getRowId={(t) => t.id}
        loading={txns.isPending}
        fetching={txns.isFetching}
        page={search.page}
        totalPages={txns.data?.totalPages ?? 0}
        totalItems={txns.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada transaksi kas kecil."
      />
      <Dialog
        open={fundForm !== null}
        onOpenChange={(o) => !o && setFundForm(null)}
      >
        {fundForm !== null ? (
          <FundForm
            value={fundForm === "new" ? undefined : fundForm}
            defaultCeiling={funds.data?.defaultCeiling ?? ""}
            onClose={() => setFundForm(null)}
          />
        ) : null}
      </Dialog>
      <Dialog
        open={txnForm !== null}
        onOpenChange={(o) => !o && setTxnForm(null)}
      >
        {txnForm !== null ? (
          <TxnForm
            type={txnForm}
            funds={funds.data?.items ?? []}
            defaultFund={search.fundId}
            onClose={() => setTxnForm(null)}
          />
        ) : null}
      </Dialog>
      <Dialog open={!!posting} onOpenChange={(o) => !o && setPosting(null)}>
        {posting ? (
          <PostForm txn={posting} onClose={() => setPosting(null)} />
        ) : null}
      </Dialog>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={
          confirm?.kind === "submit"
            ? "Ajukan Pengisian Kembali"
            : "Revisi Pengisian"
        }
        description={
          confirm?.kind === "submit"
            ? `Pengisian ${formatRupiah(confirm.t.amount)} untuk ${confirm.t.fundCode} diajukan ke Head Finance & Accounting.`
            : "Pengisian yang ditolak dikembalikan menjadi draft."
        }
        confirmLabel={confirm?.kind === "submit" ? "Ajukan" : "Revisi"}
        loading={action.isPending}
        onConfirm={() => confirm && action.mutate(confirm)}
      />
      <ReasonDialog
        open={!!cancelling}
        onOpenChange={(o) => !o && setCancelling(null)}
        title="Batalkan Transaksi Kas Kecil"
        description="Transaksi yang belum diposting dibatalkan."
        confirmLabel="Batalkan Transaksi"
        loading={cancel.isPending}
        onConfirm={(reason) =>
          cancelling && cancel.mutate({ t: cancelling, reason })
        }
      />
    </div>
  )
}

function FundForm({
  value,
  defaultCeiling,
  onClose,
}: {
  value?: PettyCashFund
  defaultCeiling: string
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const options = useQuery(expenseClaimOptionsQuery)
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [holder, setHolder] = useState<ApprovalUserOption | null>(
    value
      ? { id: value.holderUserId, fullName: value.holderName, email: "" }
      : null
  )
  const [divisionId, setDivisionId] = useState(value?.divisionId ?? "")
  const [ceilingAmount, setCeilingAmount] = useState(
    value?.ceilingAmount ?? defaultCeiling
  )
  const [isActive, setIsActive] = useState(value?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        name,
        holderUserId: holder?.id ?? "",
        divisionId,
        ceilingAmount,
        isActive,
      }
      return value
        ? updatePettyCashFund(
            value.id,
            { ...body, version: value.version },
            key
          )
        : createPettyCashFund({ ...body, code }, key)
    },
    onSuccess: async () => {
      toast.success(
        value ? "Dana kas kecil diperbarui" : "Dana kas kecil ditambahkan"
      )
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
          {value ? "Ubah Dana Kas Kecil" : "Tambah Dana Kas Kecil"}
        </DialogTitle>
        <DialogDescription>
          Dana baru bersaldo nol; isi lewat Isi Kembali sampai plafon.
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
            <FieldLabel htmlFor="fd-code">Kode</FieldLabel>
            <Input
              id="fd-code"
              value={code}
              disabled={!!value}
              placeholder="KK-HO"
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <FieldError errors={fieldErrors(errors.code)} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="fd-name">Nama</FieldLabel>
            <Input
              id="fd-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={fieldErrors(errors.name)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.holderUserId}>
          <FieldLabel htmlFor="fd-holder">Pemegang Kas Kecil</FieldLabel>
          <UserPicker
            id="fd-holder"
            value={holder}
            onValueChange={setHolder}
            aria-invalid={!!errors.holderUserId}
          />
          <FieldError errors={fieldErrors(errors.holderUserId)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.divisionId}>
            <FieldLabel htmlFor="fd-div">Divisi</FieldLabel>
            <SimpleSelect
              id="fd-div"
              value={divisionId}
              onValueChange={setDivisionId}
              placeholder="Pilih divisi"
              options={(options.data?.divisions ?? []).map((d) => ({
                value: d.id,
                label: d.name,
              }))}
              aria-invalid={!!errors.divisionId}
            />
            <FieldError errors={fieldErrors(errors.divisionId)} />
          </Field>
          <Field data-invalid={!!errors.ceilingAmount}>
            <FieldLabel htmlFor="fd-ceiling">Plafon</FieldLabel>
            <MoneyInput
              id="fd-ceiling"
              value={ceilingAmount}
              onValueChange={setCeilingAmount}
            />
            <FieldError errors={fieldErrors(errors.ceilingAmount)} />
          </Field>
        </div>
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
            Simpan Dana
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

function TxnForm({
  type,
  funds,
  defaultFund,
  onClose,
}: {
  type: PettyCashType
  funds: PettyCashFund[]
  defaultFund?: string
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const options = useQuery(expenseClaimOptionsQuery)
  const active = funds.filter((f) => f.isActive)
  const [fundId, setFundId] = useState(defaultFund ?? active[0]?.id ?? "")
  const fund = funds.find((f) => f.id === fundId)
  const [businessDate, setBusinessDate] = useState(todayBusinessDate())
  const [expenseTypeId, setExpenseTypeId] = useState("")
  const [campaignId, setCampaignId] = useState("")
  const [description, setDescription] = useState("")
  const [amount, setAmount] = useState("")
  const [receiptFileId, setReceiptFileId] = useState<string>()
  const [errors, setErrors] = useState<Record<string, string>>({})
  const expense = type === "expense"
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createPettyCash(
        {
          type,
          fundId,
          businessDate,
          expenseTypeId: expense ? expenseTypeId || undefined : undefined,
          campaignId: campaignId || undefined,
          description,
          amount: amount || undefined,
          receiptFileId,
        },
        key
      ),
    onSuccess: async () => {
      toast.success(
        expense
          ? "Draft pengeluaran disimpan; posting untuk mengurangi saldo"
          : "Draft pengisian disimpan; ajukan untuk persetujuan"
      )
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
          {expense ? "Catat Pengeluaran Kas Kecil" : "Isi Kembali Kas Kecil"}
        </DialogTitle>
        <DialogDescription>
          {expense
            ? "Bukti wajib dilampirkan. Jurnal A-17 (Dr beban / Cr 1102 Kas Kecil) dibuat saat diposting."
            : `Nominal kosong = selisih plafon dan saldo${fund ? ` (${formatRupiah(fund.shortfallAmount)})` : ""}. Jurnal A-16 setelah disetujui.`}
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
          <Field data-invalid={!!errors.fundId}>
            <FieldLabel htmlFor="tx-fund">Dana</FieldLabel>
            <SimpleSelect
              id="tx-fund"
              value={fundId}
              onValueChange={setFundId}
              placeholder="Pilih dana"
              options={active.map((f) => ({
                value: f.id,
                label: `${f.code} · saldo ${formatRupiah(f.balanceAmount)}`,
              }))}
              aria-invalid={!!errors.fundId}
            />
            <FieldError errors={fieldErrors(errors.fundId)} />
          </Field>
          <Field data-invalid={!!errors.businessDate}>
            <FieldLabel htmlFor="tx-date">Tanggal</FieldLabel>
            <DateInput
              id="tx-date"
              value={businessDate}
              onValueChange={setBusinessDate}
            />
            <FieldError errors={fieldErrors(errors.businessDate)} />
          </Field>
        </div>
        {expense ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.expenseTypeId}>
              <FieldLabel htmlFor="tx-type">Jenis Biaya</FieldLabel>
              <SimpleSelect
                id="tx-type"
                value={expenseTypeId}
                onValueChange={setExpenseTypeId}
                placeholder="Pilih jenis biaya"
                options={(options.data?.expenseTypes ?? []).map((t) => ({
                  value: t.id,
                  label: t.name,
                }))}
                aria-invalid={!!errors.expenseTypeId}
              />
              <FieldError errors={fieldErrors(errors.expenseTypeId)} />
            </Field>
            <Field data-invalid={!!errors.campaignId}>
              <FieldLabel htmlFor="tx-camp">Kampanye</FieldLabel>
              <SimpleSelect
                id="tx-camp"
                value={campaignId}
                onValueChange={setCampaignId}
                placeholder="Tanpa kampanye"
                options={[
                  { value: "", label: "Tanpa kampanye" },
                  ...(options.data?.campaigns ?? []).map((c) => ({
                    value: c.id,
                    label: `${c.code} ${c.name}`,
                  })),
                ]}
              />
              <FieldError errors={fieldErrors(errors.campaignId)} />
            </Field>
          </div>
        ) : null}
        <Field data-invalid={!!errors.amount}>
          <FieldLabel htmlFor="tx-amount">Nominal</FieldLabel>
          <MoneyInput
            id="tx-amount"
            value={amount}
            onValueChange={setAmount}
            aria-invalid={!!errors.amount}
          />
          <FieldError errors={fieldErrors(errors.amount)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="tx-desc">Keterangan</FieldLabel>
          <Input
            id="tx-desc"
            value={description}
            maxLength={300}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        {expense ? (
          <Field data-invalid={!!errors.receiptFileId}>
            <FieldLabel>Bukti</FieldLabel>
            <ReceiptUpload
              ownerType="petty_cash_transaction"
              onUploaded={setReceiptFileId}
              onRemoved={() => setReceiptFileId(undefined)}
            />
            <FieldError errors={fieldErrors(errors.receiptFileId)} />
          </Field>
        ) : null}
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

function PostForm({
  txn,
  onClose,
}: {
  txn: PettyCashTransaction
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const banks = useQuery(cashBankAccountsQuery())
  const [bankAccountId, setBankAccountId] = useState("")
  const [error, setError] = useState<string>()
  const replenish = txn.type === "replenishment"
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      postPettyCash(
        txn.id,
        txn.version,
        replenish ? bankAccountId : undefined,
        key
      ),
    onSuccess: async (t) => {
      toast.success(`${t.number ?? "Transaksi"} diposting`)
      onClose()
      await refresh()
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.fields) {
        setError(Object.values(p.fields).join(" · "))
        return
      }
      showErrorDialog(p)
    },
  })
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>
          {replenish ? "Posting Pengisian Kembali" : "Posting Pengeluaran"}
        </DialogTitle>
        <DialogDescription>
          {replenish
            ? `Transfer ${formatRupiah(txn.amount)} dari rekening ke ${txn.fundCode} (A-16). Anda akan diminta konfirmasi ulang password/kode.`
            : `Pengeluaran ${formatRupiah(txn.amount)} dari ${txn.fundCode} dijurnal (A-17) dan mengurangi saldo.`}
        </DialogDescription>
      </DialogHeader>
      {replenish ? (
        <Field data-invalid={!!error}>
          <FieldLabel htmlFor="pc-bank">Rekening Sumber</FieldLabel>
          <SimpleSelect
            id="pc-bank"
            value={bankAccountId}
            onValueChange={setBankAccountId}
            options={bankOptions(banks.data)}
            placeholder="Pilih rekening"
          />
        </Field>
      ) : null}
      {error ? <p className="text-sm text-destructive">{error}</p> : null}
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton
          loading={save.isPending}
          loadingText="Memposting…"
          onClick={() => save.mutate()}
        >
          Posting
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
