import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { DownloadIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { PageTabs } from "@/components/common/page-tabs"
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
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  cashBankAccountsQuery,
  createFinalTaxSetting,
  createTaxDeposit,
  createTaxRate,
  exportWithholdingRecap,
  finalTaxQuery,
  MONTH_LABELS,
  periodLabel,
  runFinalTax,
  setSlipNumber,
  TAX_TYPE_LABELS,
  taxDepositsQuery,
  taxRatesQuery,
  withholdingSlipsQuery,
  withholdingSummaryQuery,
  type TaxDeposit,
  type TaxRate,
  type TaxType,
  type WithholdingSlip,
} from "../api/finance-api"
import { Checkbox } from "./account-form-dialog"
import {
  bankOptions,
  fieldErrors,
  FinanceStatusBadge,
  SimpleSelect,
  useFinanceRefresh,
  yearOptions,
} from "./finance-shared"

const route = getRouteApi("/_app/finance/taxes")

type Tab = "slips" | "deposits" | "final" | "rates"

const monthOptions = MONTH_LABELS.map((m, i) => ({
  value: String(i + 1),
  label: m,
}))
const taxOptions = (Object.keys(TAX_TYPE_LABELS) as TaxType[]).map((t) => ({
  value: t,
  label: TAX_TYPE_LABELS[t],
}))

/** Keuangan & Akuntansi › Pajak & Bukti Potong: PPh 21/23/4(2), rekap Coretax, setor, PPh Final (S-406, S-407). */
export function TaxesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const now = new Date()
  const year = search.year ?? now.getFullYear()
  const month = search.month ?? now.getMonth() + 1
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Pajak & Bukti Potong
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          PPh yang dipotong dari pengajuan biaya dan tagihan pemasok, rekap
          bukti potong per masa untuk diinput ke Coretax, penyetoran pajak, dan
          PPh Final UMKM.
        </p>
      </div>
      <PageTabs<Tab>
        label="Bagian pajak"
        tabs={[
          { id: "slips", label: "Bukti Potong" },
          { id: "deposits", label: "Penyetoran" },
          { id: "final", label: "PPh Final UMKM" },
          { id: "rates", label: "Tarif" },
        ]}
        value={search.tab}
        onValueChange={(t) =>
          void navigate({ search: (s) => ({ ...s, tab: t }) })
        }
      />
      <div className="flex flex-wrap gap-3">
        <SimpleSelect
          className="w-32"
          aria-label="Tahun"
          value={String(year)}
          onValueChange={(v) =>
            void navigate({ search: (s) => ({ ...s, year: Number(v) }) })
          }
          options={yearOptions(now.getFullYear())}
          placeholder="Tahun"
        />
        {search.tab === "slips" ? (
          <SimpleSelect
            className="w-40"
            aria-label="Masa"
            value={String(month)}
            onValueChange={(v) =>
              void navigate({ search: (s) => ({ ...s, month: Number(v) }) })
            }
            options={monthOptions}
            placeholder="Masa"
          />
        ) : null}
      </div>
      {search.tab === "slips" ? <SlipsTab year={year} month={month} /> : null}
      {search.tab === "deposits" ? <DepositsTab year={year} /> : null}
      {search.tab === "final" ? <FinalTaxTab year={year} /> : null}
      {search.tab === "rates" ? <RatesTab /> : null}
    </div>
  )
}

// --- Bukti potong -----------------------------------------------------------------------------

