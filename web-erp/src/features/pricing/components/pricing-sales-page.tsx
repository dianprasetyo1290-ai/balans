import { useMutation, useQuery } from "@tanstack/react-query"
import { PlusIcon } from "lucide-react"
import { useState } from "react"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { PageTabs } from "@/components/common/page-tabs"
import { QuantityInput } from "@/components/common/quantity-input"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Checkbox } from "@/features/accounting/components/account-form-dialog"
import { usePermissions } from "@/features/auth/api/auth-api"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import { partnerOptionsQuery } from "@/features/party/api/party-api"
import {
  ChoiceField,
  fieldError,
  type FieldErrors,
} from "@/features/party/components/form-parts"
import { parseProblem } from "@/lib/api/problem"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  DISCOUNT_KIND_LABELS,
  PRICE_RULE_LABELS,
  checkDiscount,
  customerOptionsQuery,
  discountLimitsQuery,
  formatPercent,
  parsePercentInput,
  quotePrice,
  type DiscountCheckResult,
  type DiscountKind,
  type PriceQuote,
} from "../api/pricing-api"
import {
  DatedDialogs,
  EmptyRow,
  RowMenu,
  datedMenu,
  useDatedActions,
} from "./price-rules-page"
import { ChannelFields, PeriodBadge, formatPeriod } from "./pricing-parts"
import { DiscountLimitFormDialog } from "./rule-form-dialogs"

export type PricingSalesTab = "simulator" | "limits"

type PricingSalesPageProps = {
  tab: PricingSalesTab
  includeEnded: boolean
  onSearchChange: (patch: {
    tab?: PricingSalesTab
    includeEnded?: boolean
  }) => void
}

/**
 * Penjualan › Harga & Diskon: simulasi harga dengan resolver yang sama dengan Sales Order (BRD 5.18) dan batas
 * diskon manual per role (MD-12, S-119).
 */
export function PricingSalesPage({
  tab,
  includeEnded,
  onSearchChange,
}: PricingSalesPageProps) {
  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Harga & Diskon
        </h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Cek harga yang akan dipakai transaksi beserta aturan dan versi
          asalnya, serta batas diskon manual per role. Diskon di atas batas
          membuat Sales Order butuh persetujuan Head Sales.
        </p>
      </div>
      <PageTabs
        label="Bagian harga & diskon"
        value={tab}
        onValueChange={(t) => onSearchChange({ tab: t })}
        tabs={[
          { id: "simulator", label: "Simulasi Harga" },
          { id: "limits", label: "Batas Diskon" },
        ]}
      />
      {tab === "simulator" ? (
        <Simulator />
      ) : (
        <DiscountLimits
          includeEnded={includeEnded}
          onIncludeEndedChange={(v) => onSearchChange({ includeEnded: v })}
        />
      )}
    </div>
  )
}

