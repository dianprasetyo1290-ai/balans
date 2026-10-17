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
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
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
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import {
  createRepacking,
  repackingQuery,
  repackingsQuery,
  runRepackingAction,
  updateRepacking,
  type Repacking,
  type RepackingAction,
  type RepackingInput,
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
  StockPickerDialog,
  stepText,
  type ActionSpec,
  type DocTab,
} from "./doc-shared"
import { StockStatusBadge } from "./stock-filters"

const listRoute = getRouteApi("/_app/inventory/repackings/")
const detailRoute = getRouteApi("/_app/inventory/repackings/$id/")

/** Persediaan › Repacking: barang retur + packaging baru menjadi FG batch baru (INV-06). */
export function RepackingListPage() {
  const search = listRoute.useSearch()
  const navigate = listRoute.useNavigate()
  const { can } = usePermissions()
  const docs = useQuery(repackingsQuery(search))
  const columns: DataTableColumn<Repacking>[] = [
    {
      id: "number",
      header: "Nomor",
      cell: (r) => (
        <Link
          to="/inventory/repackings/$id"
          params={{ id: r.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {r.number ?? "Draft"}
        </Link>
      ),
    },
    { id: "date", header: "Tanggal", cell: (r) => formatDate(r.businessDate) },
    {
      id: "output",
      header: "Hasil",
      cell: (r) => {
        const out = r.lines.find((l) => l.direction === "output")
        return out
          ? `${out.product.sku} · ${out.batchNumber} · ${out.qty}`
          : "—"
      },
    },
    {
      id: "cost",
      header: "Nilai HPP",
      align: "right",
      cell: (r) => formatRupiah(r.costAmount),
    },
    {
      id: "status",
      header: "Status",
      cell: (r) => <StatusBadge status={r.status} />,
    },
  ]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Repacking</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Barang retur dikemas ulang dengan packaging baru menjadi FG batch
            baru bernilai HPP barang retur + packaging.
          </p>
        </div>
        {can("inventory.repacking.write") ? (
          <Button render={<Link to="/inventory/repackings/new" />}>
            <PlusIcon /> Buat Repacking
          </Button>
        ) : null}
      </div>
      <DocListFilters
        q={search.q}
        status={search.status}
        statuses={["draft", "in_progress", "posted", "cancelled"]}
        onSearch={(q) =>
          void navigate({ search: (s) => ({ ...s, q, page: 1 }) })
        }
        onStatus={(status) =>
          void navigate({ search: (s) => ({ ...s, status, page: 1 }) })
        }
      />
      <DataTable
        columns={columns}
        rows={docs.data?.items}
        getRowId={(r) => r.id}
        loading={docs.isPending}
        fetching={docs.isFetching}
        page={search.page}
        totalPages={docs.data?.totalPages ?? 0}
        totalItems={docs.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada Repacking."
      />
    </div>
  )
}

type InputLine = {
  productId: string
  label: string
  locationId: string
  batchId?: string
  batchNumber?: string
  stockStatus: "available" | "quarantine"
  qty: string
}

/** Form Buat/Ubah Repacking. */
export function RepackingFormPage({ id }: { id?: string }) {
  const doc = useQuery({ ...repackingQuery(id ?? ""), enabled: Boolean(id) })
  if (id && !doc.data) {
    return <LoadingArea loading>{null}</LoadingArea>
  }
  return <RepackingForm doc={doc.data} />
}

function RepackingForm({ doc }: { doc?: Repacking }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const output = doc?.lines.find((l) => l.direction === "output")
  const [businessDate, setBusinessDate] = useState(
    doc?.businessDate ?? todayBusinessDate()
  )
  const [note, setNote] = useState(doc?.note ?? "")
  const [inputs, setInputs] = useState<InputLine[]>(
    doc?.lines
      .filter((l) => l.direction === "input")
      .map((l) => ({
        productId: l.product.id,
        label: `${l.product.sku} ${l.product.name}`,
        locationId: l.location.id,
        batchId: l.batch?.id,
        batchNumber: l.batch?.number,
        stockStatus: l.stockStatus,
        qty: l.qty,
      })) ?? []
  )
  const [pickFrom, setPickFrom] = useState<"returns" | "raw_material" | null>(
    null
  )
  const [returnsLoc, setReturnsLoc] = useState("")
  const [rmLoc, setRmLoc] = useState("")
  const [outProduct, setOutProduct] = useState<ProductOption | null>(
    output ? { ...output.product, variant: "" } : null
  )
  const [outLocation, setOutLocation] = useState(output?.location.id ?? "")
  const [outBatch, setOutBatch] = useState(output?.batchNumber ?? "")
  const [outExpiry, setOutExpiry] = useState(output?.expiryDate ?? "")
  const [outProduction, setOutProduction] = useState(
    output?.productionDate ?? ""
  )
  const [outQty, setOutQty] = useState(output?.qty ?? "")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const outIndex = inputs.length

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: RepackingInput = {
        businessDate,
        note: note || undefined,
        lines: [
          ...inputs.map((l) => ({
            direction: "input" as const,
            productId: l.productId,
            locationId: l.locationId,
            batchId: l.batchId,
            stockStatus: l.stockStatus,
            qty: l.qty || "0",
          })),
          {
            direction: "output" as const,
            productId: outProduct?.id ?? "",
            locationId: outLocation,
            batchNumber: outBatch,
            expiryDate: outExpiry || undefined,
            productionDate: outProduction || undefined,
            qty: outQty || "0",
          },
        ],
      }
      return doc
        ? updateRepacking(doc.id, { ...body, version: doc.version }, key)
        : createRepacking(body, key)
    },
    onSuccess: async (r) => {
      toast.success("Repacking disimpan")
      await queryClient.invalidateQueries({ queryKey: ["inventory"] })
      void navigate({ to: "/inventory/repackings/$id", params: { id: r.id } })
    },
    onError: (err) => handleFormError(err, setErrors),
  })

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
        {doc ? `Ubah ${doc.number ?? "Draft Repacking"}` : "Buat Repacking"}
      </h1>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field data-invalid={!!errors.businessDate}>
          <FieldLabel htmlFor="rp-date">Tanggal</FieldLabel>
          <DateInput
            id="rp-date"
            value={businessDate}
            onValueChange={setBusinessDate}
          />
          <FieldError errors={fieldErrors(errors, "businessDate")} />
        </Field>
        <Field>
          <FieldLabel htmlFor="rp-note">Catatan</FieldLabel>
          <Textarea
            id="rp-note"
            value={note}
            maxLength={500}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      <section className="flex flex-col gap-2">
        <h2 className="font-medium">Bahan</h2>
        <FieldError errors={fieldErrors(errors, "lines")} />
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <LocationSelect
                id="rp-returns"
                label="Gudang Retur"
                value={returnsLoc}
                onChange={setReturnsLoc}
                types={["returns"]}
              />
            </div>
            <Button
              type="button"
              variant="outline"
              disabled={!returnsLoc}
              onClick={() => setPickFrom("returns")}
            >
              Tambah Barang Retur
            </Button>
          </div>
          <div className="flex items-end gap-2">
            <div className="flex-1">
              <LocationSelect
                id="rp-rm"
                label="Lokasi RM (packaging)"
                value={rmLoc}
                onChange={setRmLoc}
                types={["raw_material"]}
              />
            </div>
            <Button
              type="button"
              variant="outline"
              disabled={!rmLoc}
              onClick={() => setPickFrom("raw_material")}
            >
              Tambah Packaging
            </Button>
          </div>
        </div>
        {inputs.map((l, i) => (
          <div
            key={i}
            className="grid items-end gap-2 rounded-lg border p-3 sm:grid-cols-[1fr_10rem_auto]"
          >
            <div className="text-sm">
              <div className="font-medium">{l.label}</div>
              <div className="flex items-center gap-2 text-muted-foreground">
                {l.batchNumber ? `Batch ${l.batchNumber}` : "Tanpa batch"}
                <StockStatusBadge status={l.stockStatus} />
              </div>
              <FieldError
                errors={fieldErrors(errors, `lines[${i}].locationId`)}
              />
              <FieldError
                errors={fieldErrors(errors, `lines[${i}].productId`)}
              />
            </div>
            <Field data-invalid={!!errors[`lines[${i}].qty`]}>
              <FieldLabel htmlFor={`rp-qty-${i}`}>Qty</FieldLabel>
              <QuantityInput
                id={`rp-qty-${i}`}
                value={l.qty}
                onValueChange={(v) =>
                  setInputs((ls) =>
                    ls.map((x, j) => (j === i ? { ...x, qty: v } : x))
                  )
                }
              />
              <FieldError errors={fieldErrors(errors, `lines[${i}].qty`)} />
            </Field>
            <Button
              type="button"
              variant="ghost"
              aria-label={`Hapus bahan ${i + 1}`}
              onClick={() => setInputs((ls) => ls.filter((_, j) => j !== i))}
            >
              <Trash2Icon />
            </Button>
          </div>
        ))}
      </section>
      <section className="flex flex-col gap-3 rounded-lg border p-3">
        <h2 className="font-medium">Hasil FG</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field data-invalid={!!errors[`lines[${outIndex}].productId`]}>
            <FieldLabel htmlFor="rp-out-product">Produk Jadi</FieldLabel>
            <ProductPicker
              id="rp-out-product"
              value={outProduct}
              types={["finished_good"]}
              isBundle={false}
              onValueChange={setOutProduct}
            />
            <FieldError
              errors={fieldErrors(errors, `lines[${outIndex}].productId`)}
            />
          </Field>
          <div>
            <LocationSelect
              id="rp-out-location"
              label="Lokasi Hasil"
              value={outLocation}
              onChange={setOutLocation}
              types={["finished_goods"]}
              invalid={!!errors[`lines[${outIndex}].locationId`]}
            />
            <FieldError
              errors={fieldErrors(errors, `lines[${outIndex}].locationId`)}
            />
          </div>
          <Field
            data-invalid={
              !!errors[`lines[${outIndex}].batchNumber`] ||
              !!errors[`lines[${outIndex}].batch`]
            }
          >
            <FieldLabel htmlFor="rp-out-batch">Nomor Batch Baru</FieldLabel>
            <Input
              id="rp-out-batch"
              value={outBatch}
              maxLength={40}
              onChange={(e) => setOutBatch(e.target.value)}
            />
            <FieldError
              errors={fieldErrors(errors, `lines[${outIndex}].batchNumber`)}
            />
          </Field>
          <Field data-invalid={!!errors[`lines[${outIndex}].qty`]}>
            <FieldLabel htmlFor="rp-out-qty">Qty Hasil</FieldLabel>
            <QuantityInput
              id="rp-out-qty"
              value={outQty}
              unit={outProduct?.uom}
              onValueChange={setOutQty}
            />
            <FieldError
              errors={fieldErrors(errors, `lines[${outIndex}].qty`)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="rp-out-production">
              Tanggal Produksi
            </FieldLabel>
            <DateInput
              id="rp-out-production"
              value={outProduction}
              onValueChange={setOutProduction}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="rp-out-expiry">Tanggal Kedaluwarsa</FieldLabel>
            <DateInput
              id="rp-out-expiry"
              value={outExpiry}
              onValueChange={setOutExpiry}
            />
          </Field>
        </div>
      </section>
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
      <StockPickerDialog
        open={pickFrom !== null}
        onOpenChange={(o) => !o && setPickFrom(null)}
        locationId={pickFrom === "returns" ? returnsLoc : rmLoc}
        onPick={(b) =>
          setInputs((ls) => [
            ...ls,
            {
              productId: b.product.id,
              label: `${b.product.sku} ${b.product.name}`,
              locationId: b.location.id,
              batchId: b.batchId ?? undefined,
              batchNumber: b.batchNumber ?? undefined,
              stockStatus:
                b.status === "quarantine" ? "quarantine" : "available",
              qty: b.qty,
            },
          ])
        }
      />
    </form>
  )
}