function SlipsTab({ year, month }: { year: number; month: number }) {
  const { can } = usePermissions()
  const [taxType, setTaxType] = useState<TaxType | "">("")
  const slips = useQuery(
    withholdingSlipsQuery({
      year,
      month,
      taxType: taxType || undefined,
      page: 1,
    })
  )
  const summary = useQuery(withholdingSummaryQuery(year))
  const [editing, setEditing] = useState<WithholdingSlip | null>(null)
  const exportRecap = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      exportWithholdingRecap(
        { year, month, taxType: taxType || undefined },
        key
      ),
    onSuccess: (link) => {
      window.open(link.url, "_blank", "noopener")
      toast.success("Rekap bukti potong diunduh")
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const monthSummary = (summary.data ?? []).filter((r) => r.taxMonth === month)
  const columns: DataTableColumn<WithholdingSlip>[] = [
    { id: "type", header: "Jenis", cell: (w) => TAX_TYPE_LABELS[w.taxType] },
    {
      id: "object",
      header: "Objek Pajak",
      cell: (w) => (
        <div className="flex flex-col">
          <span>{w.objectCode}</span>
          <span className="text-xs text-muted-foreground">{w.objectName}</span>
        </div>
      ),
    },
    {
      id: "payee",
      header: "Penerima",
      cell: (w) => (
        <div className="flex flex-col">
          <span>{w.taxpayerName}</span>
          <span className="text-xs text-muted-foreground">
            {w.hasTaxId
              ? `NPWP/NIK ${w.taxpayerTaxIdMasked}`
              : "Tanpa NPWP/NIK"}
          </span>
        </div>
      ),
    },
    {
      id: "doc",
      header: "Dokumen",
      cell: (w) => (
        <div className="flex flex-col">
          <span>{w.sourceLabel}</span>
          <span className="text-xs text-muted-foreground">
            {formatDate(w.businessDate)}
          </span>
        </div>
      ),
    },
    {
      id: "gross",
      header: "Bruto",
      align: "right",
      cell: (w) => formatRupiah(w.grossAmount),
    },
    {
      id: "rate",
      header: "Tarif",
      align: "right",
      cell: (w) => `${w.ratePercent}%`,
    },
    {
      id: "tax",
      header: "PPh",
      align: "right",
      cell: (w) => (
        <span className="font-medium">{formatRupiah(w.taxAmount)}</span>
      ),
    },
    {
      id: "slip",
      header: "No. Bukti Potong",
      cell: (w) =>
        can("accounting.withholding.write") && w.status !== "cancelled" ? (
          <Button
            variant="link"
            className="h-auto p-0"
            onClick={() => setEditing(w)}
          >
            {w.slipNumber || "Isi nomor"}
          </Button>
        ) : (
          w.slipNumber || "—"
        ),
    },
    {
      id: "status",
      header: "Status",
      cell: (w) => <FinanceStatusBadge status={w.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <SimpleSelect
          className="w-48"
          aria-label="Jenis PPh"
          value={taxType}
          onValueChange={(v) => setTaxType(v as TaxType | "")}
          placeholder="Semua jenis PPh"
          options={[
            { value: "", label: "Semua jenis PPh" },
            ...taxOptions.filter((t) => t.value !== "final_umkm"),
          ]}
        />
        {can("accounting.withholding.export") ? (
          <LoadingButton
            variant="outline"
            loading={exportRecap.isPending}
            loadingText="Menyiapkan…"
            onClick={() => exportRecap.mutate()}
          >
            <DownloadIcon /> Ekspor Rekap Coretax
          </LoadingButton>
        ) : null}
      </div>
      {monthSummary.length > 0 ? (
        <div className="flex flex-wrap gap-2 text-sm">
          {monthSummary.map((r) => (
            <Badge key={r.taxType} variant="outline" className="h-auto py-1">
              {TAX_TYPE_LABELS[r.taxType]}: {r.slipCount} bukti · belum disetor{" "}
              {formatRupiah(r.openAmount)} · disetor{" "}
              {formatRupiah(r.depositedAmount)}
            </Badge>
          ))}
        </div>
      ) : null}
      <DataTable
        caption={`Bukti potong masa ${periodLabel(year, month)}`}
        columns={columns}
        rows={slips.data?.items}
        getRowId={(w) => w.id ?? `${w.sourceId}-${w.objectCode}`}
        loading={slips.isPending}
        fetching={slips.isFetching}
        page={1}
        totalPages={slips.data?.totalPages ?? 0}
        totalItems={slips.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="Belum ada PPh yang dipotong pada masa ini."
      />
      <Dialog open={!!editing} onOpenChange={(o) => !o && setEditing(null)}>
        {editing ? (
          <SlipNumberForm slip={editing} onClose={() => setEditing(null)} />
        ) : null}
      </Dialog>
    </div>
  )
}

function SlipNumberForm({
  slip,
  onClose,
}: {
  slip: WithholdingSlip
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const [value, setValue] = useState(slip.slipNumber)
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      setSlipNumber(slip.id ?? "", slip.version, value, key),
    onSuccess: async () => {
      toast.success("Nomor bukti potong disimpan")
      onClose()
      await refresh()
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Nomor Bukti Potong</DialogTitle>
        <DialogDescription>
          {slip.taxpayerName} · {slip.sourceLabel} ·{" "}
          {formatRupiah(slip.taxAmount)}. Isi nomor dari Coretax setelah bukti
          potong diterbitkan.
        </DialogDescription>
      </DialogHeader>
      <Input
        aria-label="Nomor bukti potong"
        value={value}
        maxLength={50}
        onChange={(e) => setValue(e.target.value)}
      />
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton loading={save.isPending} onClick={() => save.mutate()}>
          Simpan
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

// --- Penyetoran -------------------------------------------------------------------------------

function DepositsTab({ year }: { year: number }) {
  const { canAny } = usePermissions()
  const deposits = useQuery(taxDepositsQuery(year))
  const [creating, setCreating] = useState(false)
  const columns: DataTableColumn<TaxDeposit>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (d) => <span className="font-medium">{d.number}</span>,
    },
    { id: "type", header: "Jenis", cell: (d) => TAX_TYPE_LABELS[d.taxType] },
    {
      id: "period",
      header: "Masa",
      cell: (d) => periodLabel(d.taxYear, d.taxMonth),
    },
    {
      id: "date",
      header: "Tanggal Setor",
      cell: (d) => formatDate(d.depositDate),
    },
    { id: "bank", header: "Rekening", cell: (d) => d.bankAccountName },
    { id: "ntpn", header: "NTPN", cell: (d) => d.ntpn || "—" },
    {
      id: "amount",
      header: "Nominal",
      align: "right",
      cell: (d) => formatRupiah(d.amount),
    },
    { id: "journal", header: "Jurnal", cell: (d) => d.journalNumber },
  ]
  return (
    <div className="flex flex-col gap-3">
      {canAny("accounting.withholding.post", "accounting.final_tax.post") ? (
        <div className="flex justify-end">
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Setor Pajak
          </Button>
        </div>
      ) : null}
      <DataTable
        columns={columns}
        rows={deposits.data}
        getRowId={(d) => d.id}
        loading={deposits.isPending}
        page={1}
        totalPages={1}
        totalItems={deposits.data?.length ?? 0}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="Belum ada penyetoran pajak tahun ini."
      />
      <Dialog open={creating} onOpenChange={setCreating}>
        {creating ? (
          <DepositForm year={year} onClose={() => setCreating(false)} />
        ) : null}
      </Dialog>
    </div>
  )
}

function DepositForm({ year, onClose }: { year: number; onClose: () => void }) {
  const refresh = useFinanceRefresh()
  const banks = useQuery(cashBankAccountsQuery())
  const prev = new Date()
  prev.setMonth(prev.getMonth() - 1)
  const [taxType, setTaxType] = useState<TaxType>("pph23")
  const [taxMonth, setTaxMonth] = useState(String(prev.getMonth() + 1))
  const [bankAccountId, setBankAccountId] = useState("")
  const [depositDate, setDepositDate] = useState(todayBusinessDate())
  const [billingCode, setBillingCode] = useState("")
  const [ntpn, setNtpn] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createTaxDeposit(
        {
          taxType,
          taxYear: year,
          taxMonth: Number(taxMonth),
          bankAccountId,
          depositDate,
          billingCode,
          ntpn,
        },
        key
      ),
    onSuccess: async (d) => {
      toast.success(`Setoran ${formatRupiah(d.amount)} dicatat (${d.number})`)
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
        <DialogTitle>Setor Pajak</DialogTitle>
        <DialogDescription>
          Nominal dihitung otomatis dari PPh yang belum disetor pada masa
          tersebut (atau PPh Final yang sudah dihitung). Jurnal A-29: Dr utang
          PPh / Cr rekening. Anda akan diminta konfirmasi ulang password/kode.
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
          <Field data-invalid={!!errors.taxType}>
            <FieldLabel htmlFor="dp-type">Jenis Pajak</FieldLabel>
            <SimpleSelect
              id="dp-type"
              value={taxType}
              onValueChange={(v) => setTaxType(v as TaxType)}
              options={taxOptions}
              placeholder="Jenis"
            />
          </Field>
          <Field data-invalid={!!errors.period}>
            <FieldLabel htmlFor="dp-month">Masa {year}</FieldLabel>
            <SimpleSelect
              id="dp-month"
              value={taxMonth}
              onValueChange={setTaxMonth}
              options={monthOptions}
              placeholder="Masa"
            />
            <FieldError errors={fieldErrors(errors.period)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.bankAccountId}>
          <FieldLabel htmlFor="dp-bank">Rekening</FieldLabel>
          <SimpleSelect
            id="dp-bank"
            value={bankAccountId}
            onValueChange={setBankAccountId}
            options={bankOptions(banks.data)}
            placeholder="Pilih rekening"
          />
          <FieldError errors={fieldErrors(errors.bankAccountId)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field data-invalid={!!errors.depositDate}>
            <FieldLabel htmlFor="dp-date">Tanggal Setor</FieldLabel>
            <DateInput
              id="dp-date"
              value={depositDate}
              onValueChange={setDepositDate}
            />
            <FieldError errors={fieldErrors(errors.depositDate)} />
          </Field>
          <Field>
            <FieldLabel htmlFor="dp-billing">Kode Billing</FieldLabel>
            <Input
              id="dp-billing"
              value={billingCode}
              maxLength={30}
              onChange={(e) => setBillingCode(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="dp-ntpn">NTPN</FieldLabel>
            <Input
              id="dp-ntpn"
              value={ntpn}
              maxLength={30}
              onChange={(e) => setNtpn(e.target.value)}
            />
          </Field>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Setor Pajak
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

// --- PPh Final UMKM ---------------------------------------------------------------------------

const THRESHOLD_LABELS = {
  below: "Di bawah batas",
  approaching: "Mendekati batas Rp4,8 M",
  exceeded: "Melewati batas Rp4,8 M",
} as const

function FinalTaxTab({ year }: { year: number }) {
  const { can } = usePermissions()
  const summary = useQuery(finalTaxQuery(year))
  const refresh = useFinanceRefresh()
  const [setting, setSetting] = useState(false)
  const [running, setRunning] = useState<number | null>(null)
  const run = useIdempotentMutation({
    mutationFn: (month: number, key) => runFinalTax(year, month, key),
    onSuccess: async (r) => {
      setRunning(null)
      toast.success(
        `PPh Final ${periodLabel(r.taxYear, r.taxMonth)}: ${formatRupiah(r.taxAmount)}`
      )
      await refresh()
    },
    onError: (err) => {
      setRunning(null)
      showErrorDialog(parseProblem(err))
    },
  })
  const s = summary.data
  const thisMonth =
    new Date().getFullYear() === year ? new Date().getMonth() + 1 : 13
  return (
    <div className="flex flex-col gap-3">
      {s ? (
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="rounded-lg border p-4 text-sm">
            <p className="text-muted-foreground">Status</p>
            <p className="text-lg font-semibold">
              {s.enabled ? "Aktif" : "Tidak aktif"}
            </p>
            <p className="text-xs text-muted-foreground">
              Tarif {s.ratePercent ?? "—"}% × omzet bruto bulanan
            </p>
          </div>
          <div className="rounded-lg border p-4 text-sm">
            <p className="text-muted-foreground">Omzet kumulatif {year}</p>
            <p className="text-lg font-semibold">
              {formatRupiah(s.threshold.cumulative)}
            </p>
            <p className="text-xs text-muted-foreground">
              {s.threshold.usedPercent}% dari {formatRupiah(s.threshold.limit)}{" "}
              · sisa {formatRupiah(s.threshold.remaining)}
            </p>
          </div>
          <div
            className={`rounded-lg border p-4 text-sm ${s.threshold.status === "below" ? "" : "border-amber-500 bg-amber-500/10"}`}
            role={s.threshold.status === "below" ? undefined : "alert"}
          >
            <p className="text-muted-foreground">Pemantauan batas</p>
            <p className="text-lg font-semibold">
              {THRESHOLD_LABELS[s.threshold.status]}
            </p>
            <p className="text-xs text-muted-foreground">
              Omzet awal (cut-over) {formatRupiah(s.openingTurnover)}
            </p>
          </div>
        </div>
      ) : null}
      {can("accounting.final_tax.write") ? (
        <div className="flex justify-end">
          <Button variant="outline" onClick={() => setSetting(true)}>
            Ubah Status PPh Final
          </Button>
        </div>
      ) : null}
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-left">
            <tr>
              <th className="p-3">Masa</th>
              <th className="p-3 text-right">Omzet Bruto</th>
              <th className="p-3 text-right">Kumulatif</th>
              <th className="p-3 text-right">PPh Final</th>
              <th className="p-3">Jurnal</th>
              <th className="p-3" />
            </tr>
          </thead>
          <tbody>
            {(s?.months ?? []).map((m) => (
              <tr key={m.taxMonth} className="border-t">
                <td className="p-3">{MONTH_LABELS[m.taxMonth - 1]}</td>
                <td className="p-3 text-right tabular-nums">
                  {formatRupiah(m.grossRevenue)}
                </td>
                <td className="p-3 text-right tabular-nums">
                  {formatRupiah(m.cumulative)}
                </td>
                <td className="p-3 text-right tabular-nums">
                  {m.run ? formatRupiah(m.run.taxAmount) : "—"}
                </td>
                <td className="p-3">
                  {m.run ? (
                    <span className="flex flex-wrap gap-1">
                      {m.run.journalNumber ?? "Tanpa jurnal (nol)"}
                      {m.run.depositId ? (
                        <Badge variant="secondary">Disetor</Badge>
                      ) : null}
                    </span>
                  ) : (
                    "—"
                  )}
                </td>
                <td className="p-3 text-right">
                  {!m.run &&
                  s?.enabled &&
                  m.taxMonth < thisMonth &&
                  can("accounting.final_tax.post") ? (
                    <Button size="sm" onClick={() => setRunning(m.taxMonth)}>
                      Hitung
                    </Button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <ConfirmDialog
        open={running !== null}
        onOpenChange={(o) => !o && setRunning(null)}
        title="Hitung PPh Final UMKM"
        description={
          running
            ? `PPh Final masa ${periodLabel(year, running)} dihitung dari omzet bruto dan dijurnal (A-30: Dr 7203 / Cr 2304) per akhir bulan.`
            : ""
        }
        confirmLabel="Hitung PPh Final"
        loading={run.isPending}
        onConfirm={() => running && run.mutate(running)}
      />
      <Dialog open={setting} onOpenChange={setSetting}>
        {setting ? (
          <FinalSettingForm
            enabled={!!s?.enabled}
            onClose={() => setSetting(false)}
          />
        ) : null}
      </Dialog>
    </div>
  )
}

function FinalSettingForm({
  enabled,
  onClose,
}: {
  enabled: boolean
  onClose: () => void
}) {
  const refresh = useFinanceRefresh()
  const [isEnabled, setIsEnabled] = useState(!enabled)
  const [validFrom, setValidFrom] = useState(todayBusinessDate())
  const [openingTurnover, setOpeningTurnover] = useState("")
  const [note, setNote] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createFinalTaxSetting(
        {
          validFrom,
          isEnabled,
          openingYear: openingTurnover
            ? Number(validFrom.slice(0, 4))
            : undefined,
          openingTurnover: openingTurnover || undefined,
          note,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Status PPh Final disimpan")
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
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Status PPh Final UMKM</DialogTitle>
        <DialogDescription>
          Berlaku mulai tanggal yang dipilih (riwayat tersimpan). Omzet awal
          diisi saat cut-over agar batas Rp4,8 M dipantau sejak awal tahun.
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-4">
        <Checkbox
          label="PPh Final UMKM berlaku"
          checked={isEnabled}
          onChange={setIsEnabled}
        />
        <Field data-invalid={!!errors.validFrom}>
          <FieldLabel htmlFor="ft-from">Berlaku Mulai</FieldLabel>
          <DateInput
            id="ft-from"
            value={validFrom}
            onValueChange={setValidFrom}
          />
          <FieldError errors={fieldErrors(errors.validFrom)} />
        </Field>
        <Field data-invalid={!!errors.openingTurnover}>
          <FieldLabel htmlFor="ft-open">
            Omzet Bruto Kumulatif Sebelum Cut-over
          </FieldLabel>
          <MoneyInput
            id="ft-open"
            value={openingTurnover}
            onValueChange={setOpeningTurnover}
          />
          <FieldError errors={fieldErrors(errors.openingTurnover)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="ft-note">Catatan</FieldLabel>
          <Input
            id="ft-note"
            value={note}
            maxLength={300}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton loading={save.isPending} onClick={() => save.mutate()}>
          Simpan
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}

// --- Tarif ------------------------------------------------------------------------------------

function RatesTab() {
  const { canAny } = usePermissions()
  const rates = useQuery(taxRatesQuery())
  const [creating, setCreating] = useState<TaxRate | "new" | null>(null)
  const columns: DataTableColumn<TaxRate>[] = [
    { id: "type", header: "Jenis", cell: (r) => TAX_TYPE_LABELS[r.taxType] },
    {
      id: "object",
      header: "Objek Pajak",
      cell: (r) => (
        <div className="flex flex-col">
          <span>
            {r.objectCode}{" "}
            {r.isDefault ? <Badge variant="secondary">Default</Badge> : null}
          </span>
          <span className="text-xs text-muted-foreground">{r.name}</span>
        </div>
      ),
    },
    {
      id: "rate",
      header: "Tarif",
      cell: (r) =>
        r.brackets.length > 0
          ? `Progresif Pasal 17 (${r.brackets.map((b) => `${b.ratePercent}%`).join("/")}) × ${r.basePercent}% bruto`
          : `${r.ratePercent}%${r.basePercent !== "100" ? ` × ${r.basePercent}% bruto` : ""}`,
    },
    {
      id: "surcharge",
      header: "Tanpa NPWP",
      cell: (r) =>
        r.noTaxIdSurchargePercent !== "0"
          ? `+${r.noTaxIdSurchargePercent}%`
          : "—",
    },
    {
      id: "valid",
      header: "Berlaku",
      cell: (r) =>
        `${formatDate(r.validFrom)} – ${r.validTo ? formatDate(r.validTo) : "sekarang"}`,
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (r) =>
        !r.validTo &&
        canAny("accounting.withholding.write", "accounting.final_tax.write") ? (
          <Button size="sm" variant="outline" onClick={() => setCreating(r)}>
            Versi Baru
          </Button>
        ) : null,
    },
  ]
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">
        Tarif effective-dated: perubahan dibuat sebagai versi baru yang berlaku
        mulai tanggal tertentu, versi lama ditutup otomatis. Nilai awal adalah
        usulan dan wajib dikonfirmasi konsultan pajak.
      </p>
      {canAny("accounting.withholding.write", "accounting.final_tax.write") ? (
        <div className="flex justify-end">
          <Button onClick={() => setCreating("new")}>
            <PlusIcon /> Tambah Objek Pajak
          </Button>
        </div>
      ) : null}
      <DataTable
        columns={columns}
        rows={rates.data}
        getRowId={(r) => r.id}
        loading={rates.isPending}
        page={1}
        totalPages={1}
        totalItems={rates.data?.length ?? 0}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="Belum ada tarif."
      />
      <Dialog
        open={creating !== null}
        onOpenChange={(o) => !o && setCreating(null)}
      >
        {creating !== null ? (
          <RateForm
            base={creating === "new" ? undefined : creating}
            onClose={() => setCreating(null)}
          />
        ) : null}
      </Dialog>
    </div>
  )
}

function RateForm({ base, onClose }: { base?: TaxRate; onClose: () => void }) {
  const refresh = useFinanceRefresh()
  const [taxType, setTaxType] = useState<TaxType>(base?.taxType ?? "pph23")
  const [objectCode, setObjectCode] = useState(base?.objectCode ?? "")
  const [name, setName] = useState(base?.name ?? "")
  const [ratePercent, setRatePercent] = useState(base?.ratePercent ?? "")
  const [basePercent, setBasePercent] = useState(base?.basePercent ?? "100")
  const [surcharge, setSurcharge] = useState(
    base?.noTaxIdSurchargePercent ?? "0"
  )
  const [isDefault, setIsDefault] = useState(base?.isDefault ?? false)
  const [validFrom, setValidFrom] = useState(todayBusinessDate())
  const [note, setNote] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createTaxRate(
        {
          taxType,
          objectCode,
          name,
          ratePercent,
          brackets: base?.brackets,
          basePercent,
          noTaxIdSurchargePercent: surcharge,
          isDefault,
          validFrom,
          note,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Versi tarif disimpan")
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
  const progressive = (base?.brackets.length ?? 0) > 0
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {base ? "Versi Tarif Baru" : "Tambah Objek Pajak"}
        </DialogTitle>
        <DialogDescription>
          {progressive
            ? "Lapisan tarif progresif Pasal 17 disalin dari versi sebelumnya."
            : "Persentase maks. 4 angka di belakang koma."}
        </DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <Field>
            <FieldLabel htmlFor="rt-type">Jenis</FieldLabel>
            <SimpleSelect
              id="rt-type"
              value={taxType}
              onValueChange={(v) => setTaxType(v as TaxType)}
              options={taxOptions}
              placeholder="Jenis"
            />
          </Field>
          <Field data-invalid={!!errors.objectCode}>
            <FieldLabel htmlFor="rt-obj">Kode Objek Pajak</FieldLabel>
            <Input
              id="rt-obj"
              value={objectCode}
              disabled={!!base}
              onChange={(e) => setObjectCode(e.target.value)}
            />
            <FieldError errors={fieldErrors(errors.objectCode)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.name}>
          <FieldLabel htmlFor="rt-name">Nama</FieldLabel>
          <Input
            id="rt-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <FieldError errors={fieldErrors(errors.name)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-3">
          {!progressive ? (
            <Field data-invalid={!!errors.ratePercent}>
              <FieldLabel htmlFor="rt-rate">Tarif (%)</FieldLabel>
              <Input
                id="rt-rate"
                inputMode="decimal"
                value={ratePercent}
                onChange={(e) => setRatePercent(e.target.value)}
              />
              <FieldError errors={fieldErrors(errors.ratePercent)} />
            </Field>
          ) : null}
          <Field data-invalid={!!errors.basePercent}>
            <FieldLabel htmlFor="rt-base">DPP (% bruto)</FieldLabel>
            <Input
              id="rt-base"
              inputMode="decimal"
              value={basePercent}
              onChange={(e) => setBasePercent(e.target.value)}
            />
            <FieldError errors={fieldErrors(errors.basePercent)} />
          </Field>
          <Field data-invalid={!!errors.noTaxIdSurchargePercent}>
            <FieldLabel htmlFor="rt-sur">Kenaikan tanpa NPWP (%)</FieldLabel>
            <Input
              id="rt-sur"
              inputMode="decimal"
              value={surcharge}
              onChange={(e) => setSurcharge(e.target.value)}
            />
            <FieldError errors={fieldErrors(errors.noTaxIdSurchargePercent)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.validFrom}>
          <FieldLabel htmlFor="rt-from">Berlaku Mulai</FieldLabel>
          <DateInput
            id="rt-from"
            value={validFrom}
            onValueChange={setValidFrom}
          />
          <FieldError errors={fieldErrors(errors.validFrom)} />
        </Field>
        <Field>
          <FieldLabel htmlFor="rt-note">Catatan</FieldLabel>
          <Input
            id="rt-note"
            value={note}
            maxLength={300}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
        <Checkbox
          label="Objek default untuk jenis pajak ini"
          checked={isDefault}
          onChange={setIsDefault}
        />
      </div>
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Batal
        </Button>
        <LoadingButton loading={save.isPending} onClick={() => save.mutate()}>
          Simpan Tarif
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
