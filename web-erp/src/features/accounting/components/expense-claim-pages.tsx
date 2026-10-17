import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
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
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { openFile } from "@/features/party/api/party-api"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  cancelExpenseClaim,
  cashBankAccountsQuery,
  claimAction,
  createExpenseClaim,
  expenseClaimOptionsQuery,
  expenseClaimQuery,
  expenseClaimsQuery,
  payExpenseClaim,
  previewClaimVerification,
  updateExpenseClaim,
  verifyExpenseClaim,
  WITHHOLDING_LABELS,
  type ExpenseClaimDetail,
  type ExpenseClaimOptions,
  type ExpenseClaimStatus,
  type ExpenseClaimSummary,
  type ExpenseClaimVerifyPreview,
  type WithholdingType,
} from "../api/finance-api"
import {
  bankOptions,
  fieldErrors,
  Info,
  ReasonDialog,
  ReceiptUpload,
  SimpleSelect,
  useFinanceRefresh,
} from "./finance-shared"

const listRoute = getRouteApi("/_app/finance/expense-claims/")

const STATUS_OPTIONS: { value: ExpenseClaimStatus | "all"; label: string }[] = [
  { value: "all", label: "Semua status" },
  { value: "draft", label: "Draft" },
  { value: "pending_approval", label: "Menunggu Persetujuan" },
  { value: "approved", label: "Disetujui" },
  { value: "rejected", label: "Ditolak" },
  { value: "awaiting_payment", label: "Menunggu Pembayaran" },
  { value: "paid", label: "Lunas" },
  { value: "cancelled", label: "Dibatalkan" },
]

/** Keuangan & Akuntansi › Biaya Operasional: pengajuan biaya/reimbursement semua divisi (S-404). */
export function ExpenseClaimListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const claims = useQuery(
    expenseClaimsQuery({
      page: search.page,
      q: search.q,
      status: search.status,
    })
  )

  const columns: DataTableColumn<ExpenseClaimSummary>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (c) => (
        <Link
          to="/finance/expense-claims/$id"
          params={{ id: c.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {c.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (c) => (
        <span className="whitespace-nowrap">{formatDate(c.claimDate)}</span>
      ),
    },
    {
      id: "payee",
      header: "Penerima",
      cell: (c) => (
        <div className="flex max-w-xs flex-col">
          <span className="truncate">{c.payeeName}</span>
          <span className="text-xs text-muted-foreground">
            {c.divisionName} · diajukan {c.requesterName}
          </span>
        </div>
      ),
    },
    {
      id: "desc",
      header: "Keterangan",
      cell: (c) => (
        <span className="line-clamp-2 max-w-xs">{c.description || "—"}</span>
      ),
    },
    {
      id: "total",
      header: "Total",
      align: "right",
      cell: (c) => formatRupiah(c.totalAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (c) => <StatusBadge status={c.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Biaya Operasional
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Pengajuan biaya dan reimbursement dengan bukti, persetujuan
            berjenjang per nominal, verifikasi Finance & Accounting (termasuk
            PPh), lalu pembayaran.
          </p>
        </div>
        {can("accounting.expense_claim.write") ? (
          <Button render={<Link to="/finance/expense-claims/new" />}>
            <PlusIcon /> Ajukan Biaya
          </Button>
        ) : null}
      </div>
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
            aria-label="Cari pengajuan"
            placeholder="Cari nomor, penerima, keterangan"
            defaultValue={search.q}
            className="w-64"
          />
        </form>
        <SimpleSelect
          className="w-52"
          aria-label="Filter status"
          value={search.status ?? "all"}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                status: v === "all" ? undefined : (v as ExpenseClaimStatus),
                page: 1,
              }),
            })
          }
          placeholder="Semua status"
          options={STATUS_OPTIONS}
        />
      </div>
      <DataTable
        columns={columns}
        rows={claims.data?.items}
        getRowId={(c) => c.id}
        loading={claims.isPending}
        fetching={claims.isFetching}
        page={search.page}
        totalPages={claims.data?.totalPages ?? 0}
        totalItems={claims.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada pengajuan biaya."
      />
    </div>
  )
}

// --- Form ---------------------------------------------------------------------------------------

type LineDraft = {
  key: string
  expenseTypeId: string
  description: string
  amount: string
  receiptFileId?: string
  receiptName?: string
}

