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
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  approveRequest,
  rejectRequest,
  type ApprovalRequest,
} from "../api/approval-api"

export type Decision = "approve" | "reject"

type DecisionDialogProps = {
  request: ApprovalRequest | null
  decision: Decision
  onOpenChange: (open: boolean) => void
  onDecided?: (request: ApprovalRequest) => void
}

const MAX_NOTE = 1000

/** Setujui (catatan opsional) atau Tolak (catatan wajib) satu dokumen (S-111). */
export function DecisionDialog(props: DecisionDialogProps) {
  return (
    <Dialog open={!!props.request} onOpenChange={props.onOpenChange}>
      {props.request ? (
        <DecisionForm
          key={`${props.request.id}-${props.decision}`}
          {...props}
          request={props.request}
        />
      ) : null}
    </Dialog>
  )
}

function DecisionForm({
  request,
  decision,
  onOpenChange,
  onDecided,
}: DecisionDialogProps & { request: ApprovalRequest }) {
  const queryClient = useQueryClient()
  const [note, setNote] = useState("")
  const [error, setError] = useState<string>()
  const reject = decision === "reject"

  const save = useIdempotentMutation({
    mutationFn: (r: ApprovalRequest, key) =>
      reject
        ? rejectRequest(r.id, r.version, note.trim(), key)
        : approveRequest(r.id, r.version, note.trim(), key),
    onSuccess: async (updated) => {
      toast.success(
        reject
          ? `${updated.subjectLabel} ditolak`
          : updated.status === "approved"
            ? `${updated.subjectLabel} disetujui`
            : `${updated.subjectLabel} disetujui di level ${updated.currentLevel - 1}, diteruskan ke level berikutnya`
      )
      await queryClient.invalidateQueries({ queryKey: ["approval"] })
      await queryClient.invalidateQueries({ queryKey: ["notification"] })
      onDecided?.(updated)
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.code === "VALIDATION_FAILED" && problem.fields?.note) {
        setError(problem.fields.note)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    if (reject && note.trim() === "") {
      setError("Catatan wajib diisi saat menolak")
      return
    }
    if (note.length > MAX_NOTE) {
      setError("Catatan maksimal 1.000 karakter")
      return
    }
    save.mutate(request)
  }

  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {reject ? "Tolak" : "Setujui"} {request.subjectTypeLabel}
        </DialogTitle>
        <DialogDescription>
          {`${request.subjectLabel} dari ${request.requestedByName}${
            Number(request.amount) > 0
              ? ` · ${formatRupiah(request.amount)}`
              : ""
          } · Level ${request.currentLevel} dari ${request.totalLevels}`}
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <Field data-invalid={!!error}>
          <FieldLabel htmlFor="decision-note">
            Catatan{reject ? "" : " (opsional)"}
          </FieldLabel>
          <Textarea
            id="decision-note"
            rows={4}
            value={note}
            maxLength={MAX_NOTE}
            aria-invalid={!!error}
            placeholder={
              reject
                ? "Jelaskan alasan penolakan agar pembuat bisa merevisi"
                : ""
            }
            onChange={(e) => {
              setNote(e.target.value)
              setError(undefined)
            }}
          />
          <FieldError errors={error ? [{ message: error }] : []} />
        </Field>
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
            variant={reject ? "destructive" : "default"}
            loading={save.isPending}
            loadingText="Memproses…"
          >
            {reject ? "Tolak" : "Setujui"}
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
