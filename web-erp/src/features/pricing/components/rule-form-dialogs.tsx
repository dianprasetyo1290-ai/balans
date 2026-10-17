import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import { partnerOptionsQuery } from "@/features/party/api/party-api"
import {
  ChoiceField,
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "@/features/party/components/form-parts"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"

import {
  PARTY_TYPE_LABELS,
  createDiscountLimit,
  createSpecialPrice,
  createTieredPrice,
  customerOptionsQuery,
  discountLimitRolesQuery,
  parsePercentInput,
  priceListOptionsQuery,
  type SpecialPricePartyType,
} from "../api/pricing-api"

type DialogProps = { open: boolean; onOpenChange: (open: boolean) => void }

const ALL = "__all"

function PeriodFields({
  idPrefix,
  validFrom,
  validTo,
  setValidFrom,
  setValidTo,
  errors,
}: {
  idPrefix: string
  validFrom: string
  validTo: string
  setValidFrom: (v: string) => void
  setValidTo: (v: string) => void
  errors: FieldErrors
}) {
  return (
    <>
      <Field data-invalid={!!errors.validFrom}>
        <FieldLabel htmlFor={`${idPrefix}-from`}>Berlaku mulai</FieldLabel>
        <DateInput
          id={`${idPrefix}-from`}
          value={validFrom}
          onValueChange={setValidFrom}
          aria-invalid={!!errors.validFrom}
        />
        <FieldDescription>
          Data lama tanpa tanggal berakhir ditutup otomatis sehari sebelumnya.
        </FieldDescription>
        <FieldError errors={fieldError(errors, "validFrom")} />
      </Field>
      <Field data-invalid={!!errors.validTo}>
        <FieldLabel htmlFor={`${idPrefix}-to`}>
          Berlaku sampai (opsional)
        </FieldLabel>
        <DateInput
          id={`${idPrefix}-to`}
          value={validTo}
          onValueChange={setValidTo}
          aria-invalid={!!errors.validTo}
        />
        <FieldError errors={fieldError(errors, "validTo")} />
      </Field>
    </>
  )
}

function checkPeriod(next: FieldErrors, validFrom: string, validTo: string) {
  if (!validFrom) next.validFrom = "Tanggal mulai berlaku wajib diisi"
  if (validFrom && validTo && validTo < validFrom)
    next.validTo = "Tanggal berakhir tidak boleh sebelum tanggal mulai"
}

function Footer({
  onCancel,
  loading,
}: {
  onCancel: () => void
  loading: boolean
}) {
  return (
    <DialogFooter className="mt-6">
      <Button type="button" variant="outline" onClick={onCancel}>
        Batal
      </Button>
      <LoadingButton type="submit" loading={loading} loadingText="Menyimpan…">
        Simpan
      </LoadingButton>
    </DialogFooter>
  )
}

// --- Harga khusus ------------------------------------------------------------

/** Tambah Harga Khusus pelanggan/mitra (prioritas 1, BRD 5.18). */
export function SpecialPriceFormDialog(props: DialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <SpecialPriceForm {...props} /> : null}
    </Dialog>
  )
}

