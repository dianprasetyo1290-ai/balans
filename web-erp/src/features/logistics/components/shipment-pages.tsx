import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingArea } from "@/components/common/loading-area"
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
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  couriersQuery,
  createCourier,
  deliveryOrderQuery,
  recordShipmentEvent,
  SHIPMENT_STATUS_LABELS,
  shipmentsQuery,
  updateCourier,
  type Courier,
  type ShipmentStatus,
  type ShipmentSummary,
} from "../api/logistics-api"
import { DeliverDialog } from "./delivery-order-pages"

const shipmentsRoute = getRouteApi("/_app/logistics/shipments")
const ALL = "all"

function onProblem(setErrors?: (f: Record<string, string>) => void) {
  return (err: unknown) => {
    const p = parseProblem(err)
    if (p.fields && setErrors) {
      setErrors(p.fields)
    } else {
      showErrorDialog(p)
    }
  }
}

function ShipmentBadge({ status }: { status: ShipmentStatus }) {
  return <Badge variant="outline">{SHIPMENT_STATUS_LABELS[status]}</Badge>
}

/** Logistik › Pengiriman: daftar pengiriman, status pelacakan manual, dan konfirmasi diterima (HP Merchandiser). */
export function ShipmentListPage() {
  const search = shipmentsRoute.useSearch()
  const navigate = shipmentsRoute.useNavigate()
  const list = useQuery(shipmentsQuery(search))
  const [q, setQ] = useState(search.q ?? "")
  const [tracking, setTracking] = useState<ShipmentSummary | null>(null)
  const [delivering, setDelivering] = useState<ShipmentSummary | null>(null)
  const columns: DataTableColumn<ShipmentSummary>[] = [
    {
      id: "do",
      header: "Delivery Order",
      cell: (s) => (
        <Link
          to="/logistics/delivery-orders/$id"
          params={{ id: s.deliveryOrderId }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {s.deliveryOrderNumber}
        </Link>
      ),
    },
    { id: "recipient", header: "Penerima", cell: (s) => s.recipientName },
    {
      id: "courier",
      header: "Kurir",
      cell: (s) => (
        <div className="flex flex-col">
          <span>{s.courier.name}</span>
          <span className="text-xs text-muted-foreground">
            {s.internalCourierName || s.serviceCode}
          </span>
        </div>
      ),
    },
    {
      id: "resi",
      header: "Resi / AWB",
      cell: (s) => s.trackingNumber || s.marketplaceAwb || "—",
    },
    {
      id: "shipped",
      header: "Diserahkan",
      cell: (s) => (
        <span className="whitespace-nowrap">{formatDateTime(s.shippedAt)}</span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (s) => <ShipmentBadge status={s.status} />,
    },
    {
      id: "actions",
      header: "",
      cell: (s) =>
        s.status === "delivered" ? null : (
          <div className="flex gap-2">
            <Button
              size="sm"
              variant="outline"
              className="min-h-11"
              onClick={() => setTracking(s)}
            >
              Catat Status
            </Button>
            <Button
              size="sm"
              className="min-h-11"
              onClick={() => setDelivering(s)}
            >
              Diterima
            </Button>
          </div>
        ),
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Pengiriman</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Barang yang sudah diserahkan ke kurir. Merchandiser kurir internal
          mengonfirmasi diterima dengan foto/tanda terima dari HP.
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            void navigate({
              search: (s) => ({ ...s, q: q || undefined, page: 1 }),
            })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari pengiriman"
            placeholder="Cari DO, penerima, resi, AWB"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                status: v === ALL ? undefined : (v as ShipmentStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-48" aria-label="Filter status">
            <SelectValue>
              {(v: string) =>
                SHIPMENT_STATUS_LABELS[v as ShipmentStatus] ?? "Semua status"
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {Object.entries(SHIPMENT_STATUS_LABELS).map(([k, l]) => (
              <SelectItem key={k} value={k}>
                {l}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label className="flex min-h-11 items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-5"
            checked={!!search.mine}
            onChange={(e) =>
              void navigate({
                search: (s) => ({
                  ...s,
                  mine: e.target.checked || undefined,
                  page: 1,
                }),
              })
            }
          />
          Antaran saya (kurir internal)
        </label>
      </div>
      <DataTable
        caption="Daftar pengiriman"
        columns={columns}
        rows={list.data?.items}
        getRowId={(s) => s.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={search.page}
        totalPages={list.data?.totalPages ?? 1}
        totalItems={list.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada pengiriman."
      />
      {tracking ? (
        <TrackDialog shipment={tracking} onClose={() => setTracking(null)} />
      ) : null}
      {delivering ? (
        <DeliverFromList
          shipment={delivering}
          onClose={() => setDelivering(null)}
        />
      ) : null}
    </div>
  )
}

function DeliverFromList({
  shipment,
  onClose,
}: {
  shipment: ShipmentSummary
  onClose: () => void
}) {
  const d = useQuery(deliveryOrderQuery(shipment.deliveryOrderId))
  return d.data ? <DeliverDialog d={d.data} onClose={onClose} /> : null
}

function TrackDialog({
  shipment,
  onClose,
}: {
  shipment: ShipmentSummary
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<ShipmentStatus>("in_transit")
  const [note, setNote] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      recordShipmentEvent(
        shipment.id,
        { status, note: note || undefined },
        key
      ),
    onSuccess: async () => {
      toast.success("Status pengiriman dicatat")
      await queryClient.invalidateQueries({ queryKey: ["logistics"] })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Catat Status {shipment.deliveryOrderNumber}</DialogTitle>
          <DialogDescription>
            Pelacakan manual sesuai informasi kurir.
          </DialogDescription>
        </DialogHeader>
        <Field>
          <FieldLabel htmlFor="track-status">Status</FieldLabel>
          <Select
            value={status}
            onValueChange={(v) => setStatus(v as ShipmentStatus)}
          >
            <SelectTrigger id="track-status" className="min-h-11 w-full">
              <SelectValue>
                {(v: string) => SHIPMENT_STATUS_LABELS[v as ShipmentStatus]}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {(["picked_up", "in_transit", "returned"] as const).map((k) => (
                <SelectItem key={k} value={k}>
                  {SHIPMENT_STATUS_LABELS[k]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors.status ? <FieldError>{errors.status}</FieldError> : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="track-note">Catatan</FieldLabel>
          <Textarea
            id="track-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Batal
          </Button>
          <LoadingButton
            className="min-h-11"
            loading={save.isPending}
            onClick={() => save.mutate()}
          >
            Simpan
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Logistik › Pelacakan Pengiriman: cari nomor DO, resi, atau AWB lalu lihat riwayat status. */
export function TrackingPage({
  q,
  onSearch,
}: {
  q?: string
  onSearch: (q?: string) => void
}) {
  const [text, setText] = useState(q ?? "")
  const list = useQuery({ ...shipmentsQuery({ page: 1, q }), enabled: !!q })
  const first = list.data?.items[0]
  const detail = useQuery({
    ...deliveryOrderQuery(first?.deliveryOrderId ?? ""),
    enabled: !!first,
  })
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Pelacakan Pengiriman
        </h1>
        <p className="text-sm text-muted-foreground">
          Cari dengan nomor DO, nomor resi, atau AWB marketplace.
        </p>
      </div>
      <form
        className="flex max-w-lg gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          onSearch(text.trim() || undefined)
        }}
      >
        <Input
          aria-label="Nomor DO, resi, atau AWB"
          className="min-h-11"
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
        <Button type="submit" className="min-h-11">
          <SearchIcon /> Lacak
        </Button>
      </form>
      {q ? (
        <LoadingArea loading={list.isPending}>
          {!first ? (
            <p className="text-sm text-muted-foreground">
              Pengiriman tidak ditemukan.
            </p>
          ) : (
            <section className="flex flex-col gap-3 rounded-lg border p-4">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <Link
                  to="/logistics/delivery-orders/$id"
                  params={{ id: first.deliveryOrderId }}
                  className="font-medium text-primary underline"
                >
                  {first.deliveryOrderNumber}
                </Link>
                <ShipmentBadge status={first.status} />
              </div>
              <p className="text-sm">
                {first.recipientName} · {first.courier.name}{" "}
                {first.trackingNumber || first.marketplaceAwb}
              </p>
              <ol className="flex flex-col gap-2 border-l pl-4 text-sm">
                {(detail.data?.events ?? []).map((e) => (
                  <li key={e.id}>
                    <span className="font-medium">
                      {SHIPMENT_STATUS_LABELS[e.status]}
                    </span>{" "}
                    · {formatDateTime(e.occurredAt)}
                    {e.note ? (
                      <span className="block text-muted-foreground">
                        {e.note}
                      </span>
                    ) : null}
                  </li>
                ))}
              </ol>
              {list.data && list.data.items.length > 1 ? (
                <p className="text-xs text-muted-foreground">
                  {list.data.items.length - 1} pengiriman lain cocok; perjelas
                  pencarian.
                </p>
              ) : null}
            </section>
          )}
        </LoadingArea>
      ) : null}
    </div>
  )
}

/** Logistik › Kurir & Resi: master kurir; kurir internal mewajibkan Merchandiser pengantar. */
export function CouriersPage() {
  const list = useQuery(couriersQuery(false))
  const [editing, setEditing] = useState<Courier | "new" | null>(null)
  const columns: DataTableColumn<Courier>[] = [
    {
      id: "code",
      header: "Kode",
      cell: (c) => <span className="font-mono">{c.code}</span>,
    },
    { id: "name", header: "Nama", cell: (c) => c.name },
    {
      id: "type",
      header: "Jenis",
      cell: (c) =>
        c.isInternal ? "Kurir internal (Merchandiser)" : "Kurir eksternal",
    },
    {
      id: "active",
      header: "Status",
      cell: (c) => (c.isActive ? "Aktif" : "Nonaktif"),
    },
    {
      id: "edit",
      header: "",
      cell: (c) => (
        <Button
          size="sm"
          variant="outline"
          className="min-h-11"
          onClick={() => setEditing(c)}
        >
          Ubah
        </Button>
      ),
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Kurir & Resi
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Kurir yang bisa dipilih saat menyerahkan DO. Nomor resi dicatat
            manual di DO; AWB marketplace wajib untuk pesanan marketplace.
          </p>
        </div>
        <Button className="min-h-11" onClick={() => setEditing("new")}>
          <PlusIcon /> Tambah Kurir
        </Button>
      </div>
      <DataTable
        caption="Daftar kurir"
        columns={columns}
        rows={list.data?.items}
        getRowId={(c) => c.id}
        loading={list.isPending}
        fetching={list.isFetching}
        page={1}
        totalPages={1}
        totalItems={list.data?.items.length ?? 0}
        onSortChange={() => undefined}
        onPageChange={() => undefined}
        emptyMessage="Belum ada kurir."
      />
      {editing ? (
        <CourierDialog
          courier={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </div>
  )
}

function CourierDialog({
  courier,
  onClose,
}: {
  courier: Courier | null
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(courier?.code ?? "")
  const [name, setName] = useState(courier?.name ?? "")
  const [internal, setInternal] = useState(courier?.isInternal ?? false)
  const [active, setActive] = useState(courier?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      courier
        ? updateCourier(
            courier.id,
            {
              version: courier.version,
              name,
              isInternal: internal,
              isActive: active,
            },
            key
          )
        : createCourier(
            { code, name, isInternal: internal, isActive: active },
            key
          ),
    onSuccess: async () => {
      toast.success("Kurir disimpan")
      await queryClient.invalidateQueries({
        queryKey: ["logistics", "couriers"],
      })
      onClose()
    },
    onError: onProblem(setErrors),
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{courier ? "Ubah Kurir" : "Tambah Kurir"}</DialogTitle>
          <DialogDescription>
            Kode kurir tidak bisa diubah setelah disimpan.
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault()
            setErrors({})
            save.mutate()
          }}
        >
          <Field>
            <FieldLabel htmlFor="courier-code">Kode</FieldLabel>
            <Input
              id="courier-code"
              className="min-h-11"
              value={code}
              disabled={!!courier}
              onChange={(e) => setCode(e.target.value)}
            />
            {errors.code ? <FieldError>{errors.code}</FieldError> : null}
          </Field>
          <Field>
            <FieldLabel htmlFor="courier-name">Nama</FieldLabel>
            <Input
              id="courier-name"
              className="min-h-11"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            {errors.name ? <FieldError>{errors.name}</FieldError> : null}
          </Field>
          <label className="flex min-h-11 items-center gap-3 text-sm">
            <input
              type="checkbox"
              className="size-5"
              checked={internal}
              onChange={(e) => setInternal(e.target.checked)}
            />
            Kurir internal (wajib memilih Merchandiser)
          </label>
          <label className="flex min-h-11 items-center gap-3 text-sm">
            <input
              type="checkbox"
              className="size-5"
              checked={active}
              onChange={(e) => setActive(e.target.checked)}
            />
            Aktif
          </label>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              className="min-h-11"
            >
              Simpan
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
