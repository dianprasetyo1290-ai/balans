import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { QuantityInput } from "@/components/common/quantity-input"
import type { PickerOption } from "@/components/common/search-picker"
import { Badge } from "@/components/ui/badge"
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
import type { ProductType } from "@/features/catalog/api/catalog-api"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createOrder,
  divisionOptionsQuery,
  materialLocationsQuery,
  ORDER_KINDS,
  ORDER_TYPE_LABELS,
  orderKindOf,
  orderQuery,
  requestQuery,
  updateOrder,
  type PurchaseOrder,
  type PurchaseOrderFields,
  type PurchaseOrderType,
  type PurchaseRequest,
} from "../api/procurement-api"
import { documentTotal, lineAmount } from "../lib/procurement-lib"
import { fieldErrors, LineError, OptionSelect, SupplierPicker } from "./shared"

type LineDraft = {
  key: string
  product: ProductOption | null
  description: string
  qty: string
  unitPrice: string
  purchaseRequestLineId: string | null
  purchaseRequestNumber: string | null
  note: string
}

type MaterialDraft = {
  key: string
  product: ProductOption | null
  plannedQty: string
  note: string
}

const emptyLine = (): LineDraft => ({
  key: crypto.randomUUID(),
  product: null,
  description: "",
  qty: "",
  unitPrice: "",
  purchaseRequestLineId: null,
  purchaseRequestNumber: null,
  note: "",
})

const emptyMaterial = (): MaterialDraft => ({
  key: crypto.randomUUID(),
  product: null,
  plannedQty: "",
  note: "",
})

const isToll = (t: PurchaseOrderType) =>
  t === "toll_manufacturing" || t === "reprocess"

const LINE_PRODUCT_TYPES: Record<PurchaseOrderType, ProductType[] | undefined> =
  {
    packaging: ["packaging"],
    goods: undefined,
    other: undefined,
    toll_manufacturing: ["finished_good"],
    reprocess: ["finished_good"],
  }

const SELECTABLE_TYPES: Record<PurchaseOrderType, PurchaseOrderType[]> = {
  packaging: ["packaging", "goods", "other"],
  goods: ["packaging", "goods", "other"],
  other: ["packaging", "goods", "other"],
  toll_manufacturing: ["toll_manufacturing"],
  reprocess: ["reprocess"],
}

type Initial = {
  orderType: PurchaseOrderType
  supplier: PickerOption | null
  divisionId: string
  businessDate: string
  expectedDate: string
  paymentTermDays: string
  note: string
  materialSourceLocationId: string
  lines: LineDraft[]
  materials: MaterialDraft[]
}

function fromOrder(o: PurchaseOrder): Initial {
  return {
    orderType: o.orderType,
    supplier: {
      id: o.supplierId,
      label: o.supplierName,
      description: o.supplierCode,
    },
    divisionId: o.divisionId,
    businessDate: o.businessDate,
    expectedDate: o.expectedDate,
    paymentTermDays: String(o.paymentTermDays),
    note: o.note,
    materialSourceLocationId: o.materialSourceLocation?.id ?? "",
    lines: o.lines.map((l) => ({
      key: crypto.randomUUID(),
      product: l.product ? { ...l.product, variant: "" } : null,
      description: l.description,
      qty: l.orderedQty,
      unitPrice: l.unitPrice,
      purchaseRequestLineId: l.purchaseRequestLineId ?? null,
      purchaseRequestNumber: l.purchaseRequestNumber ?? null,
      note: l.note,
    })),
    materials: o.materials.map((m) => ({
      key: crypto.randomUUID(),
      product: { ...m.product, variant: "" },
      plannedQty: m.plannedQty,
      note: m.note,
    })),
  }
}

function fromRequest(
  orderType: PurchaseOrderType,
  r?: PurchaseRequest
): Initial {
  const lines =
    r?.lines
      .filter((l) => Number(l.remainingQty) > 0)
      .map((l) => ({
        key: crypto.randomUUID(),
        product: l.product ? { ...l.product, variant: "" } : null,
        description: l.description,
        qty: l.remainingQty,
        unitPrice: l.estimatedPrice,
        purchaseRequestLineId: l.id,
        purchaseRequestNumber: r.number ?? null,
        note: l.note,
      })) ?? []
  return {
    orderType,
    supplier: null,
    divisionId: r?.divisionId ?? "",
    businessDate: todayBusinessDate(),
    expectedDate: "",
    paymentTermDays: "",
    note: r?.note ?? "",
    materialSourceLocationId: "",
    lines: lines.length > 0 ? lines : [emptyLine()],
    materials: isToll(orderType) ? [emptyMaterial()] : [],
  }
}