function Simulator() {
  const [product, setProduct] = useState<ProductOption | null>(null)
  const [qty, setQty] = useState("1")
  const [date, setDate] = useState(todayBusinessDate())
  const [channelId, setChannelId] = useState<string | null>(null)
  const [subChannelId, setSubChannelId] = useState<string | null>(null)
  const [customer, setCustomer] = useState<PickerOption | null>(null)
  const [partner, setPartner] = useState<PickerOption | null>(null)
  const [errors, setErrors] = useState<FieldErrors>({})
  const [discountKind, setDiscountKind] = useState<DiscountKind>("percent")
  const [discountValue, setDiscountValue] = useState("")
  const [docKind, setDocKind] = useState<DiscountKind>("percent")
  const [docValue, setDocValue] = useState("")

  const quote = useMutation({
    mutationFn: quotePrice,
    onError: (err) => {
      const p = parseProblem(err)
      if (p.code === "VALIDATION_FAILED" && p.fields) setErrors(p.fields)
      else showErrorDialog(p)
    },
  })
  const discount = useMutation({
    mutationFn: checkDiscount,
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const run = () => {
    const next: FieldErrors = {}
    if (!product) next.productId = "Pilih produk"
    if (!qty || Number(qty) <= 0) next.qty = "Qty harus lebih dari 0"
    if (!date) next.date = "Tanggal transaksi wajib diisi"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    discount.reset()
    quote.mutate({
      productId: product?.id ?? "",
      qty,
      date,
      channelId,
      subChannelId,
      customerId: customer?.id ?? null,
      partnerId: partner?.id ?? null,
    })
  }

  const gross = quote.data
    ? (Number(quote.data.unitPrice) * Number(qty)).toFixed(2)
    : ""

  const runDiscount = () => {
    if (!quote.data) return
    const value = (kind: DiscountKind, v: string) =>
      v.trim()
        ? { kind, value: kind === "percent" ? parsePercentInput(v) : v }
        : null
    discount.mutate({
      lines: [
        { grossAmount: gross, discount: value(discountKind, discountValue) },
      ],
      documentDiscount: value(docKind, docValue),
    })
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
      <Card>
        <CardHeader>
          <CardTitle>Data transaksi</CardTitle>
          <CardDescription>
            Harga diambil dari versi yang berlaku pada tanggal transaksi.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              run()
            }}
          >
            <FieldGroup>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field
                  data-invalid={!!errors.productId}
                  className="sm:col-span-2"
                >
                  <FieldLabel htmlFor="sim-product">Produk</FieldLabel>
                  <ProductPicker
                    id="sim-product"
                    value={product}
                    onValueChange={setProduct}
                    types={["finished_good"]}
                    aria-invalid={!!errors.productId}
                  />
                  <FieldError errors={fieldError(errors, "productId")} />
                </Field>
                <Field data-invalid={!!errors.qty}>
                  <FieldLabel htmlFor="sim-qty">Qty</FieldLabel>
                  <QuantityInput
                    id="sim-qty"
                    value={qty}
                    onValueChange={setQty}
                    aria-invalid={!!errors.qty}
                  />
                  <FieldError errors={fieldError(errors, "qty")} />
                </Field>
                <Field data-invalid={!!errors.date}>
                  <FieldLabel htmlFor="sim-date">Tanggal transaksi</FieldLabel>
                  <DateInput
                    id="sim-date"
                    value={date}
                    onValueChange={setDate}
                    aria-invalid={!!errors.date}
                  />
                  <FieldError errors={fieldError(errors, "date")} />
                </Field>
                <ChannelFields
                  idPrefix="sim"
                  channelId={channelId}
                  subChannelId={subChannelId}
                  allowAll
                  onChange={(c, s) => {
                    setChannelId(c)
                    setSubChannelId(s)
                  }}
                  errors={{ subChannelId: errors.subChannelId }}
                />
                <Field>
                  <FieldLabel htmlFor="sim-customer">
                    Pelanggan (opsional)
                  </FieldLabel>
                  <SearchPicker
                    id="sim-customer"
                    value={customer}
                    onValueChange={setCustomer}
                    query={customerOptionsQuery}
                    toOption={(c) => ({
                      id: c.id,
                      label: c.fullName,
                      description: c.code,
                    })}
                    clearable
                    placeholder="Pilih pelanggan"
                    searchPlaceholder="Cari nama, kode, atau HP"
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="sim-partner">
                    Mitra (opsional)
                  </FieldLabel>
                  <SearchPicker
                    id="sim-partner"
                    value={partner}
                    onValueChange={setPartner}
                    query={partnerOptionsQuery}
                    toOption={(p) => ({
                      id: p.id,
                      label: p.name,
                      description: p.code,
                    })}
                    clearable
                    placeholder="Pilih mitra"
                    searchPlaceholder="Cari kode atau nama mitra"
                  />
                </Field>
              </div>
              <div>
                <LoadingButton
                  type="submit"
                  loading={quote.isPending}
                  loadingText="Menghitung…"
                >
                  Hitung Harga
                </LoadingButton>
              </div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>
      <div className="flex flex-col gap-4">
        <QuoteCard quote={quote.data} qty={qty} gross={gross} />
        {quote.data ? (
          <Card>
            <CardHeader>
              <CardTitle>Cek diskon manual</CardTitle>
              <CardDescription>
                Dibandingkan dengan batas diskon role Anda.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <ChoiceField
                  id="sim-line-kind"
                  label="Diskon baris"
                  value={discountKind}
                  onValueChange={setDiscountKind}
                  labels={DISCOUNT_KIND_LABELS}
                />
                <DiscountValueInput
                  id="sim-line-value"
                  label="Nilai"
                  kind={discountKind}
                  value={discountValue}
                  onChange={setDiscountValue}
                />
                <ChoiceField
                  id="sim-doc-kind"
                  label="Diskon dokumen"
                  value={docKind}
                  onValueChange={setDocKind}
                  labels={DISCOUNT_KIND_LABELS}
                />
                <DiscountValueInput
                  id="sim-doc-value"
                  label="Nilai"
                  kind={docKind}
                  value={docValue}
                  onChange={setDocValue}
                />
              </div>
              <div>
                <LoadingButton
                  variant="outline"
                  loading={discount.isPending}
                  loadingText="Memeriksa…"
                  onClick={runDiscount}
                >
                  Cek Batas Diskon
                </LoadingButton>
              </div>
              {discount.data ? <DiscountResult result={discount.data} /> : null}
            </CardContent>
          </Card>
        ) : null}
      </div>
    </div>
  )
}

function DiscountValueInput({
  id,
  label,
  kind,
  value,
  onChange,
}: {
  id: string
  label: string
  kind: DiscountKind
  value: string
  onChange: (v: string) => void
}) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      {kind === "amount" ? (
        <MoneyInput id={id} value={value} onValueChange={onChange} />
      ) : (
        <Input
          id={id}
          inputMode="decimal"
          placeholder="mis. 5"
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </Field>
  )
}

