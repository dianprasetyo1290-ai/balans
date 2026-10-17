import { useQueryClient } from "@tanstack/react-query"
import { useState, type ReactNode } from "react"

import { FileUpload } from "@/components/common/file-upload"
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { toast } from "sonner"

import {
  MAX_RECEIPT_BYTES,
  RECEIPT_ACCEPT,
  requestReceiptUpload,
  type CashBankAccount,
} from "../api/finance-api"

/** Pasangan label–nilai di kepala dokumen. */
export function Info({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="break-words">{value}</dd>
    </div>
  )
}

/** Error field di bawah input. */
export function fieldErrors(message?: string) {
  return message ? [{ message }] : []
}

/** Invalidate semua query keuangan (dan jurnal) setelah mutasi. */
export function useFinanceRefresh() {
  const queryClient = useQueryClient()
  return async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["finance"] }),
      queryClient.invalidateQueries({ queryKey: ["accounting"] }),
    ])
  }
}

export type SelectOption = { value: string; label: string }

/** Select sederhana dari daftar nilai–label. */
export function SimpleSelect({
  id,
  value,
  onValueChange,
  options,
  placeholder,
  className = "w-full",
  ...aria
}: {
  id?: string
  value: string
  onValueChange: (value: string) => void
  options: SelectOption[]
  placeholder: string
  className?: string
  "aria-invalid"?: boolean
  "aria-label"?: string
}) {
  return (
    <Select value={value} onValueChange={(v) => onValueChange(String(v ?? ""))}>
      <SelectTrigger
        id={id}
        className={className}
        aria-invalid={aria["aria-invalid"]}
        aria-label={aria["aria-label"]}
      >
        <SelectValue>
          {(v: string) =>
            options.find((o) => o.value === v)?.label ?? placeholder
          }
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.value} value={o.value}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** Pilihan rekening kas/bank aktif. */
export function bankOptions(
  accounts: CashBankAccount[] | undefined,
  bankOnly = false
): SelectOption[] {
  return (accounts ?? [])
    .filter((a) => a.isActive && (!bankOnly || a.kind === "bank"))
    .map((a) => ({
      value: a.id,
      label: `${a.name}${a.accountNumberMasked ? ` ${a.accountNumberMasked}` : ""} · ${a.accountCode}`,
    }))
}

/** Unggah bukti (PDF/foto) untuk pengajuan biaya, kas kecil, atau pembayaran. */
export function ReceiptUpload({
  ownerType,
  onUploaded,
  onRemoved,
  label = "Lampirkan Bukti",
}: {
  ownerType: "expense_claim" | "petty_cash_transaction" | "payment"
  onUploaded: (fileId: string) => void
  onRemoved?: () => void
  label?: string
}) {
  return (
    <FileUpload
      label={label}
      accept={RECEIPT_ACCEPT}
      maxSizeBytes={MAX_RECEIPT_BYTES}
      requestUpload={(file) => requestReceiptUpload(ownerType, file)}
      onUploaded={(id) => onUploaded(id)}
      onRemoved={onRemoved}
      onError={(message) => toast.error(message)}
    />
  )
}

/** Dialog alasan wajib untuk pembatalan dokumen keuangan. */
export function ReasonDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  loading,
  error,
  onConfirm,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel: string
  loading: boolean
  error?: string
  onConfirm: (reason: string) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <ReasonForm
          title={title}
          description={description}
          confirmLabel={confirmLabel}
          loading={loading}
          error={error}
          onCancel={() => onOpenChange(false)}
          onConfirm={onConfirm}
        />
      ) : null}
    </Dialog>
  )
}

function ReasonForm({
  title,
  description,
  confirmLabel,
  loading,
  error,
  onCancel,
  onConfirm,
}: {
  title: string
  description: string
  confirmLabel: string
  loading: boolean
  error?: string
  onCancel: () => void
  onConfirm: (reason: string) => void
}) {
  const [reason, setReason] = useState("")
  const [local, setLocal] = useState<string>()
  const message = local ?? error
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!reason.trim()) {
            setLocal("Alasan wajib diisi")
            return
          }
          setLocal(undefined)
          onConfirm(reason.trim())
        }}
      >
        <Field data-invalid={!!message}>
          <FieldLabel htmlFor="reason">Alasan</FieldLabel>
          <Textarea
            id="reason"
            value={reason}
            maxLength={400}
            onChange={(e) => setReason(e.target.value)}
            aria-invalid={!!message}
          />
          <FieldError errors={fieldErrors(message)} />
        </Field>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onCancel}>
            Kembali
          </Button>
          <LoadingButton
            type="submit"
            variant="destructive"
            loading={loading}
            loadingText="Memproses…"
          >
            {confirmLabel}
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

const LOCAL_STATUS: Record<string, { label: string; className: string }> = {
  unmatched: {
    label: "Belum Cocok",
    className: "bg-muted text-muted-foreground",
  },
  suggested: {
    label: "Ada Usulan",
    className: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  },
  matched: {
    label: "Cocok",
    className: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  },
  recorded: {
    label: "Belum Disetor",
    className: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  },
  deposited: {
    label: "Disetor",
    className: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  },
}

/** Badge status khusus rekonsiliasi bank dan bukti potong; status dokumen umum memakai StatusBadge. */
export function FinanceStatusBadge({ status }: { status: string }) {
  const spec = LOCAL_STATUS[status]
  if (!spec) {
    return <StatusBadge status={status} />
  }
  return (
    <Badge
      variant="outline"
      className={`border-transparent ${spec.className}`}
      data-status={status}
    >
      {spec.label}
    </Badge>
  )
}

/** Tahun pilihan filter pajak (tahun berjalan dan 3 tahun sebelumnya). */
export function yearOptions(current: number): SelectOption[] {
  return [0, 1, 2, 3].map((d) => ({
    value: String(current - d),
    label: String(current - d),
  }))
}
