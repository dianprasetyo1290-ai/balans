import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  accountsQuery,
  campaignsQuery,
  channelsQuery,
  createJournal,
  divisionsQuery,
  journalQuery,
  subChannelsQuery,
  updateJournal,
  type JournalEntry,
  type ManualJournalLineInput,
} from "../api/accounting-api"
import { journalTotals } from "../lib/journal-totals"
import { DimensionSelect } from "./dimension-select"

type LineDraft = {
  key: string
  accountId: string
  debit: string
  credit: string
  channelId: string | null
  subChannelId: string | null
  divisionId: string | null
  campaignId: string | null
  memo: string
}

const emptyLine = (): LineDraft => ({
  key: crypto.randomUUID(),
  accountId: "",
  debit: "",
  credit: "",
  channelId: null,
  subChannelId: null,
  divisionId: null,
  campaignId: null,
  memo: "",
})

const fromEntry = (e: JournalEntry): LineDraft[] =>
  e.lines.map((l) => ({
    key: crypto.randomUUID(),
    accountId: l.accountId,
    debit: Number(l.debit) === 0 ? "" : l.debit,
    credit: Number(l.credit) === 0 ? "" : l.credit,
    channelId: l.channel?.id ?? null,
    subChannelId: l.subChannel?.id ?? null,
    divisionId: l.division?.id ?? null,
    campaignId: l.campaign?.id ?? null,
    memo: l.memo,
  }))

/** Keuangan & Akuntansi › Jurnal Umum › Buat Jurnal Umum (S-127). */
export function JournalCreatePage() {
  return <JournalForm />
}

