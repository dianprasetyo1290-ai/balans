import { useQueryClient } from "@tanstack/react-query"
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
import { Checkbox } from "@/features/accounting/components/account-form-dialog"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  APPLIES_TO_LABELS,
  createReturnReason,
  DISPOSITION_LABELS,
  updateReturnReason,
  type ReturnDisposition,
  type ReturnReason,
  type ReturnReasonAppliesTo,
} from "../api/sales-api"

const NONE = "none"

type ReturnReasonFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = alasan baru. */
  value?: ReturnReason
}

/** Tambah/ubah alasan retur (S-124). Kode tidak bisa diubah. */
export function ReturnReasonFormDialog(props: ReturnReasonFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ReturnReasonForm {...props} /> : null}
    </Dialog>
  )
}

function ReturnReasonForm({
  onOpenChange,
  value,
}: ReturnReasonFormDialogProps) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [appliesTo, setAppliesTo] = useState<ReturnReasonAppliesTo>(
    value?.appliesTo ?? "both"
  )
  const [disposition, setDisposition] = useState<ReturnDisposition | null>(
    value?.defaultDisposition ?? null
  )
  const [description, setDescription] = useState(value?.description ?? "")
  const [isActive, setIsActive] = useState(value?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key): Promise<unknown> => {
      const body = {
        name,
        appliesTo,
        defaultDisposition: disposition,
        description,
        isActive,
      }
      return value
        ? updateReturnReason(value.id, { ...body, version: value.version }, key)
        : createReturnReason({ ...body, code }, key)
    },
    onSuccess: async () => {
      toast.success(
        value ? "Alasan retur diperbarui" : "Alasan retur ditambahkan"
      )
      await queryClient.invalidateQueries({
        queryKey: ["sales", "return-reasons"],
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
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {value ? `Ubah Alasan Retur ${value.code}` : "Tambah Alasan Retur"}
        </DialogTitle>
        <DialogDescription>
          Kode dipakai di dokumen retur dan laporan, tidak bisa diubah atau
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
            <FieldLabel htmlFor="return-reason-code">Kode</FieldLabel>
            <Input
              id="return-reason-code"
              value={code}
              disabled={!!value}
              aria-invalid={!!errors.code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <FieldDescription>
              Huruf kapital, angka, atau garis bawah, mis. DAMAGED_SHIPPING
            </FieldDescription>
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="return-reason-name">Nama</FieldLabel>
            <Input
              id="return-reason-name"
              value={name}
              maxLength={150}
              aria-invalid={!!errors.name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.appliesTo}>
              <FieldLabel htmlFor="return-reason-applies">
                Berlaku untuk
              </FieldLabel>
              <Select
                value={appliesTo}
                onValueChange={(v) =>
                  v && setAppliesTo(v as ReturnReasonAppliesTo)
                }
              >
                <SelectTrigger id="return-reason-applies" className="w-full">
                  <SelectValue>
                    {(v: ReturnReasonAppliesTo) => APPLIES_TO_LABELS[v]}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {(
                    Object.keys(APPLIES_TO_LABELS) as ReturnReasonAppliesTo[]
                  ).map((a) => (
                    <SelectItem key={a} value={a}>
                      {APPLIES_TO_LABELS[a]}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError errors={err("appliesTo")} />
            </Field>
            <Field data-invalid={!!errors.defaultDisposition}>
              <FieldLabel htmlFor="return-reason-disposition">
                Usulan Disposisi
              </FieldLabel>
              <Select
                value={disposition ?? NONE}
                onValueChange={(v) =>
                  setDisposition(
                    !v || v === NONE ? null : (v as ReturnDisposition)
                  )
                }
              >
                <SelectTrigger
                  id="return-reason-disposition"
                  className="w-full"
                >
                  <SelectValue>
                    {(v: string) =>
                      v === NONE
                        ? "Tanpa usulan"
                        : DISPOSITION_LABELS[v as ReturnDisposition]
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>Tanpa usulan</SelectItem>
                  {(Object.keys(DISPOSITION_LABELS) as ReturnDisposition[]).map(
                    (d) => (
                      <SelectItem key={d} value={d}>
                        {DISPOSITION_LABELS[d]}
                      </SelectItem>
                    )
                  )}
                </SelectContent>
              </Select>
              <FieldError errors={err("defaultDisposition")} />
            </Field>
          </div>
          <FieldDescription>
            Usulan disposisi hanya nilai awal di Disposisi Retur; staf gudang
            tetap memutuskan setelah QC.
          </FieldDescription>
          <Field data-invalid={!!errors.description}>
            <FieldLabel htmlFor="return-reason-description">
              Keterangan (opsional)
            </FieldLabel>
            <Textarea
              id="return-reason-description"
              value={description}
              maxLength={500}
              onChange={(e) => setDescription(e.target.value)}
            />
            <FieldError errors={err("description")} />
          </Field>
          <Checkbox label="Aktif" checked={isActive} onChange={setIsActive} />
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
