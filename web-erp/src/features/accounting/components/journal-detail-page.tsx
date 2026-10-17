import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DateInput } from "@/components/common/date-input"
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
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
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
  deleteJournal,
  ENTRY_TYPE_LABELS,
  journalQuery,
  postJournal,
  reverseJournal,
  type DimensionRef,
  type JournalEntry,
} from "../api/accounting-api"

function dim(ref: DimensionRef | null | undefined) {
  return ref ? ref.name || ref.code : "—"
}

/** Detail jurnal: baris, dimensi, posting, dan Balik Jurnal (S-127, S-128). */
export function JournalDetailPage({ id }: { id: string }) {
  const journal = useQuery(journalQuery(id))
  return (
    <LoadingArea loading={journal.isPending}>
      {journal.data ? <JournalDetail entry={journal.data} /> : null}
    </LoadingArea>
  )
}

function JournalDetail({ entry }: { entry: JournalEntry }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<"post" | "delete" | null>(null)
  const [reversing, setReversing] = useState(false)
  const can = (a: JournalEntry["allowedActions"][number]) =>
    entry.allowedActions.includes(a)

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["accounting"] })

  const action = useIdempotentMutation({
    mutationFn: (kind: "post" | "delete", key) =>
      kind === "post"
        ? postJournal(entry.id, entry.version, key)
        : deleteJournal(entry.id, entry.version, key),
    onSuccess: async (_, kind) => {
      setConfirm(null)
      if (kind === "post") {
        toast.success("Jurnal diposting")
        await refresh()
      } else {
        toast.success("Draft jurnal dihapus")
        await refresh()
        void navigate({ to: "/finance/journals", search: { page: 1 } })
      }
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex flex-col gap-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {entry.number ?? "Draft Jurnal Umum"}
            </h1>
            <StatusBadge status={entry.status} />
            <Badge variant="secondary">
              {ENTRY_TYPE_LABELS[entry.entryType]}
            </Badge>
            {entry.reversedByNumber && entry.reversedById ? (
              <Badge variant="outline">Dibalik</Badge>
            ) : null}
          </div>
          <p className="text-sm text-muted-foreground">{entry.description}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button
              variant="outline"
              render={
                <Link
                  to="/finance/journals/$id/edit"
                  params={{ id: entry.id }}
                />
              }
            >
              Ubah
            </Button>
          ) : null}
          {can("delete") ? (
            <Button variant="destructive" onClick={() => setConfirm("delete")}>
              Hapus
            </Button>
          ) : null}
          {can("reverse") ? (
            <Button variant="outline" onClick={() => setReversing(true)}>
              Balik Jurnal
            </Button>
          ) : null}
          {can("post") ? (
            <Button onClick={() => setConfirm("post")}>Posting Jurnal</Button>
          ) : null}
        </div>
      </div>
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <Info label="Tanggal" value={formatDate(entry.businessDate)} />
        <Info
          label="Diposting"
          value={entry.postedAt ? formatDateTime(entry.postedAt) : "—"}
        />
        {entry.entryType === "automatic" ? (
          <Info
            label="Dokumen Sumber"
            value={`${entry.postingRuleCode ?? ""} · ${entry.sourceLabel || entry.sourceType}`}
          />
        ) : null}
        {entry.reversalOfId ? (
          <div>
            <dt className="text-muted-foreground">Membalik Jurnal</dt>
            <dd>
              <Link
                to="/finance/journals/$id"
                params={{ id: entry.reversalOfId }}
                className="text-primary underline-offset-4 hover:underline"
              >
                {entry.reversalOfNumber}
              </Link>
            </dd>
          </div>
        ) : null}
        {entry.reversedById ? (
          <div>
            <dt className="text-muted-foreground">Dibalik oleh</dt>
            <dd>
              <Link
                to="/finance/journals/$id"
                params={{ id: entry.reversedById }}
                className="text-primary underline-offset-4 hover:underline"
              >
                {entry.reversedByNumber}
              </Link>
            </dd>
          </div>
        ) : null}
      </dl>
      <div className="overflow-x-auto rounded-lg border">
        <Table className="min-w-[56rem]">
          <TableHeader>
            <TableRow>
              <TableHead>Akun</TableHead>
              <TableHead className="text-right">Debit</TableHead>
              <TableHead className="text-right">Kredit</TableHead>
              <TableHead>Channel</TableHead>
              <TableHead>Sub-channel</TableHead>
              <TableHead>Divisi</TableHead>
              <TableHead>Kampanye</TableHead>
              <TableHead>Keterangan Baris</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entry.lines.map((l) => (
              <TableRow key={l.lineNo}>
                <TableCell>
                  <span className="tabular-nums">{l.accountCode}</span>{" "}
                  {l.accountName}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {Number(l.debit) ? formatRupiah(l.debit) : ""}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {Number(l.credit) ? formatRupiah(l.credit) : ""}
                </TableCell>
                <TableCell>{dim(l.channel)}</TableCell>
                <TableCell>{dim(l.subChannel)}</TableCell>
                <TableCell>{dim(l.division)}</TableCell>
                <TableCell>{l.campaign ? l.campaign.code : "—"}</TableCell>
                <TableCell className="text-muted-foreground">
                  {l.memo || "—"}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
          <TableFooter>
            <TableRow>
              <TableCell className="font-medium">Total</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRupiah(entry.totalDebit)}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRupiah(entry.totalCredit)}
              </TableCell>
              <TableCell colSpan={5} />
            </TableRow>
          </TableFooter>
        </Table>
      </div>
      {confirm ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirm(null)}
          title={
            confirm === "post" ? "Posting jurnal ini?" : "Hapus draft jurnal?"
          }
          description={
            confirm === "post"
              ? "Jurnal mendapat nomor JV dan tidak bisa diubah atau dihapus lagi. Koreksi hanya lewat Balik Jurnal."
              : "Draft dihapus dan tidak tampil di daftar jurnal."
          }
          confirmLabel={confirm === "post" ? "Posting Jurnal" : "Hapus Draft"}
          destructive={confirm === "delete"}
          loading={action.isPending}
          onConfirm={() => action.mutate(confirm)}
        />
      ) : null}
      <ReverseDialog
        entry={entry}
        open={reversing}
        onOpenChange={setReversing}
        onReversed={async (r) => {
          await refresh()
          void navigate({ to: "/finance/journals/$id", params: { id: r.id } })
        }}
      />
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}

