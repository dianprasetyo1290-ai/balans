import { useQuery } from "@tanstack/react-query"
import { useState } from "react"

import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
import { StatusBadge } from "@/components/common/status-badge"
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
import { ApprovalSteps } from "@/features/approval/components/approval-steps"

import {
  subjectApprovalsQuery,
  supplierOptionsQuery,
  type PurchaseOption,
} from "../api/procurement-api"

/** Pasangan label–nilai di kepala dokumen. */
export function Info({
  label,
  value,
}: {
  label: string
  value: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="break-words">{value}</dd>
    </div>
  )
}

/** Pesan error per baris tabel form. */
export function LineError({ message }: { message?: string }) {
  return message ? (
    <p className="mt-1 text-xs text-destructive">{message}</p>
  ) : null
}

/** Error field di bawah input. */
export function fieldErrors(message?: string) {
  return message ? [{ message }] : []
}

type OptionSelectProps = {
  id?: string
  value: string
  onValueChange: (value: string) => void
  options: PurchaseOption[]
  placeholder: string
  showCode?: boolean
  "aria-invalid"?: boolean
  "aria-label"?: string
}

/** Pilihan divisi atau akun dari daftar kode–nama. */
export function OptionSelect({
  id,
  value,
  onValueChange,
  options,
  placeholder,
  showCode = false,
  ...aria
}: OptionSelectProps) {
  const text = (o: PurchaseOption) =>
    showCode ? `${o.code} ${o.name}` : o.name
  return (
    <Select value={value} onValueChange={(v) => onValueChange(String(v ?? ""))}>
      <SelectTrigger
        id={id}
        className="w-full"
        aria-invalid={aria["aria-invalid"]}
        aria-label={aria["aria-label"]}
      >
        <SelectValue>
          {(v: string) => {
            const o = options.find((x) => x.id === v || x.code === v)
            return o ? text(o) : placeholder
          }}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o.id} value={showCode ? o.code : o.id}>
            {text(o)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

type SupplierPickerProps = {
  id?: string
  value: PickerOption | null
  onValueChange: (value: PickerOption | null) => void
  tollOnly: boolean
  "aria-invalid"?: boolean
}

/** Cari Pemasok; PO maklon/reprocess hanya menampilkan pemasok berjenis maklon. */
export function SupplierPicker({ tollOnly, ...props }: SupplierPickerProps) {
  return (
    <SearchPicker
      {...props}
      query={(q) => supplierOptionsQuery(q, tollOnly)}
      toOption={(s) => ({
        id: s.id,
        label: s.name,
        description: `${s.code} · termin ${s.paymentTermDays} hari`,
      })}
      placeholder={tollOnly ? "Pilih maklon" : "Pilih pemasok"}
      searchPlaceholder="Cari kode atau nama pemasok"
    />
  )
}

/** Panel Persetujuan di detail dokumen: langkah per level dari engine approval (Desain UI/UX 3.3). */
export function ApprovalPanel({
  subjectType,
  subjectId,
}: {
  subjectType:
    | "purchase_request"
    | "purchase_order"
    | "supplier_bill_exception"
    | "purchase_return"
  subjectId: string
}) {
  const approvals = useQuery(subjectApprovalsQuery(subjectType, subjectId))
  const requests = approvals.data?.items ?? []
  return (
    <section className="flex flex-col gap-3 rounded-lg border p-4">
      <h2 className="text-sm font-semibold">Persetujuan</h2>
      <LoadingArea loading={approvals.isPending} label="Memuat persetujuan">
        {approvals.isError ? (
          <p className="text-sm text-muted-foreground">
            Riwayat persetujuan tidak bisa dimuat.
          </p>
        ) : requests.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Belum diajukan untuk persetujuan.
          </p>
        ) : (
          <ol className="flex flex-col gap-4">
            {requests.map((r, i) => (
              <li key={r.id} className="flex flex-col gap-2">
                {requests.length > 1 ? (
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <span>Pengajuan {requests.length - i}</span>
                    <StatusBadge status={r.status} />
                  </div>
                ) : null}
                <ApprovalSteps steps={r.steps} />
              </li>
            ))}
          </ol>
        )}
      </LoadingArea>
    </section>
  )
}

type ReasonDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description: string
  confirmLabel: string
  required: boolean
  loading: boolean
  destructive?: boolean
  onConfirm: (reason: string) => void
}

/** Dialog aksi dengan alasan (tutup, batalkan). */
export function ReasonDialog(props: ReasonDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ReasonForm {...props} /> : null}
    </Dialog>
  )
}

function ReasonForm({
  onOpenChange,
  title,
  description,
  confirmLabel,
  required,
  loading,
  destructive,
  onConfirm,
}: ReasonDialogProps) {
  const [reason, setReason] = useState("")
  const [error, setError] = useState<string>()
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
          if (required && !reason.trim()) {
            setError("Alasan wajib diisi")
            return
          }
          onConfirm(reason.trim())
        }}
      >
        <Field data-invalid={!!error}>
          <FieldLabel htmlFor="action-reason">
            Alasan{required ? "" : " (opsional)"}
          </FieldLabel>
          <Textarea
            id="action-reason"
            value={reason}
            maxLength={400}
            aria-invalid={!!error}
            onChange={(e) => setReason(e.target.value)}
          />
          <FieldError errors={fieldErrors(error)} />
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
            loading={loading}
            loadingText="Memproses…"
            variant={destructive ? "destructive" : "default"}
          >
            {confirmLabel}
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