/** Form pengajuan biaya baru atau ubah draft. */
export function ExpenseClaimFormPage({ id }: { id?: string }) {
  const claim = useQuery({ ...expenseClaimQuery(id ?? ""), enabled: !!id })
  const options = useQuery(expenseClaimOptionsQuery)
  if ((id && !claim.data) || !options.data) {
    return <LoadingArea loading>{null}</LoadingArea>
  }
  return <ClaimForm value={claim.data} options={options.data} />
}

function ClaimForm({
  value,
  options,
}: {
  value?: ExpenseClaimDetail
  options: ExpenseClaimOptions
}) {
  const navigate = useNavigate()
  const refresh = useFinanceRefresh()
  const [claimType, setClaimType] = useState(value?.claimType ?? "expense")
  const [divisionId, setDivisionId] = useState(value?.divisionId ?? "")
  const [campaignId, setCampaignId] = useState(value?.campaignId ?? "")
  const [payeeName, setPayeeName] = useState(value?.payeeName ?? "")
  const [claimDate, setClaimDate] = useState(
    value?.claimDate ?? todayBusinessDate()
  )
  const [description, setDescription] = useState(value?.description ?? "")
  const [lines, setLines] = useState<LineDraft[]>(
    value?.lines.map((l) => ({
      key: l.id,
      expenseTypeId: l.expenseTypeId,
      description: l.description,
      amount: l.amount,
      receiptFileId: l.receiptFileId,
      receiptName: l.receiptFileName,
    })) ?? [
      {
        key: crypto.randomUUID(),
        expenseTypeId: "",
        description: "",
        amount: "",
      },
    ]
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const reimbursement = claimType === "reimbursement"
  const total = lines.reduce((sum, l) => sum + (Number(l.amount) || 0), 0)
  const typeOptions = options.expenseTypes.map((t) => ({
    value: t.id,
    label: `${t.name} (${t.code})`,
  }))
  const needsCampaign = lines.some(
    (l) =>
      options.expenseTypes.find((t) => t.id === l.expenseTypeId)
        ?.requiresCampaign
  )

  const setLine = (key: string, patch: Partial<LineDraft>) =>
    setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        claimType,
        divisionId,
        campaignId: campaignId || undefined,
        payeeType: reimbursement ? ("employee" as const) : ("other" as const),
        payeeName: reimbursement ? undefined : payeeName,
        claimDate,
        description,
        lines: lines.map((l) => ({
          expenseTypeId: l.expenseTypeId,
          description: l.description,
          amount: l.amount,
          receiptFileId: l.receiptFileId,
        })),
      }
      return value
        ? updateExpenseClaim(value.id, { ...body, version: value.version }, key)
        : createExpenseClaim(body, key)
    },
    onSuccess: async (saved) => {
      toast.success("Draft pengajuan disimpan")
      await refresh()
      void navigate({
        to: "/finance/expense-claims/$id",
        params: { id: saved.id },
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
    <form
      noValidate
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {value ? `Ubah ${value.number ?? "Draft Pengajuan"}` : "Ajukan Biaya"}
        </h1>
        <p className="text-sm text-muted-foreground">
          Lampirkan bukti tiap baris. Setelah disimpan, ajukan untuk persetujuan
          Head divisi (dan Head F&A/Direksi sesuai nominal).
        </p>
      </div>
      <div className="grid gap-4 rounded-lg border p-4 sm:grid-cols-2 lg:grid-cols-3">
        <Field>
          <FieldLabel htmlFor="ec-type">Jenis Pengajuan</FieldLabel>
          <SimpleSelect
            id="ec-type"
            value={claimType}
            onValueChange={(v) =>
              setClaimType(v as "expense" | "reimbursement")
            }
            placeholder="Pilih jenis"
            options={[
              { value: "expense", label: "Biaya (dibayar ke pihak lain)" },
              {
                value: "reimbursement",
                label: "Reimbursement (dibayar ke saya)",
              },
            ]}
          />
        </Field>
        <Field data-invalid={!!errors.divisionId}>
          <FieldLabel htmlFor="ec-div">Divisi</FieldLabel>
          <SimpleSelect
            id="ec-div"
            value={divisionId}
            onValueChange={setDivisionId}
            placeholder="Pilih divisi"
            options={options.divisions.map((d) => ({
              value: d.id,
              label: d.name,
            }))}
            aria-invalid={!!errors.divisionId}
          />
          <FieldError errors={fieldErrors(errors.divisionId)} />
        </Field>
        <Field data-invalid={!!errors.claimDate}>
          <FieldLabel htmlFor="ec-date">Tanggal Biaya</FieldLabel>
          <DateInput
            id="ec-date"
            value={claimDate}
            onValueChange={setClaimDate}
          />
          <FieldError errors={fieldErrors(errors.claimDate)} />
        </Field>
        {!reimbursement ? (
          <Field data-invalid={!!errors.payeeName}>
            <FieldLabel htmlFor="ec-payee">Penerima Pembayaran</FieldLabel>
            <Input
              id="ec-payee"
              value={payeeName}
              onChange={(e) => setPayeeName(e.target.value)}
              placeholder="Nama vendor/talent"
            />
            <FieldError errors={fieldErrors(errors.payeeName)} />
          </Field>
        ) : null}
        <Field data-invalid={!!errors.campaignId}>
          <FieldLabel htmlFor="ec-camp">
            Kampanye{needsCampaign ? "" : " (opsional)"}
          </FieldLabel>
          <SimpleSelect
            id="ec-camp"
            value={campaignId}
            onValueChange={setCampaignId}
            placeholder="Tanpa kampanye"
            options={[
              { value: "", label: "Tanpa kampanye" },
              ...options.campaigns.map((c) => ({
                value: c.id,
                label: `${c.code} ${c.name}`,
              })),
            ]}
            aria-invalid={!!errors.campaignId}
          />
          <FieldError errors={fieldErrors(errors.campaignId)} />
        </Field>
        <Field className="sm:col-span-2 lg:col-span-3">
          <FieldLabel htmlFor="ec-desc">Keterangan</FieldLabel>
          <Textarea
            id="ec-desc"
            value={description}
            maxLength={500}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
      </div>
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="min-w-56">Jenis Biaya</TableHead>
              <TableHead className="min-w-48">Uraian</TableHead>
              <TableHead className="min-w-40 text-right">Nominal</TableHead>
              <TableHead className="min-w-48">Bukti</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {lines.map((l, i) => (
              <TableRow key={l.key}>
                <TableCell>
                  <SimpleSelect
                    aria-label={`Jenis biaya baris ${i + 1}`}
                    value={l.expenseTypeId}
                    onValueChange={(v) => setLine(l.key, { expenseTypeId: v })}
                    placeholder="Pilih jenis biaya"
                    options={typeOptions}
                    aria-invalid={!!errors[`lines.${i}.expenseTypeId`]}
                  />
                  <LineError message={errors[`lines.${i}.expenseTypeId`]} />
                </TableCell>
                <TableCell>
                  <Input
                    aria-label={`Uraian baris ${i + 1}`}
                    value={l.description}
                    onChange={(e) =>
                      setLine(l.key, { description: e.target.value })
                    }
                  />
                </TableCell>
                <TableCell>
                  <MoneyInput
                    aria-label={`Nominal baris ${i + 1}`}
                    value={l.amount}
                    onValueChange={(v) => setLine(l.key, { amount: v })}
                  />
                  <LineError message={errors[`lines.${i}.amount`]} />
                </TableCell>
                <TableCell>
                  {l.receiptFileId && l.receiptName ? (
                    <span className="text-sm">{l.receiptName}</span>
                  ) : (
                    <ReceiptUpload
                      ownerType="expense_claim"
                      onUploaded={(fileId) =>
                        setLine(l.key, { receiptFileId: fileId })
                      }
                      onRemoved={() =>
                        setLine(l.key, { receiptFileId: undefined })
                      }
                    />
                  )}
                  <LineError message={errors[`lines.${i}.receiptFileId`]} />
                </TableCell>
                <TableCell>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Hapus baris ${i + 1}`}
                    disabled={lines.length === 1}
                    onClick={() =>
                      setLines((ls) => ls.filter((x) => x.key !== l.key))
                    }
                  >
                    <Trash2Icon />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell colSpan={2} className="font-medium">
                Total
              </TableCell>
              <TableCell className="text-right font-medium">
                {formatRupiah(String(total))}
              </TableCell>
              <TableCell colSpan={2} />
            </TableRow>
          </TableFooter>
        </Table>
      </div>
      <LineError message={errors.lines} />
      <div className="flex flex-wrap justify-between gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            setLines((ls) => [
              ...ls,
              {
                key: crypto.randomUUID(),
                expenseTypeId: "",
                description: "",
                amount: "",
              },
            ])
          }
        >
          <PlusIcon /> Tambah Baris
        </Button>
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
      </div>
    </form>
  )
}

function LineError({ message }: { message?: string }) {
  return message ? (
    <p className="mt-1 text-xs text-destructive">{message}</p>
  ) : null
}

// --- Detail -------------------------------------------------------------------------------------

/** Detail pengajuan biaya dengan aksi sesuai status (ajukan, revisi, batal, verifikasi F&A, bayar). */
export function ExpenseClaimDetailPage({ id }: { id: string }) {
  const claim = useQuery(expenseClaimQuery(id))
  return (
    <LoadingArea loading={claim.isPending}>
      {claim.data ? <ClaimDetail claim={claim.data} /> : null}
    </LoadingArea>
  )
}

function ClaimDetail({ claim }: { claim: ExpenseClaimDetail }) {
  const refresh = useFinanceRefresh()
  const [confirm, setConfirm] = useState<"submit" | "revise" | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const [verifying, setVerifying] = useState(false)
  const [paying, setPaying] = useState(false)
  const can = (a: ExpenseClaimDetail["allowedActions"][number]) =>
    claim.allowedActions.includes(a)

  const action = useIdempotentMutation({
    mutationFn: (kind: "submit" | "revise", key) =>
      claimAction(claim.id, kind, claim.version, key),
    onSuccess: async (_, kind) => {
      setConfirm(null)
      toast.success(
        kind === "submit"
          ? "Pengajuan diajukan untuk persetujuan"
          : "Pengajuan kembali menjadi draft"
      )
      await refresh()
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })
  const cancel = useIdempotentMutation({
    mutationFn: (reason: string, key) =>
      cancelExpenseClaim(claim.id, claim.version, reason, key),
    onSuccess: async () => {
      setCancelling(false)
      toast.success("Pengajuan dibatalkan")
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {claim.number ?? "Draft Pengajuan Biaya"}
            </h1>
            <StatusBadge status={claim.status} />
          </div>
          <p className="text-sm text-muted-foreground">{claim.description}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/finance/expense-claims/$id/edit"
                  params={{ id: claim.id }}
                />
              }
            >
              Ubah
            </Button>
          ) : null}
          {can("cancel") ? (
            <Button variant="ghost" onClick={() => setCancelling(true)}>
              Batalkan
            </Button>
          ) : null}
          {can("revise") ? (
            <Button variant="outline" onClick={() => setConfirm("revise")}>
              Revisi
            </Button>
          ) : null}
          {can("submit") ? (
            <Button onClick={() => setConfirm("submit")}>Ajukan</Button>
          ) : null}
          {can("verify") ? (
            <Button onClick={() => setVerifying(true)}>Verifikasi</Button>
          ) : null}
          {can("pay") ? (
            <Button onClick={() => setPaying(true)}>Bayar</Button>
          ) : null}
        </div>
      </div>
      <dl className="grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <Info
          label="Jenis"
          value={
            claim.claimType === "reimbursement" ? "Reimbursement" : "Biaya"
          }
        />
        <Info label="Penerima" value={claim.payeeName} />
        <Info label="Divisi" value={claim.divisionName} />
        <Info label="Kampanye" value={claim.campaignName ?? "—"} />
        <Info label="Tanggal biaya" value={formatDate(claim.claimDate)} />
        <Info label="Diajukan oleh" value={claim.requesterName} />
        <Info
          label="Diajukan"
          value={claim.submittedAt ? formatDateTime(claim.submittedAt) : "—"}
        />
        <Info
          label="Keputusan"
          value={
            claim.decisionNote ||
            (claim.decidedAt ? formatDateTime(claim.decidedAt) : "—")
          }
        />
        <Info label="Total" value={formatRupiah(claim.totalAmount)} />
        <Info
          label="PPh dipotong"
          value={formatRupiah(claim.withholdingAmount)}
        />
        <Info label="Dibayar" value={formatRupiah(claim.payableAmount)} />
        <Info
          label="Jurnal / Pembayaran"
          value={
            [claim.journalNumber, claim.paymentNumber]
              .filter(Boolean)
              .join(" · ") || "—"
          }
        />
        {claim.cancelReason ? (
          <Info label="Alasan batal" value={claim.cancelReason} />
        ) : null}
      </dl>
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Jenis Biaya</TableHead>
              <TableHead>Uraian</TableHead>
              <TableHead>Bukti</TableHead>
              <TableHead>PPh</TableHead>
              <TableHead className="text-right">Potongan</TableHead>
              <TableHead className="text-right">Nominal</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {claim.lines.map((l) => (
              <TableRow key={l.id}>
                <TableCell>
                  {l.expenseTypeName}
                  <span className="block text-xs text-muted-foreground">
                    {l.expenseTypeCode} · akun {l.expenseAccountCode}
                  </span>
                </TableCell>
                <TableCell>{l.description || "—"}</TableCell>
                <TableCell>
                  {l.receiptFileId ? (
                    <Button
                      variant="link"
                      className="h-auto p-0"
                      onClick={() => void openFile(l.receiptFileId ?? "")}
                    >
                      {l.receiptFileName ?? "Lihat bukti"}
                    </Button>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
                <TableCell>
                  {l.withholdingType === "none"
                    ? "—"
                    : `${WITHHOLDING_LABELS[l.withholdingType]} ${l.withholdingRate}%`}
                </TableCell>
                <TableCell className="text-right">
                  {formatRupiah(l.withholdingAmount)}
                </TableCell>
                <TableCell className="text-right">
                  {formatRupiah(l.amount)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={
          confirm === "submit" ? "Ajukan Pengajuan Biaya" : "Revisi Pengajuan"
        }
        description={
          confirm === "submit"
            ? `Pengajuan ${formatRupiah(claim.totalAmount)} diberi nomor EXP dan dikirim ke approver sesuai nominal dan divisi.`
            : "Pengajuan yang ditolak dikembalikan menjadi draft agar bisa diperbaiki."
        }
        confirmLabel={confirm === "submit" ? "Ajukan" : "Revisi"}
        loading={action.isPending}
        onConfirm={() => confirm && action.mutate(confirm)}
      />
      <ReasonDialog
        open={cancelling}
        onOpenChange={setCancelling}
        title="Batalkan Pengajuan Biaya"
        description="Pengajuan yang dibatalkan tidak bisa diajukan kembali."
        confirmLabel="Batalkan Pengajuan"
        loading={cancel.isPending}
        onConfirm={(reason) => cancel.mutate(reason)}
      />
      <Dialog open={verifying} onOpenChange={setVerifying}>
        {verifying ? (
          <VerifyForm claim={claim} onClose={() => setVerifying(false)} />
        ) : null}
      </Dialog>
      <Dialog open={paying} onOpenChange={setPaying}>
        {paying ? (
          <PayForm claim={claim} onClose={() => setPaying(false)} />
        ) : null}
      </Dialog>
    </div>
  )
}

const WHT_OPTIONS: { value: WithholdingType; label: string }[] = [
  { value: "none", label: "Tidak dipotong" },
  { value: "pph21", label: "PPh 21 (bukan pegawai)" },
  { value: "pph23", label: "PPh 23" },
  { value: "pph4_2", label: "PPh 4(2)" },
]

function VerifyForm({
  claim,
  onClose,
}: {
  claim: ExpenseClaimDetail
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const options = useQuery(expenseClaimOptionsQuery)
  const defaultType = (expenseTypeId: string): WithholdingType =>
    options.data?.expenseTypes.find((t) => t.id === expenseTypeId)
      ?.withholdingType ?? "none"
  const [taxId, setTaxId] = useState("")
  const [choices, setChoices] = useState<Record<string, WithholdingType>>({})
  const [preview, setPreview] = useState<ExpenseClaimVerifyPreview | null>(null)
  const body = () => ({
    version: claim.version,
    payeeTaxId: taxId || undefined,
    lines: claim.lines.map((l) => ({
      lineId: l.id,
      withholdingType: choices[l.id] ?? defaultType(l.expenseTypeId),
    })),
  })
  const calc = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      previewClaimVerification(claim.id, body(), key),
    onSuccess: (p) => setPreview(p),
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => verifyExpenseClaim(claim.id, body(), key),
    onSuccess: async () => {
      toast.success("Pengajuan diverifikasi dan dijurnal (A-15)")
      onClose()
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <DialogContent className="sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Verifikasi Finance & Accounting</DialogTitle>
        <DialogDescription>
          Periksa bukti tiap baris dan tentukan PPh yang dipotong. PPh dihitung
          dengan tarif yang berlaku hari ini; jurnal A-15 dibuat saat verifikasi
          disimpan.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-4">
        <Field>
          <FieldLabel htmlFor="vf-tax">NPWP/NIK Penerima</FieldLabel>
          <Input
            id="vf-tax"
            inputMode="numeric"
            value={taxId}
            placeholder={
              claim.payeeTaxIdMasked ??
              "Kosongkan bila tidak ada (tarif PPh lebih tinggi)"
            }
            onChange={(e) => {
              setTaxId(e.target.value)
              setPreview(null)
            }}
          />
        </Field>
        <ul className="flex flex-col divide-y rounded-md border">
          {claim.lines.map((l) => (
            <li
              key={l.id}
              className="flex flex-wrap items-center justify-between gap-3 p-3 text-sm"
            >
              <div className="flex min-w-0 flex-col">
                <span className="font-medium">{l.expenseTypeName}</span>
                <span className="text-xs text-muted-foreground">
                  {formatRupiah(l.amount)} ·{" "}
                  {l.receiptFileId ? "bukti terlampir" : "tanpa bukti"}
                </span>
              </div>
              <SimpleSelect
                className="w-52"
                aria-label={`PPh ${l.expenseTypeName}`}
                value={choices[l.id] ?? defaultType(l.expenseTypeId)}
                onValueChange={(v) => {
                  setChoices((c) => ({ ...c, [l.id]: v as WithholdingType }))
                  setPreview(null)
                }}
                placeholder="Jenis PPh"
                options={WHT_OPTIONS}
              />
            </li>
          ))}
        </ul>
        {preview ? (
          <div className="rounded-md border bg-muted/40 p-3 text-sm">
            {preview.withholdings.map((w) => (
              <p key={`${w.taxType}-${w.objectCode}`}>
                {w.objectName} ({w.objectCode}): {formatRupiah(w.grossAmount)} ×{" "}
                {w.ratePercent}% ={" "}
                <span className="font-medium">{formatRupiah(w.taxAmount)}</span>
              </p>
            ))}
            <p className="mt-2 font-medium">
              Total PPh {formatRupiah(preview.totalWithholding)} · Dibayar{" "}
              {formatRupiah(preview.payableAmount)}
            </p>
          </div>
        ) : null}
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton
          variant="outline"
          loading={calc.isPending}
          onClick={() => calc.mutate()}
        >
          Hitung PPh
        </LoadingButton>
        <LoadingButton
          loading={save.isPending}
          loadingText="Memverifikasi…"
          onClick={() => save.mutate()}
        >
          Verifikasi & Jurnal
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

function PayForm({
  claim,
  onClose,
}: {
  claim: ExpenseClaimDetail
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const banks = useQuery(cashBankAccountsQuery())
  const [bankAccountId, setBankAccountId] = useState("")
  const [paymentDate, setPaymentDate] = useState(todayBusinessDate())
  const [reference, setReference] = useState("")
  const [proofFileId, setProofFileId] = useState<string>()
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      payExpenseClaim(
        claim.id,
        {
          version: claim.version,
          bankAccountId,
          paymentDate,
          reference,
          proofFileId,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Pengajuan dibayar")
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
        <DialogTitle>Bayar Pengajuan Biaya</DialogTitle>
        <DialogDescription>
          Pembayaran keluar {formatRupiah(claim.payableAmount)} ke{" "}
          {claim.payeeName} (Dr 2103 Utang Biaya / Cr rekening). Anda akan
          diminta konfirmasi ulang password/kode.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!bankAccountId) {
            setErrors({ bankAccountId: "Rekening wajib dipilih" })
            return
          }
          save.mutate()
        }}
      >
        <Field data-invalid={!!errors.bankAccountId}>
          <FieldLabel htmlFor="py-bank">Dibayar dari</FieldLabel>
          <SimpleSelect
            id="py-bank"
            value={bankAccountId}
            onValueChange={setBankAccountId}
            options={bankOptions(banks.data)}
            placeholder="Pilih rekening"
            aria-invalid={!!errors.bankAccountId}
          />
          <FieldError errors={fieldErrors(errors.bankAccountId)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.paymentDate}>
            <FieldLabel htmlFor="py-date">Tanggal Bayar</FieldLabel>
            <DateInput
              id="py-date"
              value={paymentDate}
              onValueChange={setPaymentDate}
            />
            <FieldError errors={fieldErrors(errors.paymentDate)} />
          </Field>
          <Field>
            <FieldLabel htmlFor="py-ref">Referensi Transfer</FieldLabel>
            <Input
              id="py-ref"
              value={reference}
              maxLength={100}
              onChange={(e) => setReference(e.target.value)}
            />
          </Field>
        </div>
        <Field>
          <FieldLabel>Bukti Transfer</FieldLabel>
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
            loadingText="Membayar…"
          >
            Bayar
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
