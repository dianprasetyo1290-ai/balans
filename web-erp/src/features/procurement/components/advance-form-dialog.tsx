import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  advanceableOrdersQuery,
  cashBankAccountsQuery,
  createAdvance,
  updateAdvance,
  type PurchaseAdvance,
} from "../api/procurement-api"
import { fieldErrors, OptionSelect } from "./shared"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** PO yang sudah dipilih (dari detail PO). */
  order?: { id: string; number: string; supplierName: string }
  /** Draft uang muka yang diubah. */
  advance?: PurchaseAdvance
}

/** Dialog Catat Uang Muka: draft uang muka pembelian per PO yang disetujui (S-205). */
export function AdvanceFormDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <AdvanceForm {...props} /> : null}
    </Dialog>
  )
}

function AdvanceForm({ onOpenChange, order, advance }: Props) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const accounts = useQuery(cashBankAccountsQuery)
  const preset = advance
    ? {
        id: advance.purchaseOrderId,
        label: advance.purchaseOrderNumber,
        description: advance.supplierName,
      }
    : order
      ? { id: order.id, label: order.number, description: order.supplierName }
      : null
  const [po, setPo] = useState<PickerOption | null>(preset)
  const [amount, setAmount] = useState(advance?.amount ?? "")
  const [paymentDate, setPaymentDate] = useState(
    advance?.paymentDate ?? todayBusinessDate()
  )
  const [bankAccountCode, setBankAccountCode] = useState(
    advance?.bankAccountCode ?? ""
  )
  const [note, setNote] = useState(advance?.note ?? "")
  const [errors, setErrors] = useState<Record<string, string>>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const fields = { amount, paymentDate, bankAccountCode, note }
      return advance
        ? updateAdvance(
            advance.id,
            { ...fields, version: advance.version },
            key
          )
        : createAdvance({ ...fields, purchaseOrderId: po?.id ?? "" }, key)
    },
    onSuccess: async (saved) => {
      toast.success("Draft uang muka disimpan")
      onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      void navigate({
        to: "/procurement/purchase-advances/$id",
        params: { id: saved.id },
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

  const submit = () => {
    const next: Record<string, string> = {}
    if (!po) next.purchaseOrderId = "Purchase Order wajib dipilih"
    if (!amount || Number(amount) <= 0)
      next.amount = "Nominal harus lebih dari nol"
    if (!paymentDate) next.paymentDate = "Tanggal bayar wajib diisi"
    if (!bankAccountCode) next.bankAccountCode = "Akun Kas & Bank wajib dipilih"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {advance ? "Ubah Uang Muka" : "Catat Uang Muka"}
        </DialogTitle>
        <DialogDescription>
          Uang muka disimpan sebagai draft lalu diposting Finance & Accounting
          (jurnal A-19). Sisa uang muka dipotong otomatis saat tagihan pemasok
          PO ini dicatat.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <Field data-invalid={!!errors.purchaseOrderId}>
          <FieldLabel htmlFor="adv-po">Purchase Order</FieldLabel>
          <SearchPicker
            id="adv-po"
            value={po}
            disabled={!!preset}
            onValueChange={setPo}
            query={advanceableOrdersQuery}
            toOption={(o) => ({
              id: o.id,
              label: o.number ?? "Draft",
              description: `${o.supplierName} · ${formatRupiah(o.totalAmount)}`,
            })}
            placeholder="Pilih PO yang disetujui"
            searchPlaceholder="Cari nomor PO atau pemasok"
            aria-invalid={!!errors.purchaseOrderId}
          />
          <FieldError errors={fieldErrors(errors.purchaseOrderId)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.amount}>
            <FieldLabel htmlFor="adv-amount">Nominal</FieldLabel>
            <MoneyInput
              id="adv-amount"
              value={amount}
              onValueChange={setAmount}
              aria-invalid={!!errors.amount}
            />
            <FieldError errors={fieldErrors(errors.amount)} />
          </Field>
          <Field data-invalid={!!errors.paymentDate}>
            <FieldLabel htmlFor="adv-date">Tanggal Bayar</FieldLabel>
            <DateInput
              id="adv-date"
              value={paymentDate}
              onValueChange={setPaymentDate}
              aria-invalid={!!errors.paymentDate}
            />
            <FieldError errors={fieldErrors(errors.paymentDate)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.bankAccountCode}>
          <FieldLabel htmlFor="adv-bank">Dibayar dari (Kas & Bank)</FieldLabel>
          <OptionSelect
            id="adv-bank"
            value={bankAccountCode}
            onValueChange={setBankAccountCode}
            options={accounts.data ?? []}
            placeholder="Pilih akun"
            showCode
            aria-invalid={!!errors.bankAccountCode}
          />
          <FieldError errors={fieldErrors(errors.bankAccountCode)} />
        </Field>
        <Field data-invalid={!!errors.note}>
          <FieldLabel htmlFor="adv-note">Catatan</FieldLabel>
          <Textarea
            id="adv-note"
            value={note}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
          />
          <FieldError errors={fieldErrors(errors.note)} />
        </Field>
        <DialogFooter>
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
            Simpan Draft
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
