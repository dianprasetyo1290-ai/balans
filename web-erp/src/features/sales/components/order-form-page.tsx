import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon, UserPlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { QuantityInput } from "@/components/common/quantity-input"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import { createCustomer } from "@/features/party/api/party-api"
import {
  fieldErrors,
  LineError,
} from "@/features/procurement/components/shared"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  channelOptionsQuery,
  createOrder,
  customerOptionsQuery,
  locationOptionsQuery,
  ORDER_KINDS,
  ORDER_TYPE_LABELS,
  orderKindOf,
  orderQuery,
  partnerOptionsQuery,
  PRICE_RULE_LABELS,
  updateOrder,
  type SalesOrder,
  type SalesOrderFields,
  type SalesOrderType,
  type SalesPaymentMode,
} from "../api/sales-documents-api"
import { orderPreview, type DiscountKind } from "../lib/sales-lib"

type LineDraft = {
  key: string
  product: ProductOption | null
  qty: string
  discountKind: DiscountKind
  discountValue: string
  /** Harga marketplace (input) atau harga tersimpan untuk preview. */
  unitPrice: string
  priceRule: string
  note: string
}

const emptyLine = (): LineDraft => ({
  key: crypto.randomUUID(),
  product: null,
  qty: "",
  discountKind: "percent",
  discountValue: "",
  unitPrice: "",
  priceRule: "",
  note: "",
})

type Initial = {
  orderType: SalesOrderType
  subChannelId: string
  party: PickerOption | null
  partyKind: "partner" | "customer"
  buyerName: string
  externalOrderId: string
  clientPoNumber: string
  businessDate: string
  sourceLocationId: string
  paymentMode: SalesPaymentMode
  paymentTermDays: string
  shipToName: string
  shipToPhone: string
  shipToAddress: string
  docDiscountKind: DiscountKind
  docDiscountValue: string
  shipping: string
  note: string
  lines: LineDraft[]
}

function blank(orderType: SalesOrderType): Initial {
  return {
    orderType,
    subChannelId: "",
    party: null,
    partyKind:
      orderType === "modern_trade" || orderType === "b2b"
        ? "partner"
        : "customer",
    buyerName: "",
    externalOrderId: "",
    clientPoNumber: "",
    businessDate: todayBusinessDate(),
    sourceLocationId: "",
    paymentMode: "prepaid",
    paymentTermDays: "",
    shipToName: "",
    shipToPhone: "",
    shipToAddress: "",
    docDiscountKind: orderType === "marketplace" ? "amount" : "percent",
    docDiscountValue: "",
    shipping: "",
    note: "",
    lines: [emptyLine()],
  }
}

function fromOrder(o: SalesOrder): Initial {
  const partnerId = o.partnerId ?? null
  return {
    orderType: o.orderType,
    subChannelId: o.subChannelId,
    party:
      partnerId || o.customerId
        ? {
            id: partnerId ?? o.customerId ?? "",
            label: o.partyName,
            description: o.partyCode,
          }
        : null,
    partyKind: partnerId ? "partner" : "customer",
    buyerName: o.orderType === "marketplace" ? o.partyName : "",
    externalOrderId: o.externalOrderId,
    clientPoNumber: o.clientPoNumber,
    businessDate: o.businessDate,
    sourceLocationId: o.sourceLocation.id,
    paymentMode: o.paymentMode,
    paymentTermDays: o.paymentTermDays ? String(o.paymentTermDays) : "",
    shipToName: o.shipToName,
    shipToPhone: o.shipToPhone,
    shipToAddress: o.shipToAddress,
    docDiscountKind: o.documentDiscount.kind,
    docDiscountValue:
      Number(o.documentDiscount.value) > 0 ? o.documentDiscount.value : "",
    shipping: Number(o.shippingAmount) > 0 ? o.shippingAmount : "",
    note: o.note,
    lines: o.lines.map((l) => ({
      key: crypto.randomUUID(),
      product: { ...l.product, variant: "" },
      qty: l.qty,
      discountKind: l.discount.kind,
      discountValue: Number(l.discount.value) > 0 ? l.discount.value : "",
      unitPrice: l.unitPrice,
      priceRule: l.priceRule,
      note: l.note,
    })),
  }
}