function SpecialPriceForm({ onOpenChange }: DialogProps) {
  const queryClient = useQueryClient()
  const [partyType, setPartyType] = useState<SpecialPricePartyType>("customer")
  const [party, setParty] = useState<PickerOption | null>(null)
  const [product, setProduct] = useState<ProductOption | null>(null)
  const [price, setPrice] = useState("")
  const [validFrom, setValidFrom] = useState("")
  const [validTo, setValidTo] = useState("")
  const [notes, setNotes] = useState("")
  const [errors, setErrors] = useState<FieldErrors>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createSpecialPrice(
        {
          partyType,
          partyId: party?.id ?? "",
          productId: product?.id ?? "",
          priceAmount: price,
          validFrom,
          validTo: validTo || null,
          notes,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Harga khusus disimpan")
      await queryClient.invalidateQueries({
        queryKey: ["pricing", "special-prices"],
      })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })
  const submit = () => {
    const next: FieldErrors = {}
    if (!party) next.partyId = "Pilih pelanggan atau mitra"
    if (!product) next.productId = "Pilih produk"
    if (!price) next.priceAmount = "Harga khusus wajib diisi"
    checkPeriod(next, validFrom, validTo)
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Tambah Harga Khusus</DialogTitle>
        <DialogDescription>
          Harga khusus mengalahkan promo, harga bertingkat, dan daftar harga
          untuk pelanggan atau mitra tersebut.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <div className="grid gap-4 sm:grid-cols-2">
            <ChoiceField
              id="special-party-type"
              label="Untuk"
              value={partyType}
              onValueChange={(v) => {
                setPartyType(v)
                setParty(null)
              }}
              labels={PARTY_TYPE_LABELS}
              error={errors.partyType}
            />
            <Field data-invalid={!!errors.partyId}>
              <FieldLabel htmlFor="special-party">
                {PARTY_TYPE_LABELS[partyType]}
              </FieldLabel>
              {partyType === "customer" ? (
                <SearchPicker
                  id="special-party"
                  value={party}
                  onValueChange={setParty}
                  query={customerOptionsQuery}
                  toOption={(c) => ({
                    id: c.id,
                    label: c.fullName,
                    description: c.code,
                  })}
                  placeholder="Pilih pelanggan"
                  searchPlaceholder="Cari nama, kode, atau HP"
                  aria-invalid={!!errors.partyId}
                />
              ) : (
                <SearchPicker
                  id="special-party"
                  value={party}
                  onValueChange={setParty}
                  query={partnerOptionsQuery}
                  toOption={(p) => ({
                    id: p.id,
                    label: p.name,
                    description: p.code,
                  })}
                  placeholder="Pilih mitra"
                  searchPlaceholder="Cari kode atau nama mitra"
                  aria-invalid={!!errors.partyId}
                />
              )}
              <FieldError errors={fieldError(errors, "partyId")} />
            </Field>
            <Field data-invalid={!!errors.productId}>
              <FieldLabel htmlFor="special-product">Produk</FieldLabel>
              <ProductPicker
                id="special-product"
                value={product}
                onValueChange={setProduct}
                types={["finished_good"]}
                aria-invalid={!!errors.productId}
              />
              <FieldError errors={fieldError(errors, "productId")} />
            </Field>
            <Field data-invalid={!!errors.priceAmount}>
              <FieldLabel htmlFor="special-price">Harga khusus</FieldLabel>
              <MoneyInput
                id="special-price"
                value={price}
                onValueChange={setPrice}
                aria-invalid={!!errors.priceAmount}
              />
              <FieldError errors={fieldError(errors, "priceAmount")} />
            </Field>
            <PeriodFields
              idPrefix="special"
              validFrom={validFrom}
              validTo={validTo}
              setValidFrom={setValidFrom}
              setValidTo={setValidTo}
              errors={errors}
            />
          </div>
          <Field>
            <FieldLabel htmlFor="special-notes">Catatan</FieldLabel>
            <Textarea
              id="special-notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </Field>
        </FieldGroup>
        <Footer onCancel={() => onOpenChange(false)} loading={save.isPending} />
      </form>
    </DialogContent>
  )
}

// --- Harga bertingkat --------------------------------------------------------

/** Tambah Harga Bertingkat per qty minimum (prioritas 3, S-118). */
export function TieredPriceFormDialog(props: DialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <TieredPriceForm {...props} /> : null}
    </Dialog>
  )
}

