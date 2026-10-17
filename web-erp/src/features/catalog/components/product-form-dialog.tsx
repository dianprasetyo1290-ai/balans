import { useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { LoadingButton } from "@/components/common/loading-button"
import { QuantityInput } from "@/components/common/quantity-input"
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
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  categoriesQuery,
  createProduct,
  PACKAGING_TYPE_LABELS,
  PRODUCT_KINDS,
  PRODUCT_TYPE_LABELS,
  updateProduct,
  type PackagingType,
  type Product,
  type ProductKind,
  type ProductType,
} from "../api/catalog-api"
import { ProductPicker, type ProductOption } from "./product-picker"

type ProductFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  kind: ProductKind
  /** Kosong = produk baru. */
  product?: Product
  onSaved?: (p: Product) => void
}

type BarcodeRow = { key: string; barcode: string; isPrimary: boolean }
type ComponentRow = { key: string; product: ProductOption | null; qty: string }

const NONE = "none"
const newKey = () => crypto.randomUUID()

/** Tambah/ubah Produk, Packaging & Material, atau Bundel (S-113, S-114, S-115). */
export function ProductFormDialog(props: ProductFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ProductForm {...props} /> : null}
    </Dialog>
  )
}

function ProductForm({
  onOpenChange,
  kind,
  product,
  onSaved,
}: ProductFormDialogProps) {
  const queryClient = useQueryClient()
  const categories = useQuery(categoriesQuery())
  const cfg = PRODUCT_KINDS[kind]
  const [sku, setSku] = useState(product?.sku ?? "")
  const [name, setName] = useState(product?.name ?? "")
  const [variant, setVariant] = useState(product?.variant ?? "")
  const [productType, setProductType] = useState<ProductType>(
    product?.productType ?? cfg.types[0]
  )
  const [packagingType, setPackagingType] = useState<string>(
    product?.packagingType ?? NONE
  )
  const [uom, setUom] = useState(product?.uom ?? "pcs")
  const [shelfLife, setShelfLife] = useState(
    product?.shelfLifeDays ? String(product.shelfLifeDays) : ""
  )
  const [unitsPerCarton, setUnitsPerCarton] = useState(
    product?.unitsPerCarton ? String(product.unitsPerCarton) : ""
  )
  const [categoryId, setCategoryId] = useState(product?.categoryId ?? NONE)
  const [minStock, setMinStock] = useState(product?.minStockQty ?? "")
  const [hasBatch, setHasBatch] = useState(product?.hasBatch ?? false)
  const [isActive, setIsActive] = useState(product?.isActive ?? true)
  const [barcodes, setBarcodes] = useState<BarcodeRow[]>(
    () =>
      product?.barcodes.map((b) => ({ ...b, key: newKey() })) ?? [
        { key: newKey(), barcode: "", isPrimary: true },
      ]
  )
  const [components, setComponents] = useState<ComponentRow[]>(() =>
    product?.bundleComponents.length
      ? product.bundleComponents.map((c) => ({
          key: newKey(),
          qty: c.qty,
          product: {
            id: c.componentProductId,
            sku: c.componentSku,
            name: c.componentName,
            variant: c.componentVariant,
            uom: c.componentUom,
          },
        }))
      : [{ key: newKey(), product: null, qty: "1.000" }]
  )
  const [errors, setErrors] = useState<Record<string, string>>({})

  const isBundle = cfg.isBundle
  const isFinished = productType === "finished_good" && !isBundle
  const showPackaging = !isBundle
  const packagingRequired = isFinished || productType === "packaging"

  const toInt = (s: string) => (s.trim() === "" ? null : Number(s))

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const fields = {
        name,
        variant,
        uom,
        isActive,
        hasBatch,
        packagingType:
          showPackaging && packagingType !== NONE
            ? (packagingType as PackagingType)
            : null,
        shelfLifeDays: isBundle ? null : toInt(shelfLife),
        unitsPerCarton: toInt(unitsPerCarton),
        categoryId: categoryId === NONE ? null : categoryId,
        minStockQty: isBundle || minStock === "" ? null : minStock,
        barcodes: barcodes
          .filter((b) => b.barcode.trim() !== "")
          .map((b) => ({ barcode: b.barcode.trim(), isPrimary: b.isPrimary })),
        bundleComponents: isBundle
          ? components.map((c) => ({
              componentProductId: c.product?.id ?? "",
              qty: c.qty || "0",
            }))
          : [],
      }
      return product
        ? updateProduct(
            product.id,
            { ...fields, version: product.version },
            key
          )
        : createProduct({ ...fields, sku, productType, isBundle }, key)
    },
    onSuccess: async (saved) => {
      toast.success(product ? `${cfg.noun} diperbarui` : `${cfg.noun} dibuat`)
      await queryClient.invalidateQueries({ queryKey: ["catalog"] })
      onOpenChange(false)
      onSaved?.(saved)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (
        (problem.code === "VALIDATION_FAILED" ||
          problem.code === "DUPLICATE_CODE") &&
        problem.fields
      ) {
        setErrors(problem.fields)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    const next: Record<string, string> = {}
    if (!product && !sku.trim()) next.sku = "SKU wajib diisi"
    if (!name.trim()) next.name = "Nama wajib diisi"
    if (!uom.trim()) next.uom = "Satuan wajib diisi"
    if (isFinished && !shelfLife) next.shelfLifeDays = "Umur simpan wajib diisi"
    if (showPackaging && packagingRequired && packagingType === NONE)
      next.packagingType = "Tipe kemasan wajib diisi"
    if (isBundle) {
      components.forEach((c, i) => {
        if (!c.product)
          next[`bundleComponents[${i}].componentProductId`] =
            "Produk komponen wajib dipilih"
        if (!c.qty || Number(c.qty) <= 0)
          next[`bundleComponents[${i}].qty`] = "Qty harus lebih dari 0"
      })
    }
    setErrors(next)
    if (Object.keys(next).length === 0) {
      save.mutate()
    }
  }

  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])
  const title = product
    ? `Ubah ${cfg.noun}`
    : kind === "bundle"
      ? "Tambah Bundel"
      : kind === "material"
        ? "Tambah Packaging/Material"
        : "Tambah Produk"

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>
          {product
            ? "SKU dan jenis produk tidak bisa diubah."
            : "SKU unik permanen dan tidak bisa dipakai ulang walau produk dihapus."}
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
            <Field data-invalid={!!errors.sku}>
              <FieldLabel htmlFor="product-sku">SKU</FieldLabel>
              <Input
                id="product-sku"
                value={sku}
                disabled={!!product}
                autoCapitalize="characters"
                aria-invalid={!!errors.sku}
                onChange={(e) => setSku(e.target.value.toUpperCase())}
              />
              <FieldError errors={err("sku")} />
            </Field>
            {kind === "material" ? (
              <Field data-invalid={!!errors.productType}>
                <FieldLabel htmlFor="product-type">Jenis</FieldLabel>
                <Select
                  value={productType}
                  disabled={!!product}
                  onValueChange={(v) => setProductType(v as ProductType)}
                >
                  <SelectTrigger id="product-type" className="w-full">
                    <SelectValue>
                      {(v: ProductType) => PRODUCT_TYPE_LABELS[v]}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {cfg.types.map((t) => (
                      <SelectItem key={t} value={t}>
                        {PRODUCT_TYPE_LABELS[t]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError errors={err("productType")} />
              </Field>
            ) : null}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.name}>
              <FieldLabel htmlFor="product-name">Nama</FieldLabel>
              <Input
                id="product-name"
                value={name}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
              <FieldError errors={err("name")} />
            </Field>
            <Field data-invalid={!!errors.variant}>
              <FieldLabel htmlFor="product-variant">
                Varian/rasa (opsional)
              </FieldLabel>
              <Input
                id="product-variant"
                value={variant}
                placeholder="mis. Coklat 30 g"
                onChange={(e) => setVariant(e.target.value)}
              />
              <FieldError errors={err("variant")} />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            {showPackaging ? (
              <Field data-invalid={!!errors.packagingType}>
                <FieldLabel htmlFor="product-packaging">
                  Tipe kemasan{packagingRequired ? "" : " (opsional)"}
                </FieldLabel>
                <Select
                  value={packagingType}
                  onValueChange={(v) => setPackagingType(String(v ?? NONE))}
                >
                  <SelectTrigger
                    id="product-packaging"
                    className="w-full"
                    aria-invalid={!!errors.packagingType}
                  >
                    <SelectValue>
                      {(v: string) =>
                        v === NONE
                          ? "Pilih tipe"
                          : PACKAGING_TYPE_LABELS[v as PackagingType]
                      }
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {packagingRequired ? null : (
                      <SelectItem value={NONE}>Tidak ada</SelectItem>
                    )}
                    {Object.entries(PACKAGING_TYPE_LABELS).map(([v, l]) => (
                      <SelectItem key={v} value={v}>
                        {l}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError errors={err("packagingType")} />
              </Field>
            ) : null}
            <Field data-invalid={!!errors.uom}>
              <FieldLabel htmlFor="product-uom">Satuan</FieldLabel>
              <Input
                id="product-uom"
                value={uom}
                aria-invalid={!!errors.uom}
                onChange={(e) => setUom(e.target.value)}
              />
              <FieldError errors={err("uom")} />
            </Field>
            <Field data-invalid={!!errors.unitsPerCarton}>
              <FieldLabel htmlFor="product-carton">
                Isi per karton (opsional)
              </FieldLabel>
              <Input
                id="product-carton"
                type="number"
                inputMode="numeric"
                min={1}
                value={unitsPerCarton}
                aria-invalid={!!errors.unitsPerCarton}
                onChange={(e) => setUnitsPerCarton(e.target.value)}
              />
              <FieldError errors={err("unitsPerCarton")} />
            </Field>
          </div>
          {isBundle ? null : (
            <div className="grid gap-4 sm:grid-cols-3">
              <Field data-invalid={!!errors.shelfLifeDays}>
                <FieldLabel htmlFor="product-shelf-life">
                  Umur simpan (hari){isFinished ? "" : " (opsional)"}
                </FieldLabel>
                <Input
                  id="product-shelf-life"
                  type="number"
                  inputMode="numeric"
                  min={1}
                  value={shelfLife}
                  aria-invalid={!!errors.shelfLifeDays}
                  onChange={(e) => setShelfLife(e.target.value)}
                />
                <FieldError errors={err("shelfLifeDays")} />
              </Field>
              <Field data-invalid={!!errors.minStockQty}>
                <FieldLabel htmlFor="product-min-stock">
                  Stok minimum (opsional)
                </FieldLabel>
                <QuantityInput
                  id="product-min-stock"
                  value={minStock}
                  onValueChange={setMinStock}
                  unit={uom}
                  aria-invalid={!!errors.minStockQty}
                />
                <FieldError errors={err("minStockQty")} />
              </Field>
              <Field data-invalid={!!errors.categoryId}>
                <FieldLabel htmlFor="product-category">Kategori</FieldLabel>
                <CategorySelect
                  value={categoryId}
                  onValueChange={setCategoryId}
                  categories={categories.data ?? []}
                  current={product}
                />
                <FieldError errors={err("categoryId")} />
              </Field>
            </div>
          )}
          {isBundle ? (
            <Field data-invalid={!!errors.categoryId}>
              <FieldLabel htmlFor="product-category">Kategori</FieldLabel>
              <CategorySelect
                value={categoryId}
                onValueChange={setCategoryId}
                categories={categories.data ?? []}
                current={product}
              />
              <FieldError errors={err("categoryId")} />
            </Field>
          ) : null}
          <div className="flex flex-wrap gap-4">
            {kind === "material" ? (
              <label className="flex min-h-11 items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  className="size-4 accent-(--color-primary)"
                  checked={hasBatch}
                  onChange={(e) => setHasBatch(e.target.checked)}
                />
                Lacak per batch
              </label>
            ) : null}
            <label className="flex min-h-11 items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-(--color-primary)"
                checked={isActive}
                onChange={(e) => setIsActive(e.target.checked)}
              />
              Aktif (bisa dipilih di transaksi baru)
            </label>
          </div>
          {isFinished ? (
            <p className="text-xs text-muted-foreground">
              Produk Jadi selalu dilacak per batch dan tanggal kedaluwarsa
              (FEFO).
            </p>
          ) : null}

          {isBundle ? (
            <FieldSet data-invalid={!!errors.bundleComponents}>
              <FieldLegend variant="label">Komponen</FieldLegend>
              <FieldDescription>
                Qty komponen per satu bundel. Bundel tidak punya stok sendiri;
                stok komponen terpotong saat bundel terjual.
              </FieldDescription>
              <div className="flex flex-col gap-2">
                {components.map((c, i) => (
                  <div
                    key={c.key}
                    className="grid grid-cols-[1fr_8rem_auto] items-start gap-2"
                  >
                    <div>
                      <ProductPicker
                        value={c.product}
                        types={["finished_good"]}
                        isBundle={false}
                        excludeIds={components
                          .filter((x) => x.key !== c.key && x.product)
                          .map((x) => x.product?.id ?? "")}
                        aria-invalid={
                          !!errors[`bundleComponents[${i}].componentProductId`]
                        }
                        onValueChange={(p) =>
                          setComponents((rows) =>
                            rows.map((r) =>
                              r.key === c.key ? { ...r, product: p } : r
                            )
                          )
                        }
                      />
                      <FieldError
                        errors={err(
                          `bundleComponents[${i}].componentProductId`
                        )}
                      />
                    </div>
                    <div>
                      <QuantityInput
                        aria-label={`Qty komponen ${i + 1}`}
                        value={c.qty}
                        unit={c.product?.uom}
                        aria-invalid={!!errors[`bundleComponents[${i}].qty`]}
                        onValueChange={(qty) =>
                          setComponents((rows) =>
                            rows.map((r) =>
                              r.key === c.key ? { ...r, qty } : r
                            )
                          )
                        }
                      />
                      <FieldError errors={err(`bundleComponents[${i}].qty`)} />
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`Hapus komponen ${i + 1}`}
                      disabled={components.length === 1}
                      onClick={() =>
                        setComponents((rows) =>
                          rows.filter((r) => r.key !== c.key)
                        )
                      }
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                ))}
              </div>
              <div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    setComponents((rows) => [
                      ...rows,
                      { key: newKey(), product: null, qty: "1.000" },
                    ])
                  }
                >
                  <PlusIcon /> Tambah Komponen
                </Button>
              </div>
              <FieldError errors={err("bundleComponents")} />
            </FieldSet>
          ) : null}

          <FieldSet data-invalid={!!errors.barcodes}>
            <FieldLegend variant="label">Barcode</FieldLegend>
            <FieldDescription>
              Barcode utama dicetak di label produk. 13 digit dengan digit
              periksa benar dicetak sebagai EAN-13, selain itu Code 128.
            </FieldDescription>
            <div className="flex flex-col gap-2">
              {barcodes.map((b, i) => (
                <div
                  key={b.key}
                  className="grid grid-cols-[1fr_auto_auto] items-start gap-2"
                >
                  <div>
                    <Input
                      aria-label={`Barcode ${i + 1}`}
                      value={b.barcode}
                      inputMode="text"
                      aria-invalid={!!errors[`barcodes[${i}].barcode`]}
                      onChange={(e) =>
                        setBarcodes((rows) =>
                          rows.map((r) =>
                            r.key === b.key
                              ? { ...r, barcode: e.target.value }
                              : r
                          )
                        )
                      }
                    />
                    <FieldError errors={err(`barcodes[${i}].barcode`)} />
                  </div>
                  <label className="flex min-h-9 items-center gap-1.5 text-sm">
                    <input
                      type="radio"
                      name="primary-barcode"
                      className="size-4 accent-(--color-primary)"
                      checked={b.isPrimary}
                      onChange={() =>
                        setBarcodes((rows) =>
                          rows.map((r) => ({
                            ...r,
                            isPrimary: r.key === b.key,
                          }))
                        )
                      }
                    />
                    Utama
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`Hapus barcode ${i + 1}`}
                    onClick={() =>
                      setBarcodes((rows) => {
                        const rest = rows.filter((r) => r.key !== b.key)
                        if (b.isPrimary && rest.length > 0) {
                          rest[0] = { ...rest[0], isPrimary: true }
                        }
                        return rest
                      })
                    }
                  >
                    <Trash2Icon />
                  </Button>
                </div>
              ))}
            </div>
            <div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  setBarcodes((rows) => [
                    ...rows,
                    {
                      key: newKey(),
                      barcode: "",
                      isPrimary: rows.length === 0,
                    },
                  ])
                }
              >
                <PlusIcon /> Tambah Barcode
              </Button>
            </div>
            <FieldError errors={err("barcodes")} />
          </FieldSet>
        </FieldGroup>
        <DialogFooter className="mt-6">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}

function CategorySelect({
  value,
  onValueChange,
  categories,
  current,
}: {
  value: string
  onValueChange: (v: string) => void
  categories: { id: string; code: string; name: string }[]
  current?: Product
}) {
  const label = (v: string) => {
    if (v === NONE) return "Tanpa kategori"
    const c = categories.find((x) => x.id === v)
    if (c) return c.name
    return current?.categoryId === v
      ? `${current.categoryName ?? "Kategori"} (dihapus)`
      : "Kategori"
  }
  return (
    <Select
      value={value}
      onValueChange={(v) => onValueChange(String(v ?? NONE))}
    >
      <SelectTrigger id="product-category" className="w-full">
        <SelectValue>{(v: string) => label(v)}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NONE}>Tanpa kategori</SelectItem>
        {categories.map((c) => (
          <SelectItem key={c.id} value={c.id}>
            {c.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
