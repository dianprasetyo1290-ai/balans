import { useQuery, useQueryClient } from "@tanstack/react-query"
import { SearchIcon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { StatusBadge } from "@/components/common/status-badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
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
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { ApprovalSteps } from "@/features/approval/components/approval-steps"
import { EntityHistory } from "@/features/audit/components/entity-history"
import type { StockBalance } from "@/features/inventory/api/inventory-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  DOC_STATUSES,
  docLocationsQuery,
  locationStockQuery,
  subjectApprovalsQuery,
  type InventoryDocAction,
  type InventoryDocMovement,
  type InventoryDocStatus,
  type InventoryDocStep,
} from "../api/documents-api"
import { StockStatusBadge } from "./stock-filters"

const ALL = "all"

/** Satu pasangan label–nilai di kepala dokumen. */
export function Info({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value || "—"}</dd>
    </div>
  )
}

/** Pelaku dan waktu satu langkah dokumen. */
export function stepText(s?: InventoryDocStep | null) {
  return s ? `${s.name || "Sistem"} · ${formatDateTime(s.at)}` : "—"
}

/** Kepala halaman detail: nomor, status, dan tombol aksi. */
export function DocHeader({
  title,
  status,
  subtitle,
  children,
}: {
  title: string
  status: InventoryDocStatus
  subtitle?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
          <StatusBadge status={status} />
        </div>
        {subtitle ? (
          <p className="text-sm text-muted-foreground">{subtitle}</p>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2 [&_button]:min-h-11 sm:[&_button]:min-h-9">
        {children}
      </div>
    </div>
  )
}

export type ActionSpec<A extends string> = {
  action: A
  label: string
  confirmTitle: string
  confirmDescription: string
  destructive?: boolean
  primary?: boolean
}

/** Tombol aksi dokumen sesuai allowedActions, masing-masing dengan dialog konfirmasi. */
export function DocActionButtons<A extends string>({
  specs,
  allowed,
  run,
  onDone,
}: {
  specs: ActionSpec<A>[]
  allowed: InventoryDocAction[]
  run: (action: A, key: string) => Promise<unknown>
  onDone?: () => void
}) {
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<ActionSpec<A> | null>(null)
  const mutation = useIdempotentMutation({
    mutationFn: (action: A, key) => run(action, key),
    onSuccess: async () => {
      toast.success(`${confirm?.label ?? "Aksi"} berhasil`)
      setConfirm(null)
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
      await queryClient.invalidateQueries({ queryKey: ["approval"] })
      onDone?.()
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })
  return (
    <>
      {specs
        .filter((s) => allowed.includes(s.action as InventoryDocAction))
        .map((s) => (
          <Button
            key={s.action}
            variant={
              s.destructive ? "destructive" : s.primary ? "default" : "outline"
            }
            onClick={() => setConfirm(s)}
          >
            {s.label}
          </Button>
        ))}
      {confirm ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirm(null)}
          title={confirm.confirmTitle}
          description={confirm.confirmDescription}
          confirmLabel={confirm.label}
          destructive={confirm.destructive}
          loading={mutation.isPending}
          onConfirm={() => mutation.mutate(confirm.action)}
        />
      ) : null}
    </>
  )
}

/** Panel Persetujuan: langkah approval terbaru dokumen (Desain UI/UX 3.3). */
export function DocApprovalPanel({
  subjectType,
  subjectId,
}: {
  subjectType: string
  subjectId: string
}) {
  const requests = useQuery(subjectApprovalsQuery(subjectType, subjectId))
  const items = requests.data?.items ?? []
  return (
    <LoadingArea loading={requests.isPending}>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Dokumen belum diajukan untuk persetujuan.
        </p>
      ) : (
        <div className="flex flex-col gap-6">
          {items.map((r) => (
            <section key={r.id} className="flex flex-col gap-3">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <StatusBadge status={r.status} />
                <span className="text-muted-foreground">
                  Diajukan {r.requestedByName} · {formatDateTime(r.createdAt)} ·{" "}
                  {formatRupiah(r.amount)}
                </span>
              </div>
              <ApprovalSteps steps={r.steps} />
            </section>
          ))}
        </div>
      )}
    </LoadingArea>
  )
}