function TieredPriceForm({ onOpenChange }: DialogProps) {
  const queryClient = useQueryClient()
  const lists = useQuery(priceListOptionsQuery())
  const [priceListId, setPriceListId] = useState<string | null>(null)
  const [product, setProduct] = useState<ProductOption | null>(null)
  const [minQty, setMinQty] = useState("")
  const [price, setPrice] = useState("")
  const [validFrom, setValidFrom] = useState("")
  const [validTo, setValidTo] = useState("")
  const [notes, setNotes] = useState("")
  const [errors, setErrors] = useState<FieldErrors>({})
  const choices = (lists.data ?? []).filter((l) => l.priceListType !== "base")
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createTieredPrice(
        {
          priceListId,
          productId: product?.id ?? "",
          minQty,
          priceAmount: price,
          validFrom,
          validTo: validTo || null,
          notes,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Harga bertingkat disimpan")
      await queryClient.invalidateQueries({
        queryKey: ["pricing", "tiered-prices"],
      })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })
  const submit = () => {
    const next: FieldErrors = {}
    if (!product) next.productId = "Pilih produk"
    if (!minQty || Number(minQty) <= 0)
      next.minQty = "Qty minimum harus lebih dari 0"
    if (!price) next.priceAmount = "Harga wajib diisi"
    checkPeriod(next, validFrom, validTo)
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Tambah Harga Bertingkat</DialogTitle>
        <DialogDescription>
          Harga satuan untuk pembelian minimal qty tertentu. Qty minimum
          terbesar yang terpenuhi dipakai.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.priceListId}>
              <FieldLabel htmlFor="tier-list">Daftar harga</FieldLabel>
              <Select
                value={priceListId ?? ALL}
                onValueChange={(v) =>
                  setPriceListId(!v || v === ALL ? null : (v as string))
                }
              >
                <SelectTrigger id="tier-list" className="w-full">
                  <SelectValue>
                    {(v: string) =>
                      v === ALL
                        ? "Semua daftar harga"
                        : (choices.find((c) => c.id === v)?.name ?? "—")
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>Semua daftar harga</SelectItem>
                  {choices.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.code} · {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError errors={fieldError(errors, "priceListId")} />
            </Field>
            <Field data-invalid={!!errors.productId}>
              <FieldLabel htmlFor="tier-product">Produk</FieldLabel>
              <ProductPicker
                id="tier-product"
                value={product}
                onValueChange={setProduct}
                types={["finished_good"]}
                aria-invalid={!!errors.productId}
              />
              <FieldError errors={fieldError(errors, "productId")} />
            </Field>
            <Field data-invalid={!!errors.minQty}>
              <FieldLabel htmlFor="tier-qty">Qty minimum</FieldLabel>
              <QuantityInput
                id="tier-qty"
                value={minQty}
                onValueChange={setMinQty}
                aria-invalid={!!errors.minQty}
              />
              <FieldError errors={fieldError(errors, "minQty")} />
            </Field>
            <Field data-invalid={!!errors.priceAmount}>
              <FieldLabel htmlFor="tier-price">Harga satuan</FieldLabel>
              <MoneyInput
                id="tier-price"
                value={price}
                onValueChange={setPrice}
                aria-invalid={!!errors.priceAmount}
              />
              <FieldError errors={fieldError(errors, "priceAmount")} />
            </Field>
            <PeriodFields
              idPrefix="tier"
              validFrom={validFrom}
              validTo={validTo}
              setValidFrom={setValidFrom}
              setValidTo={setValidTo}
              errors={errors}
            />
          </div>
          <Field>
            <FieldLabel htmlFor="tier-notes">Catatan</FieldLabel>
            <Textarea
              id="tier-notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </Field>
        </FieldGroup>
        <Footer onCancel={() => onOpenChange(false)} loading={save.isPending} />
      </form>
    </DialogContent>
  )
}

// --- Batas diskon ------------------------------------------------------------

/** Atur Batas Diskon per role (MD-12, S-119). */
export function DiscountLimitFormDialog(props: DialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <DiscountLimitForm {...props} /> : null}
    </Dialog>
  )
}

