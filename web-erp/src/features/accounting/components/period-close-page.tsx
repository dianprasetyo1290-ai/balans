import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { AlertTriangleIcon, CheckCircle2Icon, LockIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
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
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import type { ApprovalUserOption } from "@/features/approval/api/approval-api"
import { UserPicker } from "@/features/approval/components/user-picker"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { fiscalPeriodsQuery, type FiscalPeriod } from "../api/accounting-api"
import {
  CLOSE_STEP_STATUS_LABELS,
  closeChecklistQuery,
  closeFiscalPeriod,
  requestReopen,
  updateChecklistStep,
  type CloseChecklist,
  type CloseChecklistStep,
  type CloseStepStatus,
} from "../api/close-api"

const route = getRouteApi("/_app/finance/period-close")

const STEP_TONE: Record<CloseStepStatus, string> = {
  pending: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  done: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  skipped: "bg-muted text-muted-foreground",
}

/** Periode default: periode terbuka paling lama (tutup buku berurutan), atau periode terbaru. */
function defaultPeriod(periods: FiscalPeriod[]) {
  const open = periods.filter((p) => p.status !== "closed")
  return open.length ? open[open.length - 1] : periods[0]
}

/**
 * Keuangan & Akuntansi › Tutup Buku (ACC-08, ACC-14, S-411): checklist 9 langkah BRD 5.15 dengan status & PIC,
 * Tutup Periode (re-autentikasi), dan Buka Kembali dengan approval Head F&A → Direksi.
 */
export function PeriodClosePage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const periods = useQuery(fiscalPeriodsQuery())
  const list = periods.data ?? []
  const periodId = search.period ?? defaultPeriod(list)?.id
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Tutup Buku</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Target paling lambat hari kerja ke-10 bulan berikutnya. Periode yang
            sudah ditutup menolak transaksi bertanggal di periode itu.
          </p>
        </div>
        <Field className="w-full sm:w-56">
          <FieldLabel htmlFor="close-period">Periode</FieldLabel>
          <Select
            value={periodId ?? null}
            onValueChange={(v) =>
              void navigate({ search: { period: v ? String(v) : undefined } })
            }
          >
            <SelectTrigger id="close-period" className="w-full">
              <SelectValue>
                {(v: string | null) =>
                  list.find((p) => p.id === v)?.label ?? "Pilih periode"
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {list.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
      </div>
      <LoadingArea loading={periods.isPending}>
        {periodId ? <Checklist key={periodId} periodId={periodId} /> : null}
      </LoadingArea>
    </div>
  )
}

function Checklist({ periodId }: { periodId: string }) {
  const queryClient = useQueryClient()
  const checklist = useQuery(closeChecklistQuery(periodId))
  const [editing, setEditing] = useState<CloseChecklistStep | null>(null)
  const [confirmClose, setConfirmClose] = useState(false)
  const [reopening, setReopening] = useState(false)

  const refresh = (data: CloseChecklist) => {
    queryClient.setQueryData(closeChecklistQuery(periodId).queryKey, data)
    return queryClient.invalidateQueries({
      queryKey: ["accounting", "fiscal-periods"],
    })
  }
  const close = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      closeFiscalPeriod(periodId, checklist.data?.period.version ?? 0, key),
    onSuccess: async (data) => {
      setConfirmClose(false)
      toast.success(`Periode ${data.period.label} ditutup`)
      await refresh(data)
    },
    onError: (err) => {
      setConfirmClose(false)
      showErrorDialog(parseProblem(err))
    },
  })

  const data = checklist.data
  if (!data) {
    return <LoadingArea loading={checklist.isPending}>{null}</LoadingArea>
  }
  const can = (a: CloseChecklist["allowedActions"][number]) =>
    data.allowedActions.includes(a)
  const done = data.steps.filter((s) => s.status !== "pending").length

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-4">
        <div className="flex flex-wrap items-center gap-3">
          <span className="text-lg font-semibold">{data.period.label}</span>
          <StatusBadge status={data.period.status} />
          <span className="text-sm text-muted-foreground">
            {done}/9 langkah selesai
            {data.period.closedAt
              ? ` · ditutup ${formatDateTime(data.period.closedAt)}`
              : ""}
            {data.period.reopenedCount
              ? ` · dibuka kembali ${data.period.reopenedCount}×`
              : ""}
          </span>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("request_reopen") ? (
            <Button variant="outline" onClick={() => setReopening(true)}>
              Buka Kembali Periode
            </Button>
          ) : null}
          {can("close") ? (
            <Button
              disabled={data.blockers.length > 0}
              onClick={() => setConfirmClose(true)}
            >
              <LockIcon /> Tutup Periode
            </Button>
          ) : null}
        </div>
      </div>
      {data.period.status === "open" && data.blockers.length > 0 ? (
        <div
          role="status"
          className="flex gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm"
        >
          <AlertTriangleIcon
            aria-hidden="true"
            className="mt-0.5 size-4 shrink-0"
          />
          <div>
            <p className="font-medium">Periode belum bisa ditutup</p>
            <ul className="list-disc pl-5">
              {data.blockers.map((b) => (
                <li key={b}>{b}</li>
              ))}
            </ul>
          </div>
        </div>
      ) : null}
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">#</TableHead>
              <TableHead>Aktivitas</TableHead>
              <TableHead>PIC</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Pemeriksaan</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.steps.map((s) => (
              <TableRow key={s.stepCode}>
                <TableCell className="align-top">{s.order}</TableCell>
                <TableCell className="max-w-sm align-top whitespace-normal">
                  <p>{s.title}</p>
                  {s.note ? (
                    <p className="text-xs text-muted-foreground">{s.note}</p>
                  ) : null}
                </TableCell>
                <TableCell className="align-top text-sm">
                  <p>{s.picUserName ?? s.picLabel}</p>
                  {s.picUserName ? (
                    <p className="text-xs text-muted-foreground">
                      {s.picLabel}
                    </p>
                  ) : null}
                </TableCell>
                <TableCell className="align-top">
                  <Badge
                    variant="outline"
                    className={`border-transparent ${STEP_TONE[s.status]}`}
                  >
                    {CLOSE_STEP_STATUS_LABELS[s.status]}
                  </Badge>
                  {s.completedAt ? (
                    <p className="mt-1 text-xs text-muted-foreground">
                      {s.completedByName ?? ""} {formatDateTime(s.completedAt)}
                    </p>
                  ) : null}
                </TableCell>
                <TableCell className="max-w-xs align-top text-sm whitespace-normal">
                  {s.checks.map((c, i) => (
                    <p key={i} className="flex gap-1">
                      {c.ok ? (
                        <CheckCircle2Icon
                          aria-label="Lolos"
                          className="mt-0.5 size-4 shrink-0 text-emerald-600"
                        />
                      ) : (
                        <AlertTriangleIcon
                          aria-label="Belum lolos"
                          className="mt-0.5 size-4 shrink-0 text-amber-600"
                        />
                      )}
                      {c.link ? (
                        <a
                          href={c.link}
                          className="underline-offset-4 hover:underline"
                        >
                          {c.message}
                        </a>
                      ) : (
                        <span>{c.message}</span>
                      )}
                    </p>
                  ))}
                </TableCell>
                <TableCell className="text-right align-top">
                  {can("edit_checklist") &&
                  s.stepCode !== "financial_review" ? (
                    <Button
                      variant="ghost"
                      className="min-h-11 sm:min-h-8"
                      onClick={() => setEditing(s)}
                    >
                      Ubah
                    </Button>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      {data.reopenRequests.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h2 className="text-lg font-semibold">Riwayat Buka Kembali</h2>
          <ul className="flex flex-col gap-2 text-sm">
            {data.reopenRequests.map((r) => (
              <li key={r.id} className="rounded-lg border p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <StatusBadge status={r.status} />
                  <span>
                    {r.requestedByName ?? "—"} · {formatDateTime(r.requestedAt)}
                  </span>
                </div>
                <p className="mt-1">{r.reason}</p>
                {r.decisionNote ? (
                  <p className="text-muted-foreground">
                    Catatan: {r.decisionNote}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      <ConfirmDialog
        open={confirmClose}
        onOpenChange={setConfirmClose}
        title={`Tutup periode ${data.period.label}?`}
        description="Setelah ditutup, transaksi bertanggal di periode ini ditolak. Membuka kembali periode butuh persetujuan Head Finance & Accounting dan Direksi. Anda akan diminta konfirmasi password."
        confirmLabel="Tutup Periode"
        loading={close.isPending}
        onConfirm={() => close.mutate()}
      />
      <StepDialog
        periodId={periodId}
        step={editing}
        onOpenChange={(o) => !o && setEditing(null)}
        onSaved={refresh}
      />
      <ReopenDialog
        open={reopening}
        onOpenChange={setReopening}
        checklist={data}
        onSaved={refresh}
      />
    </div>
  )
}

type StepDialogProps = {
  periodId: string
  step: CloseChecklistStep | null
  onOpenChange: (open: boolean) => void
  onSaved: (data: CloseChecklist) => Promise<unknown>
}

function StepDialog(props: StepDialogProps) {
  return (
    <Dialog open={!!props.step} onOpenChange={props.onOpenChange}>
      {props.step ? <StepForm {...props} step={props.step} /> : null}
    </Dialog>
  )
}

function StepForm({
  periodId,
  step,
  onOpenChange,
  onSaved,
}: StepDialogProps & { step: CloseChecklistStep }) {
  const [status, setStatus] = useState<CloseStepStatus>(step.status)
  const [pic, setPic] = useState<ApprovalUserOption | null>(
    step.picUserId
      ? { id: step.picUserId, fullName: step.picUserName ?? "", email: "" }
      : null
  )
  const [note, setNote] = useState(step.note)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      updateChecklistStep(
        periodId,
        step.stepCode,
        { status, picUserId: pic?.id ?? null, note, version: step.version },
        key
      ),
    onSuccess: async (data) => {
      toast.success(`Langkah ${step.order} disimpan`)
      await onSaved(data)
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
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Langkah {step.order}</DialogTitle>
        <DialogDescription>{step.title}</DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.status}>
            <FieldLabel htmlFor="step-status">Status</FieldLabel>
            <Select
              value={status}
              onValueChange={(v) => setStatus(v as CloseStepStatus)}
            >
              <SelectTrigger id="step-status" className="w-full">
                <SelectValue>
                  {(v: CloseStepStatus) => CLOSE_STEP_STATUS_LABELS[v]}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(["pending", "done", "skipped"] as const).map((s) => (
                  <SelectItem key={s} value={s}>
                    {CLOSE_STEP_STATUS_LABELS[s]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError errors={err("status")} />
          </Field>
          <Field>
            <FieldLabel htmlFor="step-pic">PIC ({step.picLabel})</FieldLabel>
            <UserPicker
              id="step-pic"
              value={pic}
              onValueChange={setPic}
              clearable
            />
          </Field>
          <Field data-invalid={!!errors.note}>
            <FieldLabel htmlFor="step-note">Catatan</FieldLabel>
            <Textarea
              id="step-note"
              value={note}
              placeholder={
                status === "skipped" ? "Alasan langkah tidak berlaku" : ""
              }
              onChange={(e) => setNote(e.target.value)}
            />
            <FieldError errors={err("note")} />
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
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

type ReopenDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  checklist: CloseChecklist
  onSaved: (data: CloseChecklist) => Promise<unknown>
}

function ReopenDialog(props: ReopenDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ReopenForm {...props} /> : null}
    </Dialog>
  )
}

function ReopenForm({ checklist, onOpenChange, onSaved }: ReopenDialogProps) {
  const [reason, setReason] = useState("")
  const [error, setError] = useState("")
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      requestReopen(checklist.period.id, checklist.period.version, reason, key),
    onSuccess: async (data) => {
      toast.success("Permintaan buka kembali diajukan ke Head F&A → Direksi")
      await onSaved(data)
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields?.reason) {
        setError(problem.fields.reason)
        return
      }
      showErrorDialog(problem)
    },
  })
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Buka Kembali Periode {checklist.period.label}</DialogTitle>
        <DialogDescription>
          Periode tetap terkunci sampai disetujui Head Finance & Accounting lalu
          Direksi. Anda akan diminta konfirmasi password.
        </DialogDescription>
      </DialogHeader>
      <Field data-invalid={!!error}>
        <FieldLabel htmlFor="reopen-reason">Alasan</FieldLabel>
        <Textarea
          id="reopen-reason"
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
        <FieldError errors={error ? [{ message: error }] : []} />
      </Field>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Batal
        </Button>
        <LoadingButton
          loading={save.isPending}
          loadingText="Mengajukan…"
          onClick={() => save.mutate()}
        >
          Ajukan Buka Kembali
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
