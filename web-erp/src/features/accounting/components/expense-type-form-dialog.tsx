import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { LoadingButton } from "@/components/common/loading-button"
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
  FieldDescription,
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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  accountsQuery,
  createExpenseType,
  EXPENSE_APPROVAL_LABELS,
  isExpenseAccount,
  updateExpenseType,
  WITHHOLDING_LABELS,
  type ExpenseApprovalSubject,
  type ExpenseType,
  type WithholdingType,
} from "../api/accounting-api"
import { Checkbox } from "./account-form-dialog"

type ExpenseTypeFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = jenis biaya baru. */
  value?: ExpenseType
}

/** Tambah/ubah jenis biaya (S-124). Kode tidak bisa diubah. */
export function ExpenseTypeFormDialog(props: ExpenseTypeFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ExpenseTypeForm {...props} /> : null}
    </Dialog>
  )
}

function ExpenseTypeForm({ onOpenChange, value }: ExpenseTypeFormDialogProps) {
  const queryClient = useQueryClient()
  const accounts = useQuery(accountsQuery(false))
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [accountId, setAccountId] = useState(value?.expenseAccountId ?? "")
  const [withholding, setWithholding] = useState<WithholdingType>(
    value?.withholdingType ?? "none"
  )
  const [approval, setApproval] = useState<ExpenseApprovalSubject>(
    value?.approvalSubject ?? "expense_claim"
  )
  const [requiresReceipt, setRequiresReceipt] = useState(
    value?.requiresReceipt ?? true
  )
  const [requiresCampaign, setRequiresCampaign] = useState(
    value?.requiresCampaign ?? false
  )
  const [description, setDescription] = useState(value?.description ?? "")
  const [isActive, setIsActive] = useState(value?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const options = (accounts.data ?? []).filter(isExpenseAccount)
  const accountLabel = (id: string) => {
    const a = options.find((o) => o.id === id)
    if (a) return `${a.code} ${a.name}`
    if (value && id === value.expenseAccountId)
      return `${value.expenseAccountCode} ${value.expenseAccountName}`
    return "Pilih akun beban"
  }

  const save = useIdempotentMutation({
    mutationFn: (_: void, key): Promise<unknown> => {
      const body = {
        name,
        expenseAccountId: accountId,
        withholdingType: withholding,
        approvalSubject: approval,
        requiresReceipt,
        requiresCampaign,
        description,
        isActive,
      }
      return value
        ? updateExpenseType(value.id, { ...body, version: value.version }, key)
        : createExpenseType({ ...body, code }, key)
    },
    onSuccess: async () => {
      toast.success(
        value ? "Jenis biaya diperbarui" : "Jenis biaya ditambahkan"
      )
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "expense-types"],
      })
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

  const submit = () => {
    const next: Record<string, string> = {}
    if (!value && !code.trim()) next.code = "Kode wajib diisi"
    if (!name.trim()) next.name = "Nama wajib diisi"
    if (!accountId) next.expenseAccountId = "Akun beban wajib dipilih"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {value ? `Ubah Jenis Biaya ${value.code}` : "Tambah Jenis Biaya"}
        </DialogTitle>
        <DialogDescription>
          Kode dipakai di pengajuan biaya dan laporan, tidak bisa diubah atau
          dipakai ulang.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="expense-type-code">Kode</FieldLabel>
            <Input
              id="expense-type-code"
              value={code}
              disabled={!!value}
              aria-invalid={!!errors.code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <FieldDescription>
              Huruf kapital, angka, atau garis bawah, mis. OFFICE_SUPPLY
            </FieldDescription>
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="expense-type-name">Nama</FieldLabel>
            <Input
              id="expense-type-name"
              value={name}
              maxLength={150}
              aria-invalid={!!errors.name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          <Field data-invalid={!!errors.expenseAccountId}>
            <FieldLabel htmlFor="expense-type-account">Akun Beban</FieldLabel>
            <Select
              value={accountId || null}
              onValueChange={(v) => setAccountId(v ? String(v) : "")}
            >
              <SelectTrigger
                id="expense-type-account"
                className="w-full"
                aria-invalid={!!errors.expenseAccountId}
              >
                <SelectValue>
                  {(v: string | null) =>
                    v ? accountLabel(v) : "Pilih akun beban"
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {options.map((a) => (
                  <SelectItem key={a.id} value={a.id}>
                    {a.code} {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              Akun beban operasional (6xxx) atau beban lain-lain (7xxx).
            </FieldDescription>
            <FieldError errors={err("expenseAccountId")} />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.withholdingType}>
              <FieldLabel htmlFor="expense-type-withholding">
                Pemotongan PPh
              </FieldLabel>
              <Select
                value={withholding}
                onValueChange={(v) => v && setWithholding(v as WithholdingType)}
              >
                <SelectTrigger id="expense-type-withholding" className="w-full">
                  <SelectValue>
                    {(v: WithholdingType) => WITHHOLDING_LABELS[v]}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {(Object.keys(WITHHOLDING_LABELS) as WithholdingType[]).map(
                    (w) => (
                      <SelectItem key={w} value={w}>
                        {WITHHOLDING_LABELS[w]}
                      </SelectItem>
                    )
                  )}
                </SelectContent>
              </Select>
              <FieldError errors={err("withholdingType")} />
            </Field>
            <Field data-invalid={!!errors.approvalSubject}>
              <FieldLabel htmlFor="expense-type-approval">
                Alur Persetujuan
              </FieldLabel>
              <Select
                value={approval}
                onValueChange={(v) =>
                  v && setApproval(v as ExpenseApprovalSubject)
                }
              >
                <SelectTrigger id="expense-type-approval" className="w-full">
                  <SelectValue>
                    {(v: ExpenseApprovalSubject) => EXPENSE_APPROVAL_LABELS[v]}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {(
                    Object.keys(
                      EXPENSE_APPROVAL_LABELS
                    ) as ExpenseApprovalSubject[]
                  ).map((s) => (
                    <SelectItem key={s} value={s}>
                      {EXPENSE_APPROVAL_LABELS[s]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError errors={err("approvalSubject")} />
            </Field>
          </div>
          <FieldDescription>
            Approver mengikuti kebijakan persetujuan jenis dokumen ini di
            Pengaturan › Matriks Persetujuan.
          </FieldDescription>
          <Field data-invalid={!!errors.description}>
            <FieldLabel htmlFor="expense-type-description">
              Keterangan (opsional)
            </FieldLabel>
            <Textarea
              id="expense-type-description"
              value={description}
              maxLength={500}
              onChange={(e) => setDescription(e.target.value)}
            />
            <FieldError errors={err("description")} />
          </Field>
          <div className="flex flex-col">
            <Checkbox
              label="Wajib lampirkan bukti"
              checked={requiresReceipt}
              onChange={setRequiresReceipt}
            />
            <Checkbox
              label="Wajib kampanye"
              checked={requiresCampaign}
              onChange={setRequiresCampaign}
            />
            <Checkbox label="Aktif" checked={isActive} onChange={setIsActive} />
          </div>
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