function QuoteCard({
  quote,
  qty,
  gross,
}: {
  quote?: PriceQuote
  qty: string
  gross: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Hasil</CardTitle>
        <CardDescription>
          Prioritas: harga khusus → promo → harga bertingkat → daftar harga →
          harga dasar.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {quote ? (
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Harga satuan</dt>
            <dd className="text-lg font-semibold tabular-nums">
              {formatRupiah(quote.unitPrice)}
            </dd>
            <dt className="text-muted-foreground">Aturan</dt>
            <dd>
              <Badge variant="secondary">{PRICE_RULE_LABELS[quote.rule]}</Badge>
            </dd>
            <dt className="text-muted-foreground">Asal harga</dt>
            <dd>{quote.description}</dd>
            {quote.regularPrice ? (
              <>
                <dt className="text-muted-foreground">Harga normal</dt>
                <dd className="tabular-nums">
                  {formatRupiah(quote.regularPrice)}
                </dd>
              </>
            ) : null}
            {quote.maxRetailPrice ? (
              <>
                <dt className="text-muted-foreground">HET</dt>
                <dd className="tabular-nums">
                  {formatRupiah(quote.maxRetailPrice)}
                </dd>
              </>
            ) : null}
            <dt className="text-muted-foreground">Total ({qty || 0})</dt>
            <dd className="tabular-nums">{formatRupiah(gross)}</dd>
            <dt className="text-muted-foreground">ID sumber</dt>
            <dd className="font-mono text-xs break-all">{quote.sourceId}</dd>
          </dl>
        ) : (
          <p className="text-sm text-muted-foreground">
            Isi data transaksi lalu pilih Hitung Harga.
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function DiscountResult({ result }: { result: DiscountCheckResult }) {
  const line = result.lines[0]
  return (
    <div className="flex flex-col gap-2 rounded-md border p-3 text-sm">
      <p>
        Batas Anda: baris {formatPercent(result.maxLineDiscountPercent)},
        dokumen {formatPercent(result.maxDocumentDiscountPercent)}.
      </p>
      {line ? (
        <p>
          Diskon baris {formatRupiah(line.discountAmount)} (
          {formatPercent(line.discountPercent)})
          {line.exceedsLimit ? " — di atas batas" : ""}
        </p>
      ) : null}
      <p>
        Diskon dokumen {formatRupiah(result.document.discountAmount)} (
        {formatPercent(result.document.discountPercent)}) dari subtotal{" "}
        {formatRupiah(result.subtotalAmount)}
        {result.document.exceedsLimit ? " — di atas batas" : ""}
      </p>
      {result.requiresApproval ? (
        <p className="font-medium text-amber-700 dark:text-amber-300">
          Butuh persetujuan Head Sales (Diskon Sales Order).
        </p>
      ) : (
        <p className="font-medium text-emerald-700 dark:text-emerald-300">
          Masih dalam batas; tidak butuh persetujuan.
        </p>
      )}
    </div>
  )
}

function DiscountLimits({
  includeEnded,
  onIncludeEndedChange,
}: {
  includeEnded: boolean
  onIncludeEndedChange: (v: boolean) => void
}) {
  const { can } = usePermissions()
  const canWrite = can("pricing.discount_limit.write")
  const list = useQuery(discountLimitsQuery(includeEnded))
  const [creating, setCreating] = useState(false)
  const actions = useDatedActions("discount-limits", [
    "pricing",
    "discount-limits",
  ])
  const today = todayBusinessDate()
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Checkbox
          label="Tampilkan yang sudah berakhir"
          checked={includeEnded}
          onChange={onIncludeEndedChange}
        />
        {canWrite ? (
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Atur Batas Diskon
          </Button>
        ) : null}
      </div>
      <LoadingArea loading={list.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Role</TableHead>
                <TableHead className="text-right">Maks. per baris</TableHead>
                <TableHead className="text-right">Maks. per dokumen</TableHead>
                <TableHead>Periode</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Catatan</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(list.data ?? []).map((l) => (
                <TableRow key={l.id}>
                  <TableCell className="font-medium">{l.roleName}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatPercent(l.maxLineDiscountPercent)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatPercent(l.maxDocumentDiscountPercent)}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {formatPeriod(l.validFrom, l.validTo)}
                  </TableCell>
                  <TableCell>
                    <PeriodBadge status={l.status} />
                  </TableCell>
                  <TableCell className="max-w-64 truncate">
                    {l.notes || "—"}
                  </TableCell>
                  <TableCell>
                    {canWrite ? (
                      <RowMenu
                        label={`Aksi untuk batas diskon ${l.roleName}`}
                        items={datedMenu(l, today, actions)}
                      />
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
              {list.data?.length === 0 ? (
                <EmptyRow
                  colSpan={7}
                  text="Belum ada batas diskon. Role tanpa batas: setiap diskon manual butuh persetujuan."
                />
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <DiscountLimitFormDialog open={creating} onOpenChange={setCreating} />
      <DatedDialogs
        actions={actions}
        resource="discount-limits"
        what="batas diskon"
        queryKey={["pricing", "discount-limits"]}
      />
    </div>
  )
}
