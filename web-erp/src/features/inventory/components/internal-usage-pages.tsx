import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { QuantityInput } from "@/components/common/quantity-input"
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { usePermissions } from "@/features/auth/api/auth-api"
import type { ProductOption } from "@/features/catalog/components/product-picker"
import { ProductPicker } from "@/features/catalog/components/product-picker"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import {
  createInternalUsage,
  dimensionOptionsQuery,
  internalUsageQuery,
  internalUsagesQuery,
  PURPOSE_LABELS,
  runInternalUsageAction,
  settleInternalUsage,
  updateInternalUsage,
  type InternalUsage,
  type InternalUsageAction,
  type InternalUsageInput,
  type InternalUsagePurpose,
} from "../api/documents-api"
import {
  DocActionButtons,
  DocHeader,
  DocListFilters,
  DocMovementsTable,
  DocTabs,
  fieldErrors,
  handleFormError,
  Info,
  LocationSelect,
  OptionSelect,
  stepText,
  type ActionSpec,
  type DocTab,
} from "./doc-shared"

const listRoute = getRouteApi("/_app/inventory/internal-usages/")
const detailRoute = getRouteApi("/_app/inventory/internal-usages/$id/")

/** Persediaan › Pemakaian Internal: daftar permintaan FG untuk marketing, event, sampel karyawan, sampling sales. */
export function InternalUsageListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const usages = useQuery(internalUsagesQuery(search))
  const columns: DataTableColumn<InternalUsage>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (u) => (
        <Link
          to="/inventory/internal-usages/$id"
          params={{ id: u.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {u.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "date",
      header: "Tanggal",
      cell: (u) => (
        <span className="whitespace-nowrap">{formatDate(u.businessDate)}</span>
      ),
    },
    { id: "purpose", header: "Tujuan", cell: (u) => u.purposeLabel },
    {
      id: "division",
      header: "Divisi / Kampanye",
      cell: (u) => (
        <div className="flex flex-col">
          <span>{u.division.name}</span>
          {u.campaign ? (
            <span className="text-xs text-muted-foreground">
              {u.campaign.code}
            </span>
          ) : null}
        </div>
      ),
    },
    { id: "requester", header: "Peminta", cell: (u) => u.requestedBy.name },
    {
      id: "value",
      header: "Nilai HPP",
      align: "right",
      cell: (u) =>
        formatRupiah(u.status === "posted" ? u.costAmount : u.estimatedCost),
    },
    {
      id: "status",
      header: "Status",
      cell: (u) => <StatusBadge status={u.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Pemakaian Internal
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Permintaan FG untuk marketing, event, sampel karyawan, dan sampling
            sales. Biaya dijurnal sebesar HPP atas qty terpakai.
          </p>
        </div>
        {can("inventory.internal_usage.write") ? (
          <Button render={<Link to="/inventory/internal-usages/new" />}>
            <PlusIcon /> Buat Pemakaian Internal
          </Button>
        ) : null}
      </div>
      <DocListFilters
        q={search.q}
        status={search.status}
        onSearch={(q) =>
          void navigate({ search: (s) => ({ ...s, q, page: 1 }) })
        }
        onStatus={(status) =>
          void navigate({ search: (s) => ({ ...s, status, page: 1 }) })
        }
      />
      <DataTable
        columns={columns}
        rows={usages.data?.items}
        getRowId={(u) => u.id}
        loading={usages.isPending}
        fetching={usages.isFetching}
        page={search.page}
        totalPages={usages.data?.totalPages ?? 0}
        totalItems={usages.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada Pemakaian Internal."
      />
    </div>
  )
}

type UsageLine = { product: ProductOption | null; qty: string; note: string }

/** Form Buat/Ubah Pemakaian Internal. */
export function InternalUsageFormPage({ id }: { id?: string }) {
  const usage = useQuery({
    ...internalUsageQuery(id ?? ""),
    enabled: Boolean(id),
  })
  if (id && !usage.data) {
    return <LoadingArea loading>{null}</LoadingArea>
  }
  return <InternalUsageForm usage={usage.data} />
}

function InternalUsageForm({ usage }: { usage?: InternalUsage }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const divisions = useQuery(dimensionOptionsQuery("division"))
  const campaigns = useQuery(dimensionOptionsQuery("campaign"))
  const [purpose, setPurpose] = useState<InternalUsagePurpose>(
    usage?.purpose ?? "marketing"
  )
  const [divisionId, setDivisionId] = useState(usage?.division.id ?? "")
  const [campaignId, setCampaignId] = useState(usage?.campaign?.id ?? "")
  const [sourceLocationId, setSource] = useState(usage?.sourceLocation.id ?? "")
  const [eventLocationId, setEventLocation] = useState(
    usage?.eventLocation?.id ?? ""
  )
  const [eventEndDate, setEventEnd] = useState(usage?.eventEndDate ?? "")
  const [businessDate, setBusinessDate] = useState(
    usage?.businessDate ?? todayBusinessDate()
  )
  const [note, setNote] = useState(usage?.note ?? "")
  const [lines, setLines] = useState<UsageLine[]>(
    usage?.lines.map((l) => ({
      product: { ...l.product, variant: "" },
      qty: l.qty,
      note: l.note,
    })) ?? [{ product: null, qty: "", note: "" }]
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const isEvent = purpose === "event"

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: InternalUsageInput = {
        purpose,
        divisionId,
        campaignId:
          campaignId && purpose !== "employee_sample" ? campaignId : undefined,
        sourceLocationId,
        eventLocationId:
          isEvent && eventLocationId ? eventLocationId : undefined,
        eventEndDate: isEvent && eventEndDate ? eventEndDate : undefined,
        businessDate,
        note: note || undefined,
        lines: lines.map((l) => ({
          productId: l.product?.id ?? "",
          qty: l.qty || "0",
          note: l.note || undefined,
        })),
      }
      return usage
        ? updateInternalUsage(
            usage.id,
            { ...body, version: usage.version },
            key
          )
        : createInternalUsage(body, key)
    },
    onSuccess: async (u) => {
      toast.success("Pemakaian Internal disimpan")
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
      void navigate({
        to: "/inventory/internal-usages/$id",
        params: { id: u.id },
      })
    },
    onError: (err) => handleFormError(err, setErrors),
  })

  const opts = (items?: { id: string; code: string; name: string }[]) =>
    (items ?? []).map((d) => ({ value: d.id, label: `${d.code} · ${d.name}` }))

  return (
    <form
      noValidate
      className="flex max-w-4xl flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        setErrors({})
        save.mutate()
      }}
    >
      <h1 className="text-2xl font-semibold tracking-tight">
        {usage
          ? `Ubah ${usage.number ?? "Draft Pemakaian Internal"}`
          : "Buat Pemakaian Internal"}
      </h1>
      <div className="grid gap-4 sm:grid-cols-2">
        <OptionSelect
          id="usage-purpose"
          label="Tujuan"
          value={purpose}
          onChange={(v) => setPurpose(v as InternalUsagePurpose)}
          options={Object.entries(PURPOSE_LABELS).map(([value, label]) => ({
            value,
            label,
          }))}
          invalid={!!errors.purpose}
        />
        <Field data-invalid={!!errors.businessDate}>
          <FieldLabel htmlFor="usage-date">Tanggal</FieldLabel>
          <DateInput
            id="usage-date"
            value={businessDate}
            onValueChange={setBusinessDate}
          />
          <FieldError errors={fieldErrors(errors, "businessDate")} />
        </Field>
        <div>
          <OptionSelect
            id="usage-division"
            label="Divisi"
            value={divisionId}
            onChange={setDivisionId}
            options={opts(divisions.data?.items)}
            invalid={!!errors.divisionId}
            placeholder="Pilih divisi"
          />
          <FieldError errors={fieldErrors(errors, "divisionId")} />
        </div>
        {purpose !== "employee_sample" ? (
          <div>
            <OptionSelect
              id="usage-campaign"
              label="Kode Kampanye"
              value={campaignId}
              onChange={setCampaignId}
              options={opts(campaigns.data?.items)}
              invalid={!!errors.campaignId}
              placeholder="Pilih kampanye"
            />
            <FieldError errors={fieldErrors(errors, "campaignId")} />
          </div>
        ) : null}
        <div>
          <LocationSelect
            id="usage-source"
            label="Lokasi Asal"
            value={sourceLocationId}
            onChange={setSource}
            types={["finished_goods", "ecommerce", "petty_stock"]}
            invalid={!!errors.sourceLocationId}
          />
          <FieldError errors={fieldErrors(errors, "sourceLocationId")} />
        </div>
        {isEvent ? (
          <>
            <div>
              <LocationSelect
                id="usage-event-location"
                label="Lokasi Event"
                value={eventLocationId}
                onChange={setEventLocation}
                types={["event"]}
                invalid={!!errors.eventLocationId}
              />
              <FieldError errors={fieldErrors(errors, "eventLocationId")} />
            </div>
            <Field data-invalid={!!errors.eventEndDate}>
              <FieldLabel htmlFor="usage-event-end">
                Tanggal Selesai Event
              </FieldLabel>
              <DateInput
                id="usage-event-end"
                value={eventEndDate}
                onValueChange={setEventEnd}
              />
              <FieldError errors={fieldErrors(errors, "eventEndDate")} />
            </Field>
          </>
        ) : null}
      </div>
      <Field data-invalid={!!errors.note}>
        <FieldLabel htmlFor="usage-note">Catatan</FieldLabel>
        <Textarea
          id="usage-note"
          value={note}
          maxLength={500}
          onChange={(e) => setNote(e.target.value)}
        />
        <FieldError errors={fieldErrors(errors, "note")} />
      </Field>
      <div className="flex flex-col gap-2">
        <h2 className="font-medium">Barang</h2>
        <FieldError errors={fieldErrors(errors, "lines")} />
        {lines.map((l, i) => (
          <div
            key={i}
            className="grid gap-2 rounded-lg border p-3 sm:grid-cols-[1fr_10rem_auto]"
          >
            <Field data-invalid={!!errors[`lines[${i}].productId`]}>
              <FieldLabel htmlFor={`usage-product-${i}`}>Produk</FieldLabel>
              <ProductPicker
                id={`usage-product-${i}`}
                value={l.product}
                types={["finished_good"]}
                isBundle={false}
                onValueChange={(p) =>
                  setLines((ls) =>
                    ls.map((x, j) => (j === i ? { ...x, product: p } : x))
                  )
                }
              />
              <FieldError
                errors={fieldErrors(errors, `lines[${i}].productId`)}
              />
            </Field>
            <Field data-invalid={!!errors[`lines[${i}].qty`]}>
              <FieldLabel htmlFor={`usage-qty-${i}`}>Qty</FieldLabel>
              <QuantityInput
                id={`usage-qty-${i}`}
                value={l.qty}
                unit={l.product?.uom}
                onValueChange={(v) =>
                  setLines((ls) =>
                    ls.map((x, j) => (j === i ? { ...x, qty: v } : x))
                  )
                }
              />
              <FieldError errors={fieldErrors(errors, `lines[${i}].qty`)} />
            </Field>
            <Button
              type="button"
              variant="ghost"
              className="self-end"
              aria-label={`Hapus baris ${i + 1}`}
              onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))}
            >
              <Trash2Icon />
            </Button>
          </div>
        ))}
        <Button
          type="button"
          variant="outline"
          className="self-start"
          onClick={() =>
            setLines((ls) => [...ls, { product: null, qty: "", note: "" }])
          }
        >
          <PlusIcon /> Tambah Barang
        </Button>
      </div>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => window.history.back()}
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
    </form>
  )
}

