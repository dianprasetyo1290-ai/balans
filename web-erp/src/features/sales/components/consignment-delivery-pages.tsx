import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { EntityHistory } from "@/features/audit/components/entity-history"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import type { DetailTab } from "@/features/procurement/components/request-detail-page"
import {
  fieldErrors,
  Info,
  LineError,
  OptionSelect,
  ReasonDialog,
} from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import {
  formatDate,
  formatDateTime,
  todayBusinessDate,
} from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { statusLabel } from "@/lib/format/status-labels"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  consignmentPartnersQuery,
  createDelivery,
  DELIVERY_STATUSES,
  DELIVERY_TYPE_LABELS,
  deliveriesQuery,
  deliveryAction,
  deliveryQuery,
  updateDelivery,
  type ConsignmentDelivery,
  type ConsignmentDeliverySummary,
  type ConsignmentDeliveryType,
  type DeliveryAction,
  type DeliveryListSearch,
} from "../api/consignment-api"
import { locationOptionsQuery } from "../api/sales-documents-api"

const ALL = "all"

/** Penjualan › Konsinyasi: pengiriman barang titipan ke lokasi konsinyasi & buffer vending mitra (S-303, S-219). */
export function ConsignmentDeliveryListPage({
  search,
  onSearchChange,
}: {
  search: DeliveryListSearch
  onSearchChange: (u: (s: DeliveryListSearch) => DeliveryListSearch) => void
}) {
  const list = useQuery(deliveriesQuery(search))
  const [creating, setCreating] = useState(false)
  const columns: DataTableColumn<ConsignmentDeliverySummary>[] = [
    {
      id: "number",
      header: "Nomor DO",
      cell: (d) => (
        <Link
          to="/sales/consignments/$id"
          params={{ id: d.id }}
          search={{}}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {d.number ?? "Draft"}
        </Link>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (d) => DELIVERY_TYPE_LABELS[d.deliveryType],
    },
    {
      id: "partner",
      header: "Mitra",
      cell: (d) => (
        <div className="flex flex-col">
          <span>{d.partnerName}</span>
          <span className="text-xs text-muted-foreground">{d.partnerCode}</span>
        </div>
      ),
    },
    {
      id: "date",
      header: "Tanggal Kirim",
      cell: (d) => formatDate(d.businessDate),
    },
    {
      id: "transfer",
      header: "Transfer",
      cell: (d) => d.transferNumber || "—",
    },
    {
      id: "cost",
      header: "Nilai HPP",
      align: "right",
      cell: (d) => formatRupiah(d.costAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (d) => <StatusBadge status={d.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Konsinyasi</h1>
          <p className="max-w-3xl text-sm text-muted-foreground">
            Pengiriman barang titipan ke lokasi konsinyasi atau buffer vending
            mitra. Hanya bisa jika PKS konsinyasi berlaku; stok tetap milik
            Balans sampai terjual dan ditagih lewat Rekonsiliasi Konsinyasi.
          </p>
        </div>
        <Button onClick={() => setCreating(true)} className="min-h-11">
          <PlusIcon /> Buat Pengiriman
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Select
          value={search.deliveryType ?? ALL}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              deliveryType:
                v === ALL || !v ? undefined : (v as ConsignmentDeliveryType),
              page: 1,
            }))
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter jenis">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua jenis"
                  : DELIVERY_TYPE_LABELS[v as ConsignmentDeliveryType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua jenis</SelectItem>
            <SelectItem value="consignment">Konsinyasi</SelectItem>
            <SelectItem value="vending_buffer">Buffer Vending</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            onSearchChange((s) => ({
              ...s,
              status:
                v === ALL || !v
                  ? undefined
                  : (v as DeliveryListSearch["status"]),
              page: 1,
            }))
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                v === ALL ? "Semua status" : statusLabel(v).label
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {DELIVERY_STATUSES.map((s) => (
              <SelectItem key={s} value={s}>
                {statusLabel(s).label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={list.data?.items}
        getRowId={(d) => d.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 0}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) => onSearchChange((s) => ({ ...s, page }))}
        emptyMessage="Belum ada pengiriman konsinyasi."
      />
      <Dialog open={creating} onOpenChange={setCreating}>
        {creating ? <DeliveryForm onClose={() => setCreating(false)} /> : null}
      </Dialog>
    </div>
  )
}

type FormLine = { key: string; product: ProductOption | null; qty: string }

function DeliveryForm({
  delivery,
  onClose,
}: {
  delivery?: ConsignmentDelivery
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const locations = useQuery(locationOptionsQuery("modern_trade"))
  const [deliveryType, setDeliveryType] = useState<ConsignmentDeliveryType>(
    delivery?.deliveryType ?? "consignment"
  )
  const [partner, setPartner] = useState<PickerOption | null>(
    delivery
      ? {
          id: delivery.partnerId,
          label: delivery.partnerName,
          description: delivery.partnerCode,
        }
      : null
  )
  const [fromLocationId, setFromLocationId] = useState(
    delivery?.fromLocation.id ?? ""
  )
  const [businessDate, setBusinessDate] = useState(
    delivery?.businessDate ?? todayBusinessDate()
  )
  const [note, setNote] = useState(delivery?.note ?? "")
  const [lines, setLines] = useState<FormLine[]>(
    delivery
      ? delivery.lines.map((l) => ({
          key: l.id,
          product: {
            id: l.product.id,
            sku: l.product.sku,
            name: l.product.name,
            uom: l.product.uom,
            variant: "",
          },
          qty: l.qty,
        }))
      : [{ key: crypto.randomUUID(), product: null, qty: "" }]
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        fromLocationId,
        businessDate,
        note: note || undefined,
        lines: lines.map((l) => ({
          productId: l.product?.id ?? "",
          qty: l.qty,
        })),
      }
      return delivery
        ? updateDelivery(delivery.id, delivery.version, body, key)
        : createDelivery(
            { ...body, deliveryType, partnerId: partner?.id ?? "" },
            key
          )
    },
    onSuccess: async (d) => {
      toast.success(
        delivery ? "Pengiriman disimpan" : "Draft pengiriman dibuat"
      )
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
      onClose()
      if (!delivery) {
        void navigate({
          to: "/sales/consignments/$id",
          params: { id: d.id },
          search: {},
        })
      }
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
  const lineErr = (i: number, f: string) => errors[`lines.${i}.${f}`]
  return (
    <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
      <DialogHeader>
        <DialogTitle>
          {delivery ? "Ubah Pengiriman" : "Buat Pengiriman Konsinyasi"}
        </DialogTitle>
        <DialogDescription>
          Tujuan otomatis lokasi konsinyasi atau buffer vending milik mitra.
          Logistik mengirim barang (Transfer Stok + jurnal A-07/A-24).
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <Field data-invalid={!!errors.deliveryType}>
            <FieldLabel htmlFor="cd-type">Jenis</FieldLabel>
            <Select
              value={deliveryType}
              disabled={!!delivery}
              onValueChange={(v) =>
                setDeliveryType(v as ConsignmentDeliveryType)
              }
            >
              <SelectTrigger id="cd-type" className="w-full">
                <SelectValue>
                  {(v: string) =>
                    DELIVERY_TYPE_LABELS[v as ConsignmentDeliveryType]
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="consignment">Konsinyasi</SelectItem>
                <SelectItem value="vending_buffer">Buffer Vending</SelectItem>
              </SelectContent>
            </Select>
          </Field>
          <Field data-invalid={!!errors.partnerId}>
            <FieldLabel htmlFor="cd-partner">Mitra Konsinyasi</FieldLabel>
            <SearchPicker
              id="cd-partner"
              value={partner}
              onValueChange={setPartner}
              disabled={!!delivery}
              query={(q) => consignmentPartnersQuery(q)}
              toOption={(p) => ({
                id: p.id,
                label: p.name,
                description: p.code,
              })}
              placeholder="Pilih mitra"
              searchPlaceholder="Cari kode atau nama mitra"
              aria-invalid={!!errors.partnerId}
            />
            <FieldError errors={fieldErrors(errors.partnerId)} />
          </Field>
          <Field data-invalid={!!errors.fromLocationId}>
            <FieldLabel htmlFor="cd-from">Lokasi Asal</FieldLabel>
            <OptionSelect
              id="cd-from"
              value={fromLocationId}
              onValueChange={setFromLocationId}
              options={(locations.data ?? []).map((l) => ({
                id: l.id,
                code: l.code,
                name: `${l.code} – ${l.name}`,
              }))}
              placeholder="Pilih lokasi FG"
              aria-invalid={!!errors.fromLocationId}
            />
            <FieldError errors={fieldErrors(errors.fromLocationId)} />
          </Field>
          <Field data-invalid={!!errors.businessDate}>
            <FieldLabel htmlFor="cd-date">Tanggal Kirim</FieldLabel>
            <DateInput
              id="cd-date"
              value={businessDate}
              onValueChange={setBusinessDate}
            />
            <FieldError errors={fieldErrors(errors.businessDate)} />
          </Field>
        </div>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead className="w-40">Qty</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {lines.map((l, i) => (
                <TableRow key={l.key}>
                  <TableCell className="min-w-64">
                    <ProductPicker
                      value={l.product}
                      types={["finished_good"]}
                      isBundle={false}
                      aria-invalid={!!lineErr(i, "productId")}
                      excludeIds={lines
                        .filter((x) => x.key !== l.key && x.product)
                        .map((x) => x.product?.id ?? "")}
                      onValueChange={(p) =>
                        setLines((ls) =>
                          ls.map((x) =>
                            x.key === l.key ? { ...x, product: p } : x
                          )
                        )
                      }
                    />
                    <LineError message={lineErr(i, "productId")} />
                  </TableCell>
                  <TableCell>
                    <QuantityInput
                      aria-label={`Qty baris ${i + 1}`}
                      value={l.qty}
                      unit={l.product?.uom}
                      aria-invalid={!!lineErr(i, "qty")}
                      onValueChange={(qty) =>
                        setLines((ls) =>
                          ls.map((x) => (x.key === l.key ? { ...x, qty } : x))
                        )
                      }
                    />
                    <LineError message={lineErr(i, "qty")} />
                  </TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`Hapus baris ${i + 1}`}
                      disabled={lines.length === 1}
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
          </Table>
        </div>
        <LineError message={errors.lines} />
        <Button
          type="button"
          variant="outline"
          className="self-start"
          onClick={() =>
            setLines((ls) => [
              ...ls,
              { key: crypto.randomUUID(), product: null, qty: "" },
            ])
          }
        >
          <PlusIcon /> Tambah Produk
        </Button>
        <Field>
          <FieldLabel htmlFor="cd-note">Catatan (opsional)</FieldLabel>
          <Textarea
            id="cd-note"
            value={note}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton type="submit" loading={save.isPending}>
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

const ACTION_TEXT: Record<
  Exclude<DeliveryAction, "cancel">,
  { label: string; toast: string }
> = {
  ship: { label: "Kirim", toast: "Barang dikirim ke lokasi mitra" },
  receive: { label: "Diterima Mitra", toast: "Barang diterima mitra" },
}

/** Detail pengiriman konsinyasi dengan aksi Kirim (Logistik), Diterima Mitra, dan Batalkan. */
export function ConsignmentDeliveryDetailPage({
  id,
  tab,
  onTabChange,
}: {
  id: string
  tab: DetailTab
  onTabChange: (t: DetailTab) => void
}) {
  const q = useQuery(deliveryQuery(id))
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const act = useIdempotentMutation({
    mutationFn: (v: { action: DeliveryAction; reason?: string }, key) =>
      deliveryAction(id, v.action, q.data?.version ?? 0, key, v.reason),
    onSuccess: async (_, v) => {
      toast.success(
        v.action === "cancel"
          ? "Pengiriman dibatalkan"
          : ACTION_TEXT[v.action].toast
      )
      setCancelling(false)
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const d = q.data
  return (
    <LoadingArea loading={q.isPending}>
      {d ? (
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {DELIVERY_TYPE_LABELS[d.deliveryType]} {d.number ?? "Draft"}
                </h1>
                <StatusBadge status={d.status} />
              </div>
              <p className="text-sm text-muted-foreground">
                {d.partnerName} · PKS {d.agreementNumber || "—"}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {d.allowedActions.includes("edit") ? (
                <Button variant="outline" onClick={() => setEditing(true)}>
                  Ubah
                </Button>
              ) : null}
              {(["ship", "receive"] as const)
                .filter((a) => d.allowedActions.includes(a))
                .map((a) => (
                  <LoadingButton
                    key={a}
                    className="min-h-11"
                    loading={act.isPending && act.variables?.action === a}
                    onClick={() => act.mutate({ action: a })}
                  >
                    {ACTION_TEXT[a].label}
                  </LoadingButton>
                ))}
              {d.allowedActions.includes("cancel") ? (
                <Button variant="outline" onClick={() => setCancelling(true)}>
                  Batalkan
                </Button>
              ) : null}
            </div>
          </div>
          <PageTabs
            label="Bagian pengiriman"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "summary", label: "Ringkasan" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "summary" ? (
            <>
              <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
                <Info
                  label="Tanggal Kirim"
                  value={formatDate(d.businessDate)}
                />
                <Info
                  label="Dari"
                  value={`${d.fromLocation.code} – ${d.fromLocation.name}`}
                />
                <Info
                  label="Ke"
                  value={`${d.toLocation.code} – ${d.toLocation.name}`}
                />
                <Info
                  label="Channel"
                  value={`${d.channelName} · ${d.subChannelName}`}
                />
                <Info label="Transfer Stok" value={d.transferNumber || "—"} />
                <Info label="Nilai HPP" value={formatRupiah(d.costAmount)} />
                <Info
                  label="Dikirim"
                  value={d.shippedAt ? formatDateTime(d.shippedAt) : "—"}
                />
                <Info
                  label="Diterima"
                  value={d.receivedAt ? formatDateTime(d.receivedAt) : "—"}
                />
                <Info label="Dibuat oleh" value={d.createdByName} />
                {d.note ? <Info label="Catatan" value={d.note} /> : null}
                {d.cancelReason ? (
                  <Info label="Alasan dibatalkan" value={d.cancelReason} />
                ) : null}
              </dl>
              <div className="overflow-x-auto rounded-lg border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Produk</TableHead>
                      <TableHead className="text-right">Qty</TableHead>
                      <TableHead className="text-right">Nilai HPP</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {d.lines.map((l) => (
                      <TableRow key={l.id}>
                        <TableCell>
                          {l.product.sku} – {l.product.name}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatQuantity(l.qty)} {l.product.uom}
                        </TableCell>
                        <TableCell className="text-right">
                          {formatRupiah(l.costAmount)}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          ) : (
            <EntityHistory entityType="consignment_delivery" entityId={d.id} />
          )}
          <Dialog open={editing} onOpenChange={setEditing}>
            {editing ? (
              <DeliveryForm delivery={d} onClose={() => setEditing(false)} />
            ) : null}
          </Dialog>
          <ReasonDialog
            open={cancelling}
            onOpenChange={setCancelling}
            title="Batalkan pengiriman"
            description="Draft pengiriman dibatalkan; stok tidak berubah."
            confirmLabel="Batalkan Pengiriman"
            required
            destructive
            loading={act.isPending}
            onConfirm={(reason) => act.mutate({ action: "cancel", reason })}
          />
        </div>
      ) : null}
    </LoadingArea>
  )
}