/** Ubah draft Jurnal Umum. */
export function JournalEditPage({ id }: { id: string }) {
  const journal = useQuery(journalQuery(id))
  return (
    <LoadingArea loading={journal.isPending}>
      {journal.data ? (
        journal.data.allowedActions.includes("edit") ? (
          <JournalForm entry={journal.data} />
        ) : (
          <p className="text-sm text-muted-foreground">
            Jurnal yang sudah diposting tidak bisa diubah. Koreksi dengan Balik
            Jurnal.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

function JournalForm({ entry }: { entry?: JournalEntry }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const accounts = useQuery(accountsQuery(false))
  const channels = useQuery(channelsQuery())
  const subChannels = useQuery(subChannelsQuery())
  const divisions = useQuery(divisionsQuery())
  const campaigns = useQuery(campaignsQuery())
  const [businessDate, setBusinessDate] = useState(
    entry?.businessDate ?? todayBusinessDate()
  )
  const [description, setDescription] = useState(entry?.description ?? "")
  const [lines, setLines] = useState<LineDraft[]>(() =>
    entry ? fromEntry(entry) : [emptyLine(), emptyLine()]
  )
  const [errors, setErrors] = useState<Record<string, string>>({})

  const manualAccounts = (accounts.data ?? []).filter(
    (a) => a.isPostable && a.allowManualPosting && a.isActive
  )
  const totals = journalTotals(lines)

  const update = (key: string, patch: Partial<LineDraft>) =>
    setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        businessDate,
        description,
        lines: lines.map((l): ManualJournalLineInput => ({
          accountId: l.accountId,
          debit: l.debit || "0",
          credit: l.credit || "0",
          channelId: l.channelId,
          subChannelId: l.subChannelId,
          divisionId: l.divisionId,
          campaignId: l.campaignId,
          memo: l.memo,
        })),
      }
      return entry
        ? updateJournal(entry.id, { ...body, version: entry.version }, key)
        : createJournal(body, key)
    },
    onSuccess: async (saved) => {
      toast.success("Draft jurnal disimpan")
      await queryClient.invalidateQueries({ queryKey: ["accounting"] })
      void navigate({
        to: "/finance/journals/$id",
        params: { id: saved.id },
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
    if (!businessDate) next.businessDate = "Tanggal jurnal wajib diisi"
    if (!description.trim()) next.description = "Keterangan jurnal wajib diisi"
    if (lines.length < 2) next.lines = "Jurnal minimal dua baris"
    lines.forEach((l, i) => {
      if (!l.accountId) next[`lines.${i}.accountId`] = "Akun wajib dipilih"
      const d = Number(l.debit || 0)
      const c = Number(l.credit || 0)
      if (d > 0 && c > 0)
        next[`lines.${i}.debit`] = "Isi debit atau kredit, tidak keduanya"
      if (d === 0 && c === 0)
        next[`lines.${i}.debit`] = "Isi nominal debit atau kredit"
    })
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  const lineError = (i: number, field: string) => errors[`lines.${i}.${field}`]
  const accountLabel = (id: string) => {
    const a = (accounts.data ?? []).find((x) => x.id === id)
    return a ? `${a.code} ${a.name}` : "Pilih akun"
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {entry ? "Ubah Jurnal Umum" : "Buat Jurnal Umum"}
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Draft bisa disimpan bertahap. Saat posting, jurnal harus balance, di
          periode terbuka, dan dimensi wajib terisi: pendapatan wajib channel +
          sub-channel; HPP, beban, dan lain-lain wajib divisi.
        </p>
      </div>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="grid gap-4 sm:grid-cols-[14rem_1fr]">
          <Field data-invalid={!!errors.businessDate}>
            <FieldLabel htmlFor="journal-date">Tanggal</FieldLabel>
            <DateInput
              id="journal-date"
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
          <Field data-invalid={!!errors.description}>
            <FieldLabel htmlFor="journal-description">Keterangan</FieldLabel>
            <Textarea
              id="journal-description"
              value={description}
              maxLength={500}
              rows={2}
              aria-invalid={!!errors.description}
              onChange={(e) => setDescription(e.target.value)}
            />
            <FieldError
              errors={
                errors.description ? [{ message: errors.description }] : []
              }
            />
          </Field>
        </div>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[72rem]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-72">Akun</TableHead>
                <TableHead className="w-40 text-right">Debit</TableHead>
                <TableHead className="w-40 text-right">Kredit</TableHead>
                <TableHead>Channel</TableHead>
                <TableHead>Sub-channel</TableHead>
                <TableHead>Divisi</TableHead>
                <TableHead>Kampanye</TableHead>
                <TableHead>Keterangan Baris</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {lines.map((l, i) => (
                <TableRow key={l.key} className="align-top">
                  <TableCell>
                    <Select
                      value={l.accountId}
                      onValueChange={(v) =>
                        update(l.key, { accountId: String(v ?? "") })
                      }
                    >
                      <SelectTrigger
                        className="w-full"
                        aria-label={`Akun baris ${i + 1}`}
                        aria-invalid={!!lineError(i, "accountId")}
                      >
                        <SelectValue>
                          {(v: string) => (v ? accountLabel(v) : "Pilih akun")}
                        </SelectValue>
                      </SelectTrigger>
                      <SelectContent>
                        {manualAccounts.map((a) => (
                          <SelectItem key={a.id} value={a.id}>
                            {a.code} {a.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <LineError message={lineError(i, "accountId")} />
                  </TableCell>
                  <TableCell>
                    <MoneyInput
                      aria-label={`Debit baris ${i + 1}`}
                      value={l.debit}
                      aria-invalid={!!lineError(i, "debit")}
                      onValueChange={(v) =>
                        update(l.key, { debit: v, credit: v ? "" : l.credit })
                      }
                    />
                    <LineError message={lineError(i, "debit")} />
                  </TableCell>
                  <TableCell>
                    <MoneyInput
                      aria-label={`Kredit baris ${i + 1}`}
                      value={l.credit}
                      aria-invalid={!!lineError(i, "credit")}
                      onValueChange={(v) =>
                        update(l.key, { credit: v, debit: v ? "" : l.debit })
                      }
                    />
                    <LineError message={lineError(i, "credit")} />
                  </TableCell>
                  <TableCell>
                    <DimensionSelect
                      aria-label={`Channel baris ${i + 1}`}
                      value={l.channelId}
                      options={channels.data ?? []}
                      placeholder="—"
                      aria-invalid={!!lineError(i, "channelId")}
                      onValueChange={(v) =>
                        update(l.key, { channelId: v, subChannelId: null })
                      }
                    />
                    <LineError message={lineError(i, "channelId")} />
                  </TableCell>
                  <TableCell>
                    <DimensionSelect
                      aria-label={`Sub-channel baris ${i + 1}`}
                      value={l.subChannelId}
                      disabled={!l.channelId}
                      options={(subChannels.data ?? []).filter(
                        (s) => s.channelId === l.channelId
                      )}
                      placeholder="—"
                      aria-invalid={!!lineError(i, "subChannelId")}
                      onValueChange={(v) => update(l.key, { subChannelId: v })}
                    />
                    <LineError message={lineError(i, "subChannelId")} />
                  </TableCell>
                  <TableCell>
                    <DimensionSelect
                      aria-label={`Divisi baris ${i + 1}`}
                      value={l.divisionId}
                      options={divisions.data ?? []}
                      placeholder="—"
                      aria-invalid={!!lineError(i, "divisionId")}
                      onValueChange={(v) => update(l.key, { divisionId: v })}
                    />
                    <LineError message={lineError(i, "divisionId")} />
                  </TableCell>
                  <TableCell>
                    <DimensionSelect
                      aria-label={`Kampanye baris ${i + 1}`}
                      value={l.campaignId}
                      options={campaigns.data ?? []}
                      placeholder="—"
                      aria-invalid={!!lineError(i, "campaignId")}
                      onValueChange={(v) => update(l.key, { campaignId: v })}
                    />
                    <LineError message={lineError(i, "campaignId")} />
                  </TableCell>
                  <TableCell>
                    <Input
                      aria-label={`Keterangan baris ${i + 1}`}
                      value={l.memo}
                      maxLength={500}
                      onChange={(e) => update(l.key, { memo: e.target.value })}
                    />
                  </TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`Hapus baris ${i + 1}`}
                      disabled={lines.length <= 2}
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
                <TableCell className="font-medium">Total</TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatRupiah(totals.debit)}
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatRupiah(totals.credit)}
                </TableCell>
                <TableCell colSpan={6}>
                  <span
                    className={cn(
                      "text-sm",
                      totals.balanced
                        ? "text-emerald-700 dark:text-emerald-300"
                        : "text-amber-700 dark:text-amber-300"
                    )}
                  >
                    {totals.balanced
                      ? "Balance"
                      : `Selisih ${formatRupiah(totals.difference)}`}
                  </span>
                </TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        </div>
        {errors.lines ? (
          <p className="text-sm text-destructive">{errors.lines}</p>
        ) : null}
        <div className="flex flex-wrap justify-between gap-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => setLines((ls) => [...ls, emptyLine()])}
          >
            <PlusIcon /> Tambah Baris
          </Button>
          <div className="flex gap-2">
            <Button
              variant="outline"
              render={
                entry ? (
                  <Link to="/finance/journals/$id" params={{ id: entry.id }} />
                ) : (
                  <Link to="/finance/journals" search={{ page: 1 }} />
                )
              }
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
    </div>
  )
}

function LineError({ message }: { message?: string }) {
  return message ? (
    <p className="mt-1 text-xs text-destructive">{message}</p>
  ) : null
}