const USAGE_ACTIONS: ActionSpec<InternalUsageAction>[] = [
  {
    action: "submit",
    label: "Ajukan",
    primary: true,
    confirmTitle: "Ajukan Pemakaian Internal?",
    confirmDescription:
      "Dokumen mendapat nomor PU dan diajukan ke Head divisi (nilai HPP > Rp5 jt juga ke Head Finance & Accounting).",
  },
  {
    action: "revise",
    label: "Ubah Kembali",
    confirmTitle: "Kembalikan ke draft?",
    confirmDescription: "Dokumen yang ditolak bisa diubah dan diajukan ulang.",
  },
  {
    action: "release",
    label: "Release",
    primary: true,
    confirmTitle: "Release Pemakaian Internal?",
    confirmDescription:
      "Logistik Leader meneruskan permintaan; stok di lokasi asal dialokasikan FEFO untuk disiapkan Staf Gudang.",
  },
  {
    action: "handover",
    label: "Konfirmasi Serah Terima",
    primary: true,
    confirmTitle: "Konfirmasi serah terima?",
    confirmDescription:
      "Pastikan barang sudah dicek ulang. Non-event: stok keluar dan biaya dijurnal sebesar HPP. Event: barang pindah ke lokasi Event tanpa jurnal.",
  },
  {
    action: "post",
    label: "Posting",
    primary: true,
    confirmTitle: "Posting pemakaian event?",
    confirmDescription:
      "Qty terpakai (kirim − sisa) keluar dari lokasi Event dan dijurnal A-11 dengan kode kampanye.",
  },
  {
    action: "cancel",
    label: "Batalkan",
    destructive: true,
    confirmTitle: "Batalkan Pemakaian Internal?",
    confirmDescription: "Dokumen dibatalkan dan tidak bisa diproses lagi.",
  },
]