/** Buat Sales Order / catat penjualan marketplace. */
export function OrderCreatePage({ orderType }: { orderType: SalesOrderType }) {
  return <OrderForm initial={blank(orderType)} />
}

/** Ubah draft Sales Order. */
export function OrderEditPage({ id }: { id: string }) {
  const order = useQuery(orderQuery(id))
  return (
    <LoadingArea loading={order.isPending}>
      {order.data ? (
        order.data.allowedActions.includes("edit") ? (
          <OrderForm order={order.data} initial={fromOrder(order.data)} />
        ) : (
          <p className="text-sm text-muted-foreground">
            Hanya Sales Order berstatus Draft yang bisa diubah.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

function OrderForm({
  order,
  initial,
}: {
  order?: SalesOrder
  initial: Initial
}) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const t = initial.orderType
  const kind = ORDER_KINDS[orderKindOf(t)]
  const marketplace = t === "marketplace"
  const mt = t === "modern_trade" || t === "b2b"
  const channels = useQuery(channelOptionsQuery)
  const locations = useQuery(locationOptionsQuery(t))
  const channel = channels.data?.find(
    (c) => c.kind === "channel" && c.code === t
  )
  const subChannels = (channels.data ?? []).filter(
    (c) => c.kind === "sub_channel" && c.channelId === channel?.id
  )

  const [subChannelId, setSubChannelId] = useState(initial.subChannelId)
  const [party, setParty] = useState(initial.party)
  const [partyKind, setPartyKind] = useState(initial.partyKind)
  const [buyerName, setBuyerName] = useState(initial.buyerName)
  const [externalOrderId, setExternalOrderId] = useState(
    initial.externalOrderId
  )
  const [clientPoNumber, setClientPoNumber] = useState(initial.clientPoNumber)
  const [businessDate, setBusinessDate] = useState(initial.businessDate)
  const [sourceLocationId, setSourceLocationId] = useState(
    initial.sourceLocationId
  )
  const [paymentMode, setPaymentMode] = useState(initial.paymentMode)
  const [paymentTermDays, setPaymentTermDays] = useState(
    initial.paymentTermDays
  )
  const [shipToName, setShipToName] = useState(initial.shipToName)
  const [shipToPhone, setShipToPhone] = useState(initial.shipToPhone)
  const [shipToAddress, setShipToAddress] = useState(initial.shipToAddress)
  const [docDiscountKind, setDocDiscountKind] = useState(
    initial.docDiscountKind
  )
  const [docDiscountValue, setDocDiscountValue] = useState(
    initial.docDiscountValue
  )
  const [shipping, setShipping] = useState(initial.shipping)
  const [note, setNote] = useState(initial.note)
  const [lines, setLines] = useState(initial.lines)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [creatingCustomer, setCreatingCustomer] = useState(false)

  const location =
    sourceLocationId ||
    (locations.data?.length === 1 ? locations.data[0].id : "")
  const subChannel =
    subChannelId ||
    (party
      ? (subChannels.find((s) => s.partyId === party.id)?.id ?? "")
      : "") ||
    (subChannels.length === 1 ? subChannels[0].id : "")

  const preview = orderPreview(
    lines.map((l) => ({
      qty: l.qty,
      unitPrice: l.unitPrice,
      discountKind: l.discountKind,
      discountValue: l.discountValue,
    })),
    { kind: docDiscountKind, value: docDiscountValue },
    shipping
  )
  const unknownPrices = lines.some((l) => l.product && !l.unitPrice)

  const updateLine = (key: string, patch: Partial<LineDraft>) =>
    setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: SalesOrderFields = {
        channelId: channel?.id ?? "",
        subChannelId: subChannel,
        customerId:
          !marketplace && partyKind === "customer" ? (party?.id ?? null) : null,
        partnerId:
          !marketplace && partyKind === "partner" ? (party?.id ?? null) : null,
        buyerName: marketplace ? buyerName : undefined,
        externalOrderId: marketplace ? externalOrderId : undefined,
        clientPoNumber: mt ? clientPoNumber : undefined,
        businessDate,
        sourceLocationId: location || null,
        paymentMode: mt ? paymentMode : "prepaid",
        paymentTermDays:
          mt && paymentMode === "credit" ? Number(paymentTermDays || 0) : 0,
        shipToName,
        shipToPhone,
        shipToAddress,
        documentDiscount: {
          kind: docDiscountKind,
          value: docDiscountValue || "0",
        },
        shippingAmount: marketplace ? "0" : shipping || "0",
        note,
        lines: lines.map((l) => ({
          productId: l.product?.id ?? "",
          qty: l.qty,
          discount: { kind: l.discountKind, value: l.discountValue || "0" },
          unitPrice: marketplace && l.unitPrice ? l.unitPrice : null,
          note: l.note,
        })),
      }
      return order
        ? updateOrder(order.id, { ...body, version: order.version }, key)
        : createOrder({ ...body, orderType: t }, key)
    },
    onSuccess: async (saved) => {
      toast.success(
        marketplace
          ? "Penjualan marketplace dicatat, stok dialokasikan"
          : "Draft Sales Order disimpan"
      )
      await queryClient.invalidateQueries({ queryKey: ["sales"] })
      void navigate({
        to: "/sales/orders/$id",
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
    if (!subChannel) next.subChannelId = "Pilih sub-channel"
    if (!businessDate) next.businessDate = "Tanggal wajib diisi"
    if (!location) next.sourceLocationId = "Pilih lokasi asal stok"
    if (!marketplace && !party)
      next[partyKind === "partner" ? "partnerId" : "customerId"] =
        partyKind === "partner" ? "Pilih mitra" : "Pilih pelanggan"
    if (marketplace && !externalOrderId.trim())
      next.externalOrderId = "Nomor order marketplace wajib diisi"
    lines.forEach((l, i) => {
      if (!l.product) next[`lines.${i}.productId`] = "Pilih produk"
      if (!l.qty || Number(l.qty) <= 0)
        next[`lines.${i}.qty`] = "Qty harus lebih dari nol"
    })
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  const err = (k: string) => errors[k]
  const lineError = (i: number, f: string) => errors[`lines.${i}.${f}`]
  const partyField = partyKind === "partner" ? "partnerId" : "customerId"

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {order
            ? `Ubah Sales Order ${order.number ?? ""}`.trim()
            : marketplace
              ? "Catat Penjualan Marketplace"
              : `Buat Sales Order ${ORDER_TYPE_LABELS[t]}`}
        </h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          {kind.description} Harga satuan diambil otomatis dari daftar harga
          {marketplace ? " bila harga marketplace kosong" : ""}; total final
          dihitung sistem. NON-PKP: tanpa PPN.
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
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {marketplace ? (
            <>
              <Field data-invalid={!!err("externalOrderId")}>
                <FieldLabel htmlFor="so-ext">
                  Nomor Order Marketplace
                </FieldLabel>
                <Input
                  id="so-ext"
                  value={externalOrderId}
                  maxLength={80}
                  aria-invalid={!!err("externalOrderId")}
                  onChange={(e) => setExternalOrderId(e.target.value)}
                />
                <FieldError errors={fieldErrors(err("externalOrderId"))} />
              </Field>
              <Field>
                <FieldLabel htmlFor="so-buyer">Nama Pembeli</FieldLabel>
                <Input
                  id="so-buyer"
                  value={buyerName}
                  maxLength={200}
                  onChange={(e) => setBuyerName(e.target.value)}
                />
              </Field>
            </>
          ) : (
            <Field data-invalid={!!err(partyField)}>
              <div className="flex items-center justify-between gap-2">
                <FieldLabel htmlFor="so-party">
                  {partyKind === "partner"
                    ? t === "b2b"
                      ? "Mitra B2B"
                      : "Mitra Modern Trade"
                    : t === "employee"
                      ? "Pelanggan Karyawan"
                      : t === "b2b"
                        ? "Pelanggan B2B"
                        : "Pelanggan (cari nama atau nomor HP)"}
                </FieldLabel>
                {t === "b2b" ? (
                  <button
                    type="button"
                    className="text-xs text-primary underline-offset-4 hover:underline"
                    onClick={() => {
                      setParty(null)
                      setPartyKind(
                        partyKind === "partner" ? "customer" : "partner"
                      )
                    }}
                  >
                    {partyKind === "partner"
                      ? "Pakai pelanggan B2B"
                      : "Pakai mitra B2B"}
                  </button>
                ) : null}
              </div>
              <div className="flex gap-2">
                <div className="min-w-0 flex-1">
                  {partyKind === "partner" ? (
                    <SearchPicker
                      id="so-party"
                      value={party}
                      onValueChange={setParty}
                      query={(q) =>
                        partnerOptionsQuery(q, t === "b2b" ? "b2b" : undefined)
                      }
                      toOption={(p) => ({
                        id: p.id,
                        label: p.name,
                        description: p.code,
                      })}
                      placeholder="Pilih mitra"
                      searchPlaceholder="Cari kode atau nama mitra"
                      aria-invalid={!!err(partyField)}
                    />
                  ) : (
                    <SearchPicker
                      id="so-party"
                      value={party}
                      onValueChange={(p) => setParty(p)}
                      query={(q) =>
                        customerOptionsQuery(
                          q,
                          t === "employee" ? "employee" : undefined
                        )
                      }
                      toOption={(c) => ({
                        id: c.id,
                        label: c.fullName,
                        description: `${c.code} · ${c.phone}`,
                      })}
                      placeholder="Pilih pelanggan"
                      searchPlaceholder="Cari nama, kode, atau nomor HP"
                      aria-invalid={!!err(partyField)}
                    />
                  )}
                </div>
                {t === "social_commerce" ? (
                  <Button
                    type="button"
                    variant="outline"
                    aria-label="Buat pelanggan baru"
                    onClick={() => setCreatingCustomer(true)}
                  >
                    <UserPlusIcon />
                    <span className="hidden sm:inline">Pelanggan Baru</span>
                  </Button>
                ) : null}
              </div>
              <FieldError errors={fieldErrors(err(partyField))} />
            </Field>
          )}
          <Field data-invalid={!!err("subChannelId")}>
            <FieldLabel htmlFor="so-sub">
              {marketplace ? "Marketplace" : "Sub-channel"}
            </FieldLabel>
            <Select
              value={subChannel}
              onValueChange={(v) => setSubChannelId(String(v ?? ""))}
            >
              <SelectTrigger
                id="so-sub"
                className="w-full"
                aria-invalid={!!err("subChannelId")}
              >
                <SelectValue>
                  {(v: string) =>
                    subChannels.find((s) => s.id === v)?.name ??
                    "Pilih sub-channel"
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {subChannels.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError
              errors={fieldErrors(err("subChannelId") ?? err("channelId"))}
            />
          </Field>
          <Field data-invalid={!!err("businessDate")}>
            <FieldLabel htmlFor="so-date">
              {marketplace ? "Tanggal Order" : "Tanggal Sales Order"}
            </FieldLabel>
            <DateInput
              id="so-date"
              value={businessDate}
              onValueChange={setBusinessDate}
              aria-invalid={!!err("businessDate")}
            />
            <FieldError errors={fieldErrors(err("businessDate"))} />
          </Field>
          <Field data-invalid={!!err("sourceLocationId")}>
            <FieldLabel htmlFor="so-location">Lokasi Asal Stok</FieldLabel>
            <Select
              value={location}
              onValueChange={(v) => setSourceLocationId(String(v ?? ""))}
            >
              <SelectTrigger
                id="so-location"
                className="w-full"
                aria-invalid={!!err("sourceLocationId")}
              >
                <SelectValue>
                  {(v: string) => {
                    const l = locations.data?.find((x) => x.id === v)
                    return l ? `${l.code} · ${l.name}` : "Pilih lokasi"
                  }}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(locations.data ?? []).map((l) => (
                  <SelectItem key={l.id} value={l.id}>
                    {l.code} · {l.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError errors={fieldErrors(err("sourceLocationId"))} />
          </Field>
          {mt ? (
            <>
              <Field>
                <FieldLabel htmlFor="so-po">Nomor PO Klien</FieldLabel>
                <Input
                  id="so-po"
                  value={clientPoNumber}
                  maxLength={200}
                  onChange={(e) => setClientPoNumber(e.target.value)}
                />
              </Field>
              <Field data-invalid={!!err("paymentMode")}>
                <FieldLabel htmlFor="so-mode">Cara Bayar</FieldLabel>
                <div className="flex gap-2" role="radiogroup" id="so-mode">
                  {(
                    [
                      ["prepaid", "Bayar di muka (Proforma)"],
                      ["credit", "Tempo (invoice)"],
                    ] as const
                  ).map(([m, label]) => (
                    <button
                      key={m}
                      type="button"
                      role="radio"
                      aria-checked={paymentMode === m}
                      className={cn(
                        "min-h-11 flex-1 rounded-md border px-3 text-sm sm:min-h-9",
                        paymentMode === m
                          ? "border-primary bg-primary/10 font-medium"
                          : "text-muted-foreground hover:bg-accent"
                      )}
                      onClick={() => setPaymentMode(m)}
                    >
                      {label}
                    </button>
                  ))}
                </div>
                <FieldError errors={fieldErrors(err("paymentMode"))} />
              </Field>
              {paymentMode === "credit" ? (
                <Field data-invalid={!!err("paymentTermDays")}>
                  <FieldLabel htmlFor="so-term">Termin (hari)</FieldLabel>
                  <Input
                    id="so-term"
                    inputMode="numeric"
                    value={paymentTermDays}
                    aria-invalid={!!err("paymentTermDays")}
                    onChange={(e) =>
                      setPaymentTermDays(
                        e.target.value.replace(/\D/g, "").slice(0, 3)
                      )
                    }
                  />
                  <FieldError errors={fieldErrors(err("paymentTermDays"))} />
                </Field>
              ) : null}
            </>
          ) : null}
        </div>

        <section className="flex flex-col gap-2">
          <h2 className="text-sm font-semibold">Item</h2>
          <div className="overflow-x-auto rounded-lg border">
            <Table className="min-w-[56rem]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[30%]">Produk / Bundel</TableHead>
                  <TableHead className="w-32 text-right">Qty</TableHead>
                  <TableHead className="w-40 text-right">
                    {marketplace ? "Harga Marketplace" : "Harga"}
                  </TableHead>
                  <TableHead className="w-56">Diskon</TableHead>
                  <TableHead>Catatan</TableHead>
                  <TableHead className="w-12">
                    <span className="sr-only">Hapus</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {lines.map((l, i) => (
                  <TableRow key={l.key} className="align-top">
                    <TableCell>
                      <ProductPicker
                        value={l.product}
                        types={["finished_good"]}
                        aria-invalid={!!lineError(i, "productId")}
                        excludeIds={lines
                          .filter((x) => x.key !== l.key && x.product)
                          .map((x) => x.product?.id ?? "")}
                        onValueChange={(p) =>
                          updateLine(l.key, {
                            product: p,
                            unitPrice: marketplace ? l.unitPrice : "",
                            priceRule: "",
                          })
                        }
                      />
                      <LineError message={lineError(i, "productId")} />
                    </TableCell>
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
                    <TableCell className="text-right tabular-nums">
                      {marketplace ? (
                        <>
                          <MoneyInput
                            aria-label={`Harga marketplace baris ${i + 1}`}
                            value={l.unitPrice}
                            placeholder="Daftar harga"
                            onValueChange={(v) =>
                              updateLine(l.key, { unitPrice: v })
                            }
                          />
                          <LineError message={lineError(i, "unitPrice")} />
                        </>
                      ) : l.unitPrice ? (
                        <div className="flex flex-col items-end">
                          <span>{formatRupiah(l.unitPrice)}</span>
                          <span className="text-xs text-muted-foreground">
                            {PRICE_RULE_LABELS[l.priceRule] ?? l.priceRule}
                          </span>
                        </div>
                      ) : (
                        <span className="text-xs text-muted-foreground">
                          Otomatis saat disimpan
                        </span>
                      )}
                      {!marketplace ? (
                        <LineError message={lineError(i, "unitPrice")} />
                      ) : null}
                    </TableCell>
                    <TableCell>
                      <DiscountInput
                        label={`Diskon baris ${i + 1}`}
                        kind={l.discountKind}
                        value={l.discountValue}
                        onChange={(kind, value) =>
                          updateLine(l.key, {
                            discountKind: kind,
                            discountValue: value,
                          })
                        }
                      />
                      <LineError message={lineError(i, "discount")} />
                    </TableCell>
                    <TableCell>
                      <Input
                        aria-label={`Catatan baris ${i + 1}`}
                        value={l.note}
                        maxLength={200}
                        onChange={(e) =>
                          updateLine(l.key, { note: e.target.value })
                        }
                      />
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
          <LineError message={err("lines")} />
          <div>
            <Button
              type="button"
              variant="outline"
              onClick={() => setLines((ls) => [...ls, emptyLine()])}
            >
              <PlusIcon /> Tambah Item
            </Button>
          </div>
        </section>

        <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
          <div className="grid gap-4 sm:grid-cols-2">
            {!marketplace ? (
              <>
                <Field>
                  <FieldLabel htmlFor="so-ship-name">Nama Penerima</FieldLabel>
                  <Input
                    id="so-ship-name"
                    value={shipToName}
                    maxLength={200}
                    onChange={(e) => setShipToName(e.target.value)}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="so-ship-phone">HP Penerima</FieldLabel>
                  <Input
                    id="so-ship-phone"
                    value={shipToPhone}
                    inputMode="tel"
                    maxLength={200}
                    onChange={(e) => setShipToPhone(e.target.value)}
                  />
                </Field>
                <Field className="sm:col-span-2">
                  <FieldLabel htmlFor="so-ship-address">
                    Alamat Kirim (kosong = alamat utama)
                  </FieldLabel>
                  <Textarea
                    id="so-ship-address"
                    value={shipToAddress}
                    maxLength={1000}
                    onChange={(e) => setShipToAddress(e.target.value)}
                  />
                </Field>
              </>
            ) : null}
            <Field className="sm:col-span-2" data-invalid={!!err("note")}>
              <FieldLabel htmlFor="so-note">Catatan</FieldLabel>
              <Textarea
                id="so-note"
                value={note}
                maxLength={1000}
                onChange={(e) => setNote(e.target.value)}
              />
              <FieldError errors={fieldErrors(err("note"))} />
            </Field>
          </div>
          <div className="flex flex-col gap-3 rounded-lg border p-4 text-sm">
            <Row label="Bruto" value={formatRupiah(preview.gross)} />
            <Row
              label="Diskon baris"
              value={`−${formatRupiah(preview.lineDiscount)}`}
            />
            <Field data-invalid={!!err("documentDiscount")}>
              <FieldLabel>
                {marketplace ? "Voucher penjual" : "Diskon dokumen"}
              </FieldLabel>
              <DiscountInput
                label={marketplace ? "Voucher penjual" : "Diskon dokumen"}
                kind={docDiscountKind}
                value={docDiscountValue}
                onChange={(k, v) => {
                  setDocDiscountKind(k)
                  setDocDiscountValue(v)
                }}
              />
              <FieldError errors={fieldErrors(err("documentDiscount"))} />
            </Field>
            {!marketplace ? (
              <Field data-invalid={!!err("shippingAmount")}>
                <FieldLabel htmlFor="so-shipping">Ongkos Kirim</FieldLabel>
                <MoneyInput
                  id="so-shipping"
                  value={shipping}
                  onValueChange={setShipping}
                />
                <FieldError errors={fieldErrors(err("shippingAmount"))} />
              </Field>
            ) : null}
            <div className="border-t pt-3">
              <Row
                label="Total (tanpa PPN — NON-PKP)"
                value={formatRupiah(preview.total)}
                strong
              />
              {unknownPrices && !marketplace ? (
                <p className="mt-1 text-xs text-muted-foreground">
                  Harga produk baru dihitung otomatis saat disimpan.
                </p>
              ) : null}
            </div>
          </div>
        </div>

        <div className="flex flex-wrap justify-end gap-2">
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
            {marketplace ? "Simpan Penjualan Marketplace" : "Simpan Draft"}
          </LoadingButton>
        </div>
      </form>
      <NewCustomerDialog
        open={creatingCustomer}
        onOpenChange={setCreatingCustomer}
        onCreated={(c) => {
          setParty(c)
          setCreatingCustomer(false)
        }}
      />
    </div>
  )
}

function Row({
  label,
  value,
  strong = false,
}: {
  label: string
  value: string
  strong?: boolean
}) {
  return (
    <div
      className={cn(
        "flex items-baseline justify-between gap-2",
        strong && "font-semibold"
      )}
    >
      <span className={strong ? "" : "text-muted-foreground"}>{label}</span>
      <span className="tabular-nums">{value}</span>
    </div>
  )
}

/** Diskon manual persen atau nominal rupiah (MD-05). */
function DiscountInput({
  label,
  kind,
  value,
  onChange,
}: {
  label: string
  kind: DiscountKind
  value: string
  onChange: (kind: DiscountKind, value: string) => void
}) {
  return (
    <div className="flex gap-1">
      <Select
        value={kind}
        onValueChange={(v) => onChange((v as DiscountKind) ?? "percent", "")}
      >
        <SelectTrigger className="w-20" aria-label={`${label} — jenis`}>
          <SelectValue>
            {(v: string) => (v === "percent" ? "%" : "Rp")}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="percent">%</SelectItem>
          <SelectItem value="amount">Rp</SelectItem>
        </SelectContent>
      </Select>
      <div className="min-w-0 flex-1">
        {kind === "percent" ? (
          <Input
            aria-label={label}
            inputMode="decimal"
            value={value}
            placeholder="0"
            onChange={(e) =>
              onChange(
                kind,
                e.target.value.replace(",", ".").replace(/[^\d.]/g, "")
              )
            }
          />
        ) : (
          <MoneyInput
            aria-label={label}
            value={value}
            onValueChange={(v) => onChange(kind, v)}
          />
        )}
      </div>
    </div>
  )
}

/** Buat pelanggan baru langsung dari pesanan WA/IG (nomor HP dicek unik oleh server). */
function NewCustomerDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: (c: PickerOption) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? (
        <NewCustomerForm
          onCancel={() => onOpenChange(false)}
          onCreated={onCreated}
        />
      ) : null}
    </Dialog>
  )
}

function NewCustomerForm({
  onCancel,
  onCreated,
}: {
  onCancel: () => void
  onCreated: (c: PickerOption) => void
}) {
  const queryClient = useQueryClient()
  const [fullName, setFullName] = useState("")
  const [phone, setPhone] = useState("")
  const [address, setAddress] = useState("")
  const [city, setCity] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const create = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createCustomer(
        {
          fullName,
          phone,
          customerType: "retail",
          marketingConsent: false,
          addresses: address.trim()
            ? [
                {
                  recipientName: fullName,
                  phone,
                  addressLine: address,
                  city,
                  isDefault: true,
                },
              ]
            : [],
        },
        key
      ),
    onSuccess: async (c) => {
      toast.success("Pelanggan dibuat")
      await queryClient.invalidateQueries({
        queryKey: ["sales", "customer-options"],
      })
      onCreated({
        id: c.id,
        label: c.fullName,
        description: `${c.code} · ${c.phone}`,
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
  const e = (k: string) =>
    errors[k] ??
    Object.entries(errors).find(([f]) => f.startsWith(`addresses.0.${k}`))?.[1]
  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Pelanggan Baru</DialogTitle>
        <DialogDescription>
          Nomor HP dipakai untuk mencari pelanggan dan tidak boleh ganda.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        className="flex flex-col gap-3"
        onSubmit={(ev) => {
          ev.preventDefault()
          create.mutate()
        }}
      >
        <Field data-invalid={!!e("fullName")}>
          <FieldLabel htmlFor="nc-name">Nama Lengkap</FieldLabel>
          <Input
            id="nc-name"
            value={fullName}
            onChange={(ev) => setFullName(ev.target.value)}
          />
          <FieldError errors={fieldErrors(e("fullName"))} />
        </Field>
        <Field data-invalid={!!e("phone")}>
          <FieldLabel htmlFor="nc-phone">Nomor WhatsApp/HP</FieldLabel>
          <Input
            id="nc-phone"
            inputMode="tel"
            value={phone}
            onChange={(ev) => setPhone(ev.target.value)}
          />
          <FieldError errors={fieldErrors(e("phone"))} />
        </Field>
        <Field data-invalid={!!e("addressLine")}>
          <FieldLabel htmlFor="nc-address">Alamat Pengiriman</FieldLabel>
          <Textarea
            id="nc-address"
            value={address}
            onChange={(ev) => setAddress(ev.target.value)}
          />
          <FieldError errors={fieldErrors(e("addressLine"))} />
        </Field>
        <Field data-invalid={!!e("city")}>
          <FieldLabel htmlFor="nc-city">Kota/Kabupaten</FieldLabel>
          <Input
            id="nc-city"
            value={city}
            onChange={(ev) => setCity(ev.target.value)}
          />
          <FieldError errors={fieldErrors(e("city"))} />
        </Field>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onCancel}>
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={create.isPending}
            loadingText="Menyimpan…"
          >
            Simpan Pelanggan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