const REPACK_ACTIONS: ActionSpec<RepackingAction>[] = [
  {
    action: "start",
    label: "Mulai Repacking",
    primary: true,
    confirmTitle: "Mulai repacking?",
    confirmDescription:
      "Dokumen mendapat nomor RPK dan bahan berstatus Tersedia dialokasikan selama proses.",
  },
  {
    action: "post",
    label: "Posting",
    primary: true,
    confirmTitle: "Posting repacking?",
    confirmDescription:
      "Bahan keluar dari stok, FG hasil masuk dengan batch baru bernilai HPP barang retur + packaging, dan jurnal A-26 dibuat.",
  },
  {
    action: "cancel",
    label: "Batalkan",
    destructive: true,
    confirmTitle: "Batalkan repacking?",
    confirmDescription:
      "Alokasi bahan dilepas dan dokumen tidak bisa diproses lagi.",
  },
]

/** Detail Repacking. */
export function RepackingDetailPage({ id }: { id: string }) {
  const doc = useQuery(repackingQuery(id))
  return (
    <LoadingArea loading={doc.isPending}>
      {doc.data ? <RepackingDetail doc={doc.data} /> : null}
    </LoadingArea>
  )
}

function RepackingDetail({ doc: r }: { doc: Repacking }) {
  const search = detailRoute.useSearch()
  const navigate = detailRoute.useNavigate()
  return (
    <div className="flex flex-col gap-4">
      <DocHeader
        title={r.number ?? "Draft Repacking"}
        status={r.status}
        subtitle={r.note}
      >
        {r.allowedActions.includes("edit") ? (
          <Button
            variant="outline"
            render={
              <Link to="/inventory/repackings/$id/edit" params={{ id: r.id }} />
            }
          >
            Ubah
          </Button>
        ) : null}
        <DocActionButtons
          specs={REPACK_ACTIONS}
          allowed={r.allowedActions}
          run={(a, key) => runRepackingAction(r.id, a, r.version, key)}
        />
      </DocHeader>
      <DocTabs
        tab={search.tab}
        onTabChange={(tab: DocTab) => void navigate({ search: { tab } })}
        entityType="repacking"
        entityId={r.id}
      >
        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
          <Info label="Tanggal" value={formatDate(r.businessDate)} />
          <Info label="Dibuat" value={r.createdBy.name} />
          <Info label="Mulai" value={stepText(r.started)} />
          <Info label="Diposting" value={stepText(r.posted)} />
          <Info label="Nilai HPP" value={formatRupiah(r.costAmount)} />
          {r.returnDispositionId ? (
            <Info
              label="Disposisi Retur"
              value={
                <Link
                  to="/warehouse/return-dispositions/$id"
                  params={{ id: r.returnDispositionId }}
                  search={{ tab: "detail" }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat disposisi
                </Link>
              }
            />
          ) : null}
          {r.journalId ? (
            <Info
              label="Jurnal"
              value={
                <Link
                  to="/finance/journals/$id"
                  params={{ id: r.journalId }}
                  className="text-primary underline-offset-4 hover:underline"
                >
                  Lihat jurnal A-26
                </Link>
              }
            />
          ) : null}
        </dl>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[48rem]">
            <TableHeader>
              <TableRow>
                <TableHead>Arah</TableHead>
                <TableHead>Produk</TableHead>
                <TableHead>Lokasi</TableHead>
                <TableHead>Batch</TableHead>
                <TableHead className="text-right">Qty</TableHead>
                <TableHead className="text-right">HPP</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {r.lines.map((l) => (
                <TableRow key={l.id}>
                  <TableCell>
                    {l.direction === "input" ? "Bahan" : "Hasil"}
                  </TableCell>
                  <TableCell>
                    <span className="tabular-nums">{l.product.sku}</span>{" "}
                    {l.product.name}
                  </TableCell>
                  <TableCell>{l.location.code}</TableCell>
                  <TableCell>
                    {l.direction === "output"
                      ? l.batchNumber
                      : (l.batch?.number ?? "FEFO")}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {l.qty}
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
        <DocMovementsTable movements={r.movements} />
      </DocTabs>
    </div>
  )
}