export type DocTab = "detail" | "approval" | "history"

/** Tab Rincian / Persetujuan / Riwayat di detail dokumen. */
export function DocTabs({
  tab,
  onTabChange,
  withApproval,
  entityType,
  entityId,
  children,
  approvalSubject,
}: {
  tab?: DocTab
  onTabChange: (t: DocTab) => void
  withApproval?: boolean
  entityType: string
  entityId: string
  approvalSubject?: string
  children: ReactNode
}) {
  const tabs: { id: DocTab; label: string }[] = [
    { id: "detail", label: "Rincian" },
  ]
  if (withApproval) tabs.push({ id: "approval", label: "Persetujuan" })
  tabs.push({ id: "history", label: "Riwayat" })
  return (
    <div className="flex flex-col gap-4">
      <PageTabs
        tabs={tabs}
        value={tab ?? "detail"}
        onValueChange={onTabChange}
        label="Bagian dokumen"
      />
      {(tab ?? "detail") === "detail" ? children : null}
      {tab === "approval" && withApproval ? (
        <DocApprovalPanel
          subjectType={approvalSubject ?? entityType}
          subjectId={entityId}
        />
      ) : null}
      {tab === "history" ? (
        <EntityHistory entityType={entityType} entityId={entityId} />
      ) : null}
    </div>
  )
}