type ReverseDialogProps = {
  entry: JournalEntry
  open: boolean
  onOpenChange: (open: boolean) => void
  onReversed: (reversal: JournalEntry) => Promise<void>
}

function ReverseDialog(props: ReverseDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ReverseForm {...props} /> : null}
    </Dialog>
  )
}

function ReverseForm({ entry, onOpenChange, onReversed }: ReverseDialogProps) {
  const [businessDate, setBusinessDate] = useState(todayBusinessDate())
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})

  const reverse = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      reverseJournal(
        entry.id,
        { version: entry.version, businessDate, reason },
        key
      ),
    onSuccess: async (r) => {
      toast.success(`Jurnal dibalik dengan ${r.number ?? "jurnal pembalik"}`)
      onOpenChange(false)
      await onReversed(r)
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

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Balik Jurnal {entry.number}</DialogTitle>
        <DialogDescription>
          Sistem membuat JV pembalik dengan debit dan kredit ditukar. Jurnal
          asal tetap tidak berubah.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!reason.trim()) {
            setErrors({ reason: "Alasan Balik Jurnal wajib diisi" })
            return
          }
          reverse.mutate()
        }}
      >
        <Field data-invalid={!!errors.businessDate}>
          <FieldLabel htmlFor="reverse-date">Tanggal pembalik</FieldLabel>
          <DateInput
            id="reverse-date"
            value={businessDate}
            onValueChange={setBusinessDate}
            aria-invalid={!!errors.businessDate}
          />
          <FieldError
            errors={
              errors.businessDate ? [{ message: errors.businessDate }] : []
            }
          />
        </Field>
        <Field data-invalid={!!errors.reason}>
          <FieldLabel htmlFor="reverse-reason">Alasan</FieldLabel>
          <Textarea
            id="reverse-reason"
            value={reason}
            maxLength={400}
            aria-invalid={!!errors.reason}
            onChange={(e) => setReason(e.target.value)}
          />
          <FieldError
            errors={errors.reason ? [{ message: errors.reason }] : []}
          />
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
            loading={reverse.isPending}
            loadingText="Memproses…"
          >
            Balik Jurnal
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