/** Buat Purchase Order (barang/packaging, maklon, atau reprocess), opsional dari Purchase Request. */
export function OrderCreatePage({
  orderType,
  fromRequestId,
}: {
  orderType: PurchaseOrderType
  fromRequestId?: string
}) {
  const request = useQuery({
    ...requestQuery(fromRequestId ?? ""),
    enabled: !!fromRequestId,
  })
  if (fromRequestId && request.isPending) {
    return <LoadingArea loading>{null}</LoadingArea>
  }
  return (
    <OrderForm
      initial={fromRequest(orderType, request.data)}
      allTypes={!!fromRequestId}
    />
  )
}

/** Ubah draft Purchase Order. */
export function OrderEditPage({ id }: { id: string }) {
  const order = useQuery(orderQuery(id))
  return (
    <LoadingArea loading={order.isPending}>
      {order.data ? (
        order.data.allowedActions.includes("edit") ? (
          <OrderForm order={order.data} initial={fromOrder(order.data)} />
        ) : (
          <p className="text-sm text-muted-foreground">
            Hanya Purchase Order berstatus Draft yang bisa diubah.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

const ALL_TYPES: PurchaseOrderType[] = [
  "packaging",
  "goods",
  "other",
  "toll_manufacturing",
  "reprocess",
]

function OrderForm({
  order,
  initial,
  allTypes = false,
}: {
  order?: PurchaseOrder
  initial: Initial
  /** PO dari Purchase Request boleh dijadikan jenis apa pun. */
  allTypes?: boolean
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const divisions = useQuery(divisionOptionsQuery)
  const [orderType, setOrderType] = useState(initial.orderType)
  const toll = isToll(orderType)
  const selectable = allTypes ? ALL_TYPES : SELECTABLE_TYPES[orderType]
  const locations = useQuery({ ...materialLocationsQuery, enabled: toll })
  const [supplier, setSupplier] = useState(initial.supplier)
  const [divisionId, setDivisionId] = useState(initial.divisionId)
  const [businessDate, setBusinessDate] = useState(initial.businessDate)
  const [expectedDate, setExpectedDate] = useState(initial.expectedDate)
  const [paymentTermDays, setPaymentTermDays] = useState(
    initial.paymentTermDays
  )
  const [note, setNote] = useState(initial.note)
  const [sourceLocationId, setSourceLocationId] = useState(
    initial.materialSourceLocationId
  )
  const [lines, setLines] = useState(initial.lines)
  const [materials, setMaterials] = useState(initial.materials)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const kind = ORDER_KINDS[orderKindOf(orderType)]
  const sourceType =
    orderType === "reprocess" ? "goods_material" : "raw_material"
  const sourceOptions = (locations.data ?? [])
    .filter((l) => l.locationType === sourceType)
    .map((l) => ({ id: l.id, code: l.code, name: `${l.code} · ${l.name}` }))
  const total = documentTotal(
    lines.map((l) => ({ qty: l.qty, price: l.unitPrice }))
  )

  const updateLine = (key: string, patch: Partial<LineDraft>) =>
    setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)))
  const updateMaterial = (key: string, patch: Partial<MaterialDraft>) =>
    setMaterials((ms) =>
      ms.map((m) => (m.key === key ? { ...m, ...patch } : m))
    )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: PurchaseOrderFields = {
        supplierId: supplier?.id ?? "",
        divisionId,
        businessDate,
        expectedDate,
        paymentTermDays:
          paymentTermDays === "" ? null : Number(paymentTermDays),
        note,
        materialSourceLocationId:
          toll && sourceLocationId ? sourceLocationId : null,
        lines: lines.map((l) => ({
          productId: l.product?.id ?? null,
          description: l.description,
          qty: l.qty,
          unitPrice: l.unitPrice || "0",
          purchaseRequestLineId: l.purchaseRequestLineId,
          note: l.note,
        })),
        materials: toll
          ? materials.map((m) => ({
              productId: m.product?.id ?? "",
              plannedQty: m.plannedQty,
              note: m.note,
            }))
          : [],
      }
      return order
        ? updateOrder(order.id, { ...body, version: order.version }, key)
        : createOrder({ ...body, orderType }, key)
    },
    onSuccess: async (saved) => {
      toast.success("Draft Purchase Order disimpan")
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      void navigate({
        to: "/procurement/purchase-orders/$id",
        params: { id: saved.id },
        search: {},
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
    if (!supplier)
      next.supplierId = toll ? "Maklon wajib dipilih" : "Pemasok wajib dipilih"
    if (!divisionId) next.divisionId = "Divisi wajib dipilih"
    if (!businessDate) next.businessDate = "Tanggal PO wajib diisi"
    if (!expectedDate)
      next.expectedDate = "Tanggal kirim diharapkan wajib diisi"
    if (orderType === "reprocess" && !sourceLocationId)
      next.materialSourceLocationId =
        "Lokasi Gudang GM asal bahan wajib dipilih"
    lines.forEach((l, i) => {
      if (!l.product && (orderType !== "other" || !l.description.trim()))
        next[`lines.${i}.productId`] =
          orderType === "other"
            ? "Pilih produk atau isi uraian"
            : "Produk wajib dipilih"
      if (!l.qty || Number(l.qty) <= 0)
        next[`lines.${i}.qty`] = "Qty harus lebih dari nol"
    })
    if (toll) {
      if (materials.length === 0)
        next.materials = "Minimal satu bahan yang dikirim ke maklon"
      materials.forEach((m, i) => {
        if (!m.product) next[`materials.${i}.productId`] = "Bahan wajib dipilih"
        if (!m.plannedQty || Number(m.plannedQty) <= 0)
          next[`materials.${i}.plannedQty`] = "Qty harus lebih dari nol"
      })
    }
    if (Number(total) <= 0 && !next.lines)
      next.lines = "Total PO harus lebih dari nol"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  const err = (k: string) => errors[k]
  const lineError = (i: number, f: string) => errors[`lines.${i}.${f}`]
  const materialError = (i: number, f: string) => errors[`materials.${i}.${f}`]

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {order
            ? `Ubah ${kind.title} ${order.number ?? ""}`.trim()
            : `Buat ${kind.title}`}
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          {kind.description} Total final dihitung sistem saat disimpan.
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
        {!order && selectable.length > 1 ? (
          <fieldset className="flex flex-col gap-2">
            <legend className="text-sm font-medium">Jenis PO</legend>
            <div className="flex flex-wrap gap-2" role="radiogroup">
              {selectable.map((t) => (
                <button
                  key={t}
                  type="button"
                  role="radio"
                  aria-checked={orderType === t}
                  className={cn(
                    "min-h-11 rounded-md border px-3 text-sm sm:min-h-9",
                    orderType === t
                      ? "border-primary bg-primary/10 font-medium"
                      : "text-muted-foreground hover:bg-accent"
                  )}
                  onClick={() => {
                    setOrderType(t)
                    if (isToll(t) && materials.length === 0) {
                      setMaterials([emptyMaterial()])
                    }
                  }}
                >
                  {ORDER_TYPE_LABELS[t]}
                </button>
              ))}
            </div>
          </fieldset>
        ) : null}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <Field data-invalid={!!err("supplierId")}>
            <FieldLabel htmlFor="po-supplier">
              {toll ? "Maklon" : "Pemasok"}
            </FieldLabel>
            <SupplierPicker
              id="po-supplier"
              value={supplier}
              onValueChange={setSupplier}
              tollOnly={toll}
              aria-invalid={!!err("supplierId")}
            />
            <FieldError errors={fieldErrors(err("supplierId"))} />
          </Field>
          <Field data-invalid={!!err("divisionId")}>
            <FieldLabel htmlFor="po-division">Divisi</FieldLabel>
            <OptionSelect
              id="po-division"
              value={divisionId}
              onValueChange={setDivisionId}
              options={divisions.data ?? []}
              placeholder="Pilih divisi"
              aria-invalid={!!err("divisionId")}
            />
            <FieldError errors={fieldErrors(err("divisionId"))} />
          </Field>
          <Field data-invalid={!!err("paymentTermDays")}>
            <FieldLabel htmlFor="po-term">Termin Pembayaran (hari)</FieldLabel>
            <Input
              id="po-term"
              inputMode="numeric"
              value={paymentTermDays}
              placeholder="Default pemasok"
              aria-invalid={!!err("paymentTermDays")}
              onChange={(e) =>
                setPaymentTermDays(
                  e.target.value.replace(/\D/g, "").slice(0, 3)
                )
              }
            />
            <FieldError errors={fieldErrors(err("paymentTermDays"))} />
          </Field>
          <Field data-invalid={!!err("businessDate")}>
            <FieldLabel htmlFor="po-date">Tanggal PO</FieldLabel>
            <DateInput
              id="po-date"
              value={businessDate}
              onValueChange={setBusinessDate}
              aria-invalid={!!err("businessDate")}
            />
            <FieldError errors={fieldErrors(err("businessDate"))} />
          </Field>
          <Field data-invalid={!!err("expectedDate")}>
            <FieldLabel htmlFor="po-expected">
              Tanggal Kirim Diharapkan
            </FieldLabel>
            <DateInput
              id="po-expected"
              value={expectedDate}
              onValueChange={setExpectedDate}
              aria-invalid={!!err("expectedDate")}
            />
            <FieldError errors={fieldErrors(err("expectedDate"))} />
          </Field>
          {toll ? (
            <Field data-invalid={!!err("materialSourceLocationId")}>
              <FieldLabel htmlFor="po-source">
                {orderType === "reprocess"
                  ? "Lokasi Asal Bahan (Gudang GM – GM)"
                  : "Lokasi Asal Packaging (opsional)"}
              </FieldLabel>
              <Select
                value={sourceLocationId || "none"}
                onValueChange={(v) =>
                  setSourceLocationId(!v || v === "none" ? "" : String(v))
                }
              >
                <SelectTrigger
                  id="po-source"
                  className="w-full"
                  aria-invalid={!!err("materialSourceLocationId")}
                >
                  <SelectValue>
                    {(v: string) =>
                      sourceOptions.find((o) => o.id === v)?.name ??
                      (orderType === "reprocess"
                        ? "Pilih lokasi"
                        : "Sudah di lokasi maklon")
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {orderType !== "reprocess" ? (
                    <SelectItem value="none">Sudah di lokasi maklon</SelectItem>
                  ) : null}
                  {sourceOptions.map((o) => (
                    <SelectItem key={o.id} value={o.id}>
                      {o.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError
                errors={fieldErrors(err("materialSourceLocationId"))}
              />
            </Field>
          ) : null}
        </div>
        <Field data-invalid={!!err("note")}>
          <FieldLabel htmlFor="po-note">Catatan</FieldLabel>
          <Textarea
            id="po-note"
            value={note}
            rows={2}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
          />
          <FieldError errors={fieldErrors(err("note"))} />
        </Field>
        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-semibold">
            {toll ? "FG Hasil & Harga Jasa" : "Baris Barang"}
          </h2>
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[56rem]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-72">
                    {toll ? "Produk Jadi" : "Produk"}
                  </TableHead>
                  {orderType === "other" ? <TableHead>Uraian</TableHead> : null}
                  <TableHead className="w-36 text-right">Qty</TableHead>
                  <TableHead className="w-40 text-right">
                    {toll ? "Harga Jasa / Unit" : "Harga"}
                  </TableHead>
                  <TableHead className="w-36 text-right">Jumlah</TableHead>
                  <TableHead className="w-12" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {lines.map((l, i) => (
                  <TableRow key={l.key} className="align-top">
                    <TableCell>
                      <ProductPicker
                        value={l.product}
                        types={LINE_PRODUCT_TYPES[orderType]}
                        isBundle={false}
                        onValueChange={(p) => updateLine(l.key, { product: p })}
                        aria-invalid={!!lineError(i, "productId")}
                      />
                      {l.purchaseRequestNumber ? (
                        <Badge variant="outline" className="mt-1">
                          dari {l.purchaseRequestNumber}
                        </Badge>
                      ) : null}
                      <LineError message={lineError(i, "productId")} />
                      <LineError
                        message={lineError(i, "purchaseRequestLineId")}
                      />
                    </TableCell>
                    {orderType === "other" ? (
                      <TableCell>
                        <Input
                          aria-label={`Uraian baris ${i + 1}`}
                          value={l.description}
                          maxLength={500}
                          onChange={(e) =>
                            updateLine(l.key, { description: e.target.value })
                          }
                        />
                      </TableCell>
                    ) : null}
                    <TableCell>
                      <QuantityInput
                        aria-label={`Qty baris ${i + 1}`}
                        value={l.qty}
                        unit={l.product?.uom}
                        aria-invalid={!!lineError(i, "qty")}
                        onValueChange={(v) => updateLine(l.key, { qty: v })}
                      />
                      <LineError message={lineError(i, "qty")} />
                    </TableCell>
                    <TableCell>
                      <MoneyInput
                        aria-label={`Harga baris ${i + 1}`}
                        value={l.unitPrice}
                        aria-invalid={!!lineError(i, "unitPrice")}
                        onValueChange={(v) =>
                          updateLine(l.key, { unitPrice: v })
                        }
                      />
                      <LineError message={lineError(i, "unitPrice")} />
                    </TableCell>
                    <TableCell className="pt-4 text-right tabular-nums">
                      {formatRupiah(lineAmount(l.qty, l.unitPrice))}
                    </TableCell>
                    <TableCell>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={`Hapus baris ${i + 1}`}
                        disabled={lines.length <= 1}
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
                  <TableCell
                    colSpan={orderType === "other" ? 4 : 3}
                    className="font-medium"
                  >
                    Total (preview, tanpa PPN — NON-PKP)
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(total)}
                  </TableCell>
                  <TableCell />
                </TableRow>
              </TableFooter>
            </Table>
          </div>
          {err("lines") ? (
            <p className="text-sm text-destructive">{err("lines")}</p>
          ) : null}
          <div>
            <Button
              type="button"
              variant="outline"
              onClick={() => setLines((ls) => [...ls, emptyLine()])}
            >
              <PlusIcon /> Tambah Baris
            </Button>
          </div>
        </section>
        {toll ? (
          <section className="flex flex-col gap-2">
            <h2 className="text-sm font-semibold">
              {orderType === "reprocess"
                ? "Bahan GM yang Dikirim ke Maklon"
                : "Packaging yang Akan Dipakai Maklon"}
            </h2>
            <div className="overflow-x-auto rounded-lg border">
              <Table className="min-w-[44rem]">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-80">
                      {orderType === "reprocess"
                        ? "Material (GM)"
                        : "Packaging"}
                    </TableHead>
                    <TableHead className="w-40 text-right">
                      Qty Rencana
                    </TableHead>
                    <TableHead>Catatan</TableHead>
                    <TableHead className="w-12" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {materials.map((m, i) => (
                    <TableRow key={m.key} className="align-top">
                      <TableCell>
                        <ProductPicker
                          value={m.product}
                          types={
                            orderType === "reprocess"
                              ? ["reprocess_material"]
                              : ["packaging"]
                          }
                          isBundle={false}
                          excludeIds={materials
                            .filter((x) => x.key !== m.key && x.product)
                            .map((x) => x.product?.id ?? "")}
                          onValueChange={(p) =>
                            updateMaterial(m.key, { product: p })
                          }
                          aria-invalid={!!materialError(i, "productId")}
                        />
                        <LineError message={materialError(i, "productId")} />
                      </TableCell>
                      <TableCell>
                        <QuantityInput
                          aria-label={`Qty bahan ${i + 1}`}
                          value={m.plannedQty}
                          unit={m.product?.uom}
                          aria-invalid={!!materialError(i, "plannedQty")}
                          onValueChange={(v) =>
                            updateMaterial(m.key, { plannedQty: v })
                          }
                        />
                        <LineError message={materialError(i, "plannedQty")} />
                      </TableCell>
                      <TableCell>
                        <Input
                          aria-label={`Catatan bahan ${i + 1}`}
                          value={m.note}
                          maxLength={500}
                          onChange={(e) =>
                            updateMaterial(m.key, { note: e.target.value })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          aria-label={`Hapus bahan ${i + 1}`}
                          disabled={materials.length <= 1}
                          onClick={() =>
                            setMaterials((ms) =>
                              ms.filter((x) => x.key !== m.key)
                            )
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
            {err("materials") ? (
              <p className="text-sm text-destructive">{err("materials")}</p>
            ) : null}
            <div>
              <Button
                type="button"
                variant="outline"
                onClick={() => setMaterials((ms) => [...ms, emptyMaterial()])}
              >
                <PlusIcon /> Tambah Bahan
              </Button>
            </div>
          </section>
        ) : null}
        <div className="flex flex-wrap justify-end gap-2">
          <Button
            variant="outline"
            render={
              order ? (
                <Link
                  to="/procurement/purchase-orders/$id"
                  params={{ id: order.id }}
                  search={{}}
                />
              ) : (
                <Link to={kind.path} search={{ page: 1 }} />
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
      </form>
    </div>
  )
}