/** Tabel mutasi stok dokumen: batch yang dialokasikan, dikirim, atau keluar. */
export function DocMovementsTable({
  movements,
}: {
  movements: InventoryDocMovement[]
}) {
  if (movements.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">Belum ada mutasi stok.</p>
    )
  }
  return (
    <div className="overflow-x-auto rounded-lg border">
      <Table className="min-w-[48rem]">
        <TableHeader>
          <TableRow>
            <TableHead>Jenis</TableHead>
            <TableHead>Produk</TableHead>
            <TableHead>Lokasi</TableHead>
            <TableHead>Batch</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Qty</TableHead>
            <TableHead className="text-right">Nilai HPP</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {movements.map((m) => (
            <TableRow key={m.id}>
              <TableCell>{m.movementTypeLabel}</TableCell>
              <TableCell>
                <span className="tabular-nums">{m.product.sku}</span>{" "}
                {m.product.name}
              </TableCell>
              <TableCell>{m.location.code}</TableCell>
              <TableCell>
                {m.batch ? (
                  <span>
                    {m.batch.number}
                    {m.batch.expiryDate
                      ? ` · ${formatDate(m.batch.expiryDate)}`
                      : ""}
                  </span>
                ) : (
                  "—"
                )}
              </TableCell>
              <TableCell>
                <StockStatusBadge status={m.status} />
              </TableCell>
              <TableCell className="text-right tabular-nums">{m.qty}</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRupiah(m.costAmount)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}

/** Pencarian nomor + filter status untuk daftar dokumen. */
export function DocListFilters({
  q,
  status,
  statuses = DOC_STATUSES,
  onSearch,
  onStatus,
  children,
}: {
  q?: string
  status?: InventoryDocStatus
  statuses?: InventoryDocStatus[]
  onSearch: (q: string | undefined) => void
  onStatus: (s: InventoryDocStatus | undefined) => void
  children?: ReactNode
}) {
  const [text, setText] = useState(q ?? "")
  return (
    <div className="flex flex-wrap items-center gap-3">
      <form
        className="relative w-full sm:w-72"
        onSubmit={(e) => {
          e.preventDefault()
          onSearch(text || undefined)
        }}
      >
        <SearchIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          aria-label="Cari dokumen"
          placeholder="Cari nomor atau catatan"
          className="pl-8"
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
      </form>
      <Select
        value={status ?? ALL}
        onValueChange={(v) =>
          onStatus(!v || v === ALL ? undefined : (v as InventoryDocStatus))
        }
      >
        <SelectTrigger className="w-48" aria-label="Filter status">
          <SelectValue>
            {(v: string) => (v === ALL ? "Semua status" : statusLabel(v).label)}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL}>Semua status</SelectItem>
          {statuses.map((s) => (
            <SelectItem key={s} value={s}>
              {statusLabel(s).label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {children}
    </div>
  )
}

/** Pemilih lokasi aktif berjenis tertentu (mis. FG Non-Ecommerce, Retur, Event). */
export function LocationSelect({
  id,
  label,
  value,
  onChange,
  types,
  invalid,
}: {
  id: string
  label: string
  value?: string
  onChange: (id: string) => void
  types: string[]
  invalid?: boolean
}) {
  const locations = useQuery(docLocationsQuery)
  const items = (locations.data?.items ?? []).filter(
    (l) =>
      l.isActive &&
      !l.deletedAt &&
      (types.length === 0 || types.includes(l.locationType))
  )
  return (
    <Field data-invalid={invalid}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select
        value={value ?? ""}
        onValueChange={(v) => v && onChange(String(v))}
      >
        <SelectTrigger id={id} className="w-full" aria-invalid={invalid}>
          <SelectValue>
            {(v: string) => {
              const l = items.find((x) => x.id === v)
              return l ? `${l.code} · ${l.name}` : "Pilih lokasi"
            }}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {items.map((l) => (
            <SelectItem key={l.id} value={l.id}>
              {l.code} · {l.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  )
}

/** Dialog memilih saldo stok (produk × batch × status) di satu lokasi untuk ditambahkan ke baris dokumen. */
export function StockPickerDialog({
  open,
  onOpenChange,
  locationId,
  onPick,
  title = "Pilih Barang dari Stok",
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  locationId?: string
  onPick: (b: StockBalance) => void
  title?: string
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>
              Saldo Tersedia dan Karantina di lokasi terpilih.
            </DialogDescription>
          </DialogHeader>
          {locationId ? (
            <StockPickerTable
              locationId={locationId}
              onPick={(b) => {
                onPick(b)
                onOpenChange(false)
              }}
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              Pilih lokasi terlebih dahulu.
            </p>
          )}
        </DialogContent>
      ) : null}
    </Dialog>
  )
}

function StockPickerTable({
  locationId,
  onPick,
}: {
  locationId: string
  onPick: (b: StockBalance) => void
}) {
  const stock = useQuery(locationStockQuery(locationId))
  const items = stock.data?.items ?? []
  return (
    <LoadingArea loading={stock.isPending}>
      {items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Tidak ada stok di lokasi ini.
        </p>
      ) : (
        <div className="max-h-[60vh] overflow-auto rounded-lg border">
          <Table className="min-w-[40rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead>Batch</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Qty</TableHead>
                <TableHead />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((b) => (
                <TableRow
                  key={`${b.product.id}-${b.batchId ?? ""}-${b.status}`}
                >
                  <TableCell>
                    <span className="tabular-nums">{b.product.sku}</span>{" "}
                    {b.product.name}
                  </TableCell>
                  <TableCell>
                    {b.batchNumber ?? "—"}
                    {b.expiryDate ? ` · ${formatDate(b.expiryDate)}` : ""}
                  </TableCell>
                  <TableCell>
                    <StockStatusBadge status={b.status} />
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {b.qty}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => onPick(b)}
                    >
                      Pilih
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </LoadingArea>
  )
}

/** Pemilih dari daftar pilihan sederhana. */
export function OptionSelect({
  id,
  label,
  value,
  onChange,
  options,
  invalid,
  placeholder = "Pilih",
}: {
  id: string
  label: string
  value?: string
  onChange: (v: string) => void
  options: { value: string; label: string }[]
  invalid?: boolean
  placeholder?: string
}) {
  return (
    <Field data-invalid={invalid}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select
        value={value ?? ""}
        onValueChange={(v) => v && onChange(String(v))}
      >
        <SelectTrigger id={id} className="w-full" aria-invalid={invalid}>
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
    </Field>
  )
}

/** Menangani error mutasi form: field error ke state, selain itu dialog error. */
export function handleFormError(
  err: unknown,
  setErrors: (e: Record<string, string>) => void
) {
  const problem = parseProblem(err)
  if (problem.fields) {
    setErrors(problem.fields)
    return
  }
  showErrorDialog(problem)
}

/** Pesan error field form dari problem+json. */
export function fieldErrors(errors: Record<string, string>, key: string) {
  return errors[key] ? [{ message: errors[key] }] : []
}