function DiscountLimitForm({ onOpenChange }: DialogProps) {
  const queryClient = useQueryClient()
  const roles = useQuery(discountLimitRolesQuery())
  const [roleCode, setRoleCode] = useState<string | null>(null)
  const [line, setLine] = useState("")
  const [doc, setDoc] = useState("")
  const [validFrom, setValidFrom] = useState("")
  const [validTo, setValidTo] = useState("")
  const [notes, setNotes] = useState("")
  const [errors, setErrors] = useState<FieldErrors>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      createDiscountLimit(
        {
          roleCode: roleCode ?? "",
          maxLineDiscountPercent: parsePercentInput(line),
          maxDocumentDiscountPercent: parsePercentInput(doc),
          validFrom,
          validTo: validTo || null,
          notes,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Batas diskon disimpan")
      await queryClient.invalidateQueries({
        queryKey: ["pricing", "discount-limits"],
      })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })
  const submit = () => {
    const next: FieldErrors = {}
    if (!roleCode) next.roleCode = "Pilih role"
    if (!line.trim())
      next.maxLineDiscountPercent = "Batas per baris wajib diisi"
    if (!doc.trim())
      next.maxDocumentDiscountPercent = "Batas per dokumen wajib diisi"
    checkPeriod(next, validFrom, validTo)
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const roleName = (code: string) =>
    roles.data?.find((r) => r.code === code)?.name ?? code
  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>Atur Batas Diskon</DialogTitle>
        <DialogDescription>
          Diskon manual di atas batas role pembuat membuat Sales Order butuh
          persetujuan Head Sales. User dengan beberapa role memakai batas
          terbesar.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.roleCode} className="sm:col-span-2">
              <FieldLabel htmlFor="limit-role">Role</FieldLabel>
              <Select
                value={roleCode}
                onValueChange={(v) => setRoleCode((v as string) ?? null)}
              >
                <SelectTrigger
                  id="limit-role"
                  className="w-full"
                  aria-invalid={!!errors.roleCode}
                >
                  <SelectValue>
                    {(v: string | null) => (v ? roleName(v) : "Pilih role")}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {(roles.data ?? []).map((r) => (
                    <SelectItem key={r.code} value={r.code}>
                      {r.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError errors={fieldError(errors, "roleCode")} />
            </Field>
            <Field data-invalid={!!errors.maxLineDiscountPercent}>
              <FieldLabel htmlFor="limit-line">
                Maks. diskon per baris (%)
              </FieldLabel>
              <Input
                id="limit-line"
                inputMode="decimal"
                placeholder="mis. 5"
                value={line}
                aria-invalid={!!errors.maxLineDiscountPercent}
                onChange={(e) => setLine(e.target.value)}
              />
              <FieldError
                errors={fieldError(errors, "maxLineDiscountPercent")}
              />
            </Field>
            <Field data-invalid={!!errors.maxDocumentDiscountPercent}>
              <FieldLabel htmlFor="limit-doc">
                Maks. diskon per dokumen (%)
              </FieldLabel>
              <Input
                id="limit-doc"
                inputMode="decimal"
                placeholder="mis. 2"
                value={doc}
                aria-invalid={!!errors.maxDocumentDiscountPercent}
                onChange={(e) => setDoc(e.target.value)}
              />
              <FieldError
                errors={fieldError(errors, "maxDocumentDiscountPercent")}
              />
            </Field>
            <PeriodFields
              idPrefix="limit"
              validFrom={validFrom}
              validTo={validTo}
              setValidFrom={setValidFrom}
              setValidTo={setValidTo}
              errors={errors}
            />
          </div>
          <Field>
            <FieldLabel htmlFor="limit-notes">Catatan</FieldLabel>
            <Textarea
              id="limit-notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </Field>
        </FieldGroup>
        <Footer onCancel={() => onOpenChange(false)} loading={save.isPending} />
      </form>
    </DialogContent>
  )
}
