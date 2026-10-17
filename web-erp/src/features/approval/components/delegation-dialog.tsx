import { useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { createDelegation, type ApprovalUserOption } from "../api/approval-api"
import { UserPicker } from "./user-picker"

type DelegationDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentUserId: string
  /** Pengelola kebijakan boleh mencatat delegasi untuk user lain. */
  canChooseFrom?: boolean
}

type Errors = Partial<
  Record<"fromUserId" | "toUserId" | "validFrom" | "validTo" | "reason", string>
>

/** Delegasikan persetujuan selama cuti (BRD 5.17, S-111). */
export function DelegationDialog(props: DelegationDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <DelegationForm {...props} /> : null}
    </Dialog>
  )
}

function DelegationForm({
  onOpenChange,
  currentUserId,
  canChooseFrom = false,
}: DelegationDialogProps) {
  const queryClient = useQueryClient()
  const [from, setFrom] = useState<ApprovalUserOption | null>(null)
  const [to, setTo] = useState<ApprovalUserOption | null>(null)
  const [validFrom, setValidFrom] = useState(todayBusinessDate)
  const [validTo, setValidTo] = useState("")
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<Errors>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createDelegation(
        {
          fromUserId: from?.id ?? null,
          toUserId: to?.id ?? "",
          validFrom,
          validTo,
          reason: reason.trim() || undefined,
        },
        key
      ),
    onSuccess: async (d) => {
      toast.success(`Persetujuan didelegasikan ke ${d.toUserName}`)
      await queryClient.invalidateQueries({
        queryKey: ["approval", "delegations"],
      })
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.code === "VALIDATION_FAILED" && problem.fields) {
        setErrors(problem.fields as Errors)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    const next: Errors = {}
    if (!to) next.toUserId = "Penerima delegasi wajib diisi"
    if (!validFrom) next.validFrom = "Tanggal mulai wajib diisi"
    if (!validTo) next.validTo = "Tanggal selesai wajib diisi"
    else if (validFrom && validTo < validFrom)
      next.validTo = "Tanggal selesai tidak boleh sebelum tanggal mulai"
    setErrors(next)
    if (Object.keys(next).length === 0) {
      save.mutate()
    }
  }

  const err = (k: keyof Errors) => (errors[k] ? [{ message: errors[k] }] : [])
  const fromId = from?.id ?? currentUserId

  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Delegasikan Persetujuan</DialogTitle>
        <DialogDescription>
          Selama periode ini penerima bisa menyetujui atau menolak dokumen atas
          nama pemberi delegasi. Maksimal 90 hari.
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
          {canChooseFrom ? (
            <Field data-invalid={!!errors.fromUserId}>
              <FieldLabel htmlFor="delegation-from">
                Pemberi delegasi
              </FieldLabel>
              <UserPicker
                id="delegation-from"
                value={from}
                onValueChange={setFrom}
                placeholder="Saya sendiri"
                clearable
                aria-invalid={!!errors.fromUserId}
              />
              <FieldDescription>
                Kosongkan untuk mendelegasikan persetujuan Anda sendiri.
              </FieldDescription>
              <FieldError errors={err("fromUserId")} />
            </Field>
          ) : null}
          <Field data-invalid={!!errors.toUserId}>
            <FieldLabel htmlFor="delegation-to">Penerima delegasi</FieldLabel>
            <UserPicker
              id="delegation-to"
              value={to}
              onValueChange={setTo}
              excludeIds={[fromId]}
              aria-invalid={!!errors.toUserId}
            />
            <FieldError errors={err("toUserId")} />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.validFrom}>
              <FieldLabel htmlFor="delegation-valid-from">Mulai</FieldLabel>
              <DateInput
                id="delegation-valid-from"
                value={validFrom}
                onValueChange={setValidFrom}
                aria-invalid={!!errors.validFrom}
              />
              <FieldError errors={err("validFrom")} />
            </Field>
            <Field data-invalid={!!errors.validTo}>
              <FieldLabel htmlFor="delegation-valid-to">Sampai</FieldLabel>
              <DateInput
                id="delegation-valid-to"
                value={validTo}
                onValueChange={setValidTo}
                aria-invalid={!!errors.validTo}
              />
              <FieldError errors={err("validTo")} />
            </Field>
          </div>
          <Field data-invalid={!!errors.reason}>
            <FieldLabel htmlFor="delegation-reason">
              Alasan (opsional)
            </FieldLabel>
            <Textarea
              id="delegation-reason"
              rows={2}
              maxLength={500}
              value={reason}
              placeholder="Mis. cuti tahunan"
              onChange={(e) => setReason(e.target.value)}
            />
            <FieldError errors={err("reason")} />
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
            Simpan Delegasi
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
