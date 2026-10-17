import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  channelsQuery,
  subChannelsQuery,
} from "@/features/accounting/api/accounting-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate } from "@/lib/format/date"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  endDated,
  PERIOD_STATUS_LABELS,
  type DatedResource,
  type PricingPeriodStatus,
} from "../api/pricing-api"

const ALL = "__all"

/** Nama channel dan sub-channel dari master dimensi (butuh accounting.dimension.read; tanpa itu tampil "—"). */
export function useChannelNames() {
  const channels = useQuery({ ...channelsQuery(true), retry: false })
  const subChannels = useQuery({ ...subChannelsQuery(true), retry: false })
  const names = new Map<string, string>()
  for (const c of channels.data ?? []) names.set(c.id, c.name)
  for (const s of subChannels.data ?? []) names.set(s.id, s.name)
  return {
    channels: (channels.data ?? []).filter((c) => c.isActive),
    subChannels: (subChannels.data ?? []).filter((s) => s.isActive),
    name: (id?: string | null) => (id ? (names.get(id) ?? "—") : null),
  }
}

type ChannelFieldsProps = {
  idPrefix: string
  channelId: string | null
  subChannelId: string | null
  onChange: (channelId: string | null, subChannelId: string | null) => void
  /** Izinkan "Semua channel" (promo, simulasi). */
  allowAll?: boolean
  disabled?: boolean
  errors?: { channelId?: string; subChannelId?: string }
}

/** Pilihan channel dan sub-channel (opsional) dari master dimensi Keuangan. */
export function ChannelFields({
  idPrefix,
  channelId,
  subChannelId,
  onChange,
  allowAll = false,
  disabled,
  errors = {},
}: ChannelFieldsProps) {
  const { channels, subChannels, name } = useChannelNames()
  const subs = subChannels.filter((s) => s.channelId === channelId)
  return (
    <>
      <Field data-invalid={!!errors.channelId}>
        <FieldLabel htmlFor={`${idPrefix}-channel`}>Channel</FieldLabel>
        <Select
          value={channelId ?? ALL}
          disabled={disabled}
          onValueChange={(v) =>
            onChange(!v || v === ALL ? null : (v as string), null)
          }
        >
          <SelectTrigger
            id={`${idPrefix}-channel`}
            className="w-full"
            aria-invalid={!!errors.channelId}
          >
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? allowAll
                    ? "Semua channel"
                    : "Pilih channel"
                  : (name(v) ?? "Pilih channel")
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            {allowAll ? (
              <SelectItem value={ALL}>Semua channel</SelectItem>
            ) : null}
            {channels.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FieldError
          errors={errors.channelId ? [{ message: errors.channelId }] : []}
        />
      </Field>
      <Field data-invalid={!!errors.subChannelId}>
        <FieldLabel htmlFor={`${idPrefix}-sub-channel`}>
          Sub-channel (opsional)
        </FieldLabel>
        <Select
          value={subChannelId ?? ALL}
          disabled={disabled || !channelId || subs.length === 0}
          onValueChange={(v) =>
            onChange(channelId, !v || v === ALL ? null : (v as string))
          }
        >
          <SelectTrigger id={`${idPrefix}-sub-channel`} className="w-full">
            <SelectValue>
              {(v: string) =>
                v === ALL ? "Semua sub-channel" : (name(v) ?? "—")
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua sub-channel</SelectItem>
            {subs.map((s) => (
              <SelectItem key={s.id} value={s.id}>
                {s.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FieldError
          errors={errors.subChannelId ? [{ message: errors.subChannelId }] : []}
        />
      </Field>
    </>
  )
}

const PERIOD_TONES: Record<PricingPeriodStatus, string> = {
  scheduled: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  active: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  expired: "bg-muted text-muted-foreground",
}

/** Status periode berlaku: Terjadwal, Berlaku, Berakhir. */
export function PeriodBadge({ status }: { status: PricingPeriodStatus }) {
  return (
    <Badge
      variant="outline"
      className={cn("border-transparent", PERIOD_TONES[status])}
    >
      {PERIOD_STATUS_LABELS[status]}
    </Badge>
  )
}

/** Periode "1 Nov 2026 – 30 Nov 2026" atau "Mulai 1 Nov 2026". */
export function formatPeriod(validFrom: string, validTo?: string | null) {
  return validTo
    ? `${formatDate(validFrom)} – ${formatDate(validTo)}`
    : `Mulai ${formatDate(validFrom)}`
}

type EndPeriodDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  resource: DatedResource
  item: { id: string; version: number; validFrom: string }
  title: string
  queryKey: readonly unknown[]
}

/**
 * Mengisi tanggal berakhir data effective-dated (harga khusus, harga bertingkat, batas diskon). Periode yang sudah
 * lewat tidak diubah; paling cepat kemarin (berhenti berlaku hari ini).
 */
export function EndPeriodDialog(props: EndPeriodDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <EndPeriodForm {...props} /> : null}
    </Dialog>
  )
}

function EndPeriodForm({
  onOpenChange,
  resource,
  item,
  title,
  queryKey,
}: EndPeriodDialogProps) {
  const queryClient = useQueryClient()
  const [validTo, setValidTo] = useState("")
  const [error, setError] = useState<string | null>(null)
  const end = useIdempotentMutation({
    mutationFn: (to: string, key) =>
      endDated(resource, item.id, to, item.version, key),
    onSuccess: async () => {
      toast.success("Tanggal berakhir disimpan")
      await queryClient.invalidateQueries({ queryKey })
      onOpenChange(false)
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.code === "VALIDATION_FAILED" && p.fields?.validTo) {
        setError(p.fields.validTo)
        return
      }
      showErrorDialog(p)
    },
  })
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>
          Berlaku sejak {formatDate(item.validFrom)}. Tanggal berakhir adalah
          hari terakhir data ini dipakai; transaksi yang sudah dibuat tidak
          berubah.
        </DialogDescription>
      </DialogHeader>
      <Field data-invalid={!!error}>
        <FieldLabel htmlFor="end-valid-to">Berlaku sampai</FieldLabel>
        <DateInput
          id="end-valid-to"
          value={validTo}
          onValueChange={setValidTo}
          aria-invalid={!!error}
        />
        <FieldError errors={error ? [{ message: error }] : []} />
      </Field>
      <DialogFooter>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          Batal
        </Button>
        <LoadingButton
          loading={end.isPending}
          loadingText="Menyimpan…"
          onClick={() => {
            if (!validTo) {
              setError("Tanggal berakhir wajib diisi")
              return
            }
            end.mutate(validTo)
          }}
        >
          Simpan Tanggal Berakhir
        </LoadingButton>
      </DialogFooter>
    </DialogContent>
  )
}