/** Detail Pemakaian Internal dengan aksi, panel Persetujuan, dan Riwayat. */
export function InternalUsageDetailPage({ id }: { id: string }) {
  const usage = useQuery(internalUsageQuery(id))
  return (
    <LoadingArea loading={usage.isPending}>
      {usage.data ? <InternalUsageDetail usage={usage.data} /> : null}
    </LoadingArea>
  )
}

function InternalUsageDetail({ usage: u }: { usage: InternalUsage }) {
  const search = detailRoute.useSearch()
  const navigate = detailRoute.useNavigate()
  const [settling, setSettling] = useState(false)
  return (
    <div className="flex flex-col gap-4">
      <DocHeader
        title={u.number ?? "Draft Pemakaian Internal"}
        status={u.status}
        subtitle={`${u.purposeLabel} · ${u.division.name}${u.campaign ? ` · ${u.campaign.code}` : ""}`}
      >
        {u.allowedActions.includes("edit") ? (
          <Button
            variant="outline"
            render={
              <Link
                to="/inventory/internal-usages/$id/edit"
                params={{ id: u.id }}
              />
            }
          >
            Ubah
          </Button>
        ) : null}
        {u.allowedActions.includes("settle") ? (
          <Button onClick={() => setSettling(true)}>Catat Sisa Event</Button>
        ) : null}
        <DocActionButtons
          specs={USAGE_ACTIONS}
          allowed={u.allowedActions}
          run={(a, key) => runInternalUsageAction(u.id, a, u.version, key)}
        />
      </DocHeader>
      <DocTabs
        tab={search.tab}
        onTabChange={(tab: DocTab) => void navigate({ search: { tab } })}
        withApproval={!u.returnDispositionId}
        entityType="internal_usage"
        entityId={u.id}
      >
        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <Info label="Tanggal" value={formatDate(u.businessDate)} />
          <Info
            label="Lokasi Asal"
            value={`${u.sourceLocation.code} · ${u.sourceLocation.name}`}
          />
          {u.eventLocation ? (
            <Info
              label="Lokasi Event"
              value={`${u.eventLocation.code} · ${u.eventLocation.name}`}
            />
          ) : null}
          {u.eventEndDate ? (
            <Info label="Selesai Event" value={formatDate(u.eventEndDate)} />
          ) : null}
          {u.settleDueDate ? (
            <Info
              label="Batas Catat Sisa (H+1)"
              value={formatDate(u.settleDueDate)}
            />
          ) : null}
          <Info label="Peminta" value={u.requestedBy.name} />
          <Info label="Estimasi HPP" value={formatRupiah(u.estimatedCost)} />
          <Info label="HPP Terpakai" value={formatRupiah(u.costAmount)} />
          <Info label="Release" value={stepText(u.released)} />
          <Info label="Serah Terima" value={stepText(u.handedOver)} />
          {u.purpose === "event" ? (
            <Info label="Sisa Dicatat" value={stepText(u.settled)} />
          ) : null}
          <Info label="Diposting" value={stepText(u.posted)} />
          {u.journalId ? (
            <Info
              label="Jurnal"
              value={
                <Link
                  to="/finance/journals/$id"
                  params={{ id: u.journalId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat jurnal
                </Link>
              }
            />
          ) : null}
          {u.note ? <Info label="Catatan" value={u.note} /> : null}
        </dl>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[44rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead className="text-right">Qty Kirim</TableHead>
                <TableHead className="text-right">Qty Sisa</TableHead>
                <TableHead className="text-right">Qty Terpakai</TableHead>
                <TableHead className="text-right">HPP</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {u.lines.map((l) => (
                <TableRow key={l.id}>
                  <TableCell>
                    <span className="tabular-nums">{l.product.sku}</span>{" "}
                    {l.product.name}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {l.qty}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {l.returnedQty}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {l.consumedQty}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(l.costAmount)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
        <h2 className="font-medium">Mutasi Stok</h2>
        <DocMovementsTable movements={u.movements} />
      </DocTabs>
      <SettleDialog usage={u} open={settling} onOpenChange={setSettling} />
    </div>
  )
}

function SettleDialog({
  usage,
  open,
  onOpenChange,
}: {
  usage: InternalUsage
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <SettleForm usage={usage} onOpenChange={onOpenChange} /> : null}
    </Dialog>
  )
}

function SettleForm({
  usage,
  onOpenChange,
}: {
  usage: InternalUsage
  onOpenChange: (o: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [returned, setReturned] = useState<Record<string, string>>(
    Object.fromEntries(usage.lines.map((l) => [l.id, ""]))
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const settle = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      settleInternalUsage(
        usage.id,
        usage.version,
        usage.lines.map((l) => ({
          lineId: l.id,
          returnedQty: returned[l.id] || "0",
        })),
        key
      ),
    onSuccess: async () => {
      toast.success("Sisa barang event dicatat dan dikembalikan ke Inbound")
      onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
    },
    onError: (err) => handleFormError(err, setErrors),
  })
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Catat Sisa Barang Event</DialogTitle>
        <DialogDescription>
          Sisa barang dikembalikan ke Inbound berstatus Karantina untuk dicek
          ulang. Biaya hanya atas qty terpakai.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-3"
        onSubmit={(e) => {
          e.preventDefault()
          settle.mutate()
        }}
      >
        {usage.lines.map((l, i) => (
          <Field key={l.id} data-invalid={!!errors[`lines[${i}].returnedQty`]}>
            <FieldLabel htmlFor={`settle-${l.id}`}>
              {l.product.sku} {l.product.name} (dikirim {l.qty})
            </FieldLabel>
            <QuantityInput
              id={`settle-${l.id}`}
              value={returned[l.id] ?? ""}
              unit={l.product.uom}
              onValueChange={(v) => setReturned((r) => ({ ...r, [l.id]: v }))}
            />
            <FieldError
              errors={fieldErrors(errors, `lines[${i}].returnedQty`)}
            />
          </Field>
        ))}
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
            loading={settle.isPending}
            loadingText="Menyimpan…"
          >
            Catat Sisa
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
