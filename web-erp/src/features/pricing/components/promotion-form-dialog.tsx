import { useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { Checkbox } from "@/features/accounting/components/account-form-dialog"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import {
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "@/features/party/components/form-parts"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"

import {
  createPromotion,
  parsePercentInput,
  promotionQuery,
  updatePromotion,
  type Promotion,
  type PromotionInput,
} from "../api/pricing-api"
import { ChannelFields } from "./pricing-parts"

type PromotionFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Promo yang diubah; kosong = promo baru. */
  promotionId?: string
}

/** Tambah/Ubah Promo periode per channel (S-118). */
export function PromotionFormDialog(props: PromotionFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <PromotionLoader {...props} /> : null}
    </Dialog>
  )
}

function PromotionLoader(props: PromotionFormDialogProps) {
  const promo = useQuery({
    ...promotionQuery(props.promotionId ?? ""),
    enabled: !!props.promotionId,
  })
  return (
    <DialogContent className="max-h-[92svh] overflow-y-auto sm:max-w-3xl">
      <LoadingArea loading={!!props.promotionId && promo.isPending}>
        {!props.promotionId || promo.data ? (
          <PromotionForm {...props} promotion={promo.data} />
        ) : null}
      </LoadingArea>
    </DialogContent>
  )
}

type Mode = "price" | "percent"

type Row = {
  key: string
  product: ProductOption | null
  mode: Mode
  price: string
  percent: string
}

let seq = 0
const newRow = (r?: Partial<Row>): Row => ({
  key: `promo-row-${++seq}`,
  product: null,
  mode: "price",
  price: "",
  percent: "",
  ...r,
})

const MODE_LABELS: Record<Mode, string> = {
  price: "Harga promo",
  percent: "Potongan %",
}

function PromotionForm({
  onOpenChange,
  promotion,
}: PromotionFormDialogProps & { promotion?: Promotion }) {
  const queryClient = useQueryClient()
  const started = !!promotion && promotion.validFrom <= todayBusinessDate()
  const [code, setCode] = useState(promotion?.code ?? "")
  const [name, setName] = useState(promotion?.name ?? "")
  const [channelId, setChannelId] = useState(promotion?.channelId ?? null)
  const [subChannelId, setSubChannelId] = useState(
    promotion?.subChannelId ?? null
  )
  const [validFrom, setValidFrom] = useState(promotion?.validFrom ?? "")
  const [validTo, setValidTo] = useState(promotion?.validTo ?? "")
  const [isActive, setIsActive] = useState(promotion?.isActive ?? true)
  const [description, setDescription] = useState(promotion?.description ?? "")
  const [rows, setRows] = useState<Row[]>(() =>
    promotion?.items?.length
      ? promotion.items.map((it) =>
          newRow({
            product: {
              id: it.productId,
              sku: it.productSku,
              name: it.productName,
              variant: "",
              uom: "",
            },
            mode: it.discountPercent ? "percent" : "price",
            price: it.priceAmount ?? "",
            percent: it.discountPercent
              ? String(Number(it.discountPercent)).replace(".", ",")
              : "",
          })
        )
      : [newRow()]
  )
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (body: PromotionInput, key) =>
      promotion
        ? updatePromotion(
            promotion.id,
            {
              ...body,
              items: started ? [] : body.items,
              version: promotion.version,
            },
            key
          )
        : createPromotion(body, key),
    onSuccess: async () => {
      toast.success(promotion ? "Promo diperbarui" : "Promo dibuat")
      await queryClient.invalidateQueries({ queryKey: ["pricing"] })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const update = (key: string, patch: Partial<Row>) =>
    setRows((cur) => cur.map((x) => (x.key === key ? { ...x, ...patch } : x)))

  const submit = () => {
    const next: FieldErrors = {}
    if (!promotion && !code.trim()) next.code = "Kode promo wajib diisi"
    if (!name.trim()) next.name = "Nama promo wajib diisi"
    if (!validFrom) next.validFrom = "Tanggal mulai wajib diisi"
    if (!validTo) next.validTo = "Tanggal selesai wajib diisi"
    if (validFrom && validTo && validTo < validFrom)
      next.validTo = "Tanggal selesai tidak boleh sebelum tanggal mulai"
    rows.forEach((r, i) => {
      if (!r.product) next[`items.${i}.productId`] = "Pilih produk"
      if (r.mode === "price" && !r.price)
        next[`items.${i}.priceAmount`] = "Harga promo wajib diisi"
      if (r.mode === "percent" && !r.percent.trim())
        next[`items.${i}.discountPercent`] = "Potongan wajib diisi"
    })
    if (rows.length === 0) next.items = "Isi minimal satu produk promo"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate({
      code,
      name,
      channelId,
      subChannelId,
      validFrom,
      validTo,
      isActive,
      description,
      items: rows.map((r) => ({
        productId: r.product?.id ?? "",
        priceAmount: r.mode === "price" ? r.price : null,
        discountPercent:
          r.mode === "percent" ? parsePercentInput(r.percent) : null,
      })),
    })
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>
          {promotion ? `Ubah Promo ${promotion.code}` : "Tambah Promo"}
        </DialogTitle>
        <DialogDescription>
          {started
            ? "Promo sudah berjalan: hanya nama, keterangan, status, dan tanggal selesai yang bisa diubah."
            : "Promo berlaku pada periode dan channel yang dipilih; potongan persen dihitung dari harga normal."}
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
            <Field data-invalid={!!errors.code}>
              <FieldLabel htmlFor="promo-code">Kode</FieldLabel>
              <Input
                id="promo-code"
                value={code}
                disabled={!!promotion}
                placeholder="mis. PROMO-1111"
                aria-invalid={!!errors.code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
              />
              <FieldError errors={fieldError(errors, "code")} />
            </Field>
            <Field data-invalid={!!errors.name}>
              <FieldLabel htmlFor="promo-name">Nama</FieldLabel>
              <Input
                id="promo-name"
                value={name}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "name")} />
            </Field>
            <ChannelFields
              idPrefix="promo"
              channelId={channelId}
              subChannelId={subChannelId}
              allowAll
              disabled={started}
              onChange={(c, s) => {
                setChannelId(c)
                setSubChannelId(s)
              }}
              errors={{
                channelId: errors.channelId,
                subChannelId: errors.subChannelId,
              }}
            />
            <Field data-invalid={!!errors.validFrom}>
              <FieldLabel htmlFor="promo-from">Mulai</FieldLabel>
              <DateInput
                id="promo-from"
                value={validFrom}
                disabled={started}
                onValueChange={setValidFrom}
                aria-invalid={!!errors.validFrom}
              />
              <FieldError errors={fieldError(errors, "validFrom")} />
            </Field>
            <Field data-invalid={!!errors.validTo}>
              <FieldLabel htmlFor="promo-to">Selesai</FieldLabel>
              <DateInput
                id="promo-to"
                value={validTo}
                onValueChange={setValidTo}
                aria-invalid={!!errors.validTo}
              />
              <FieldError errors={fieldError(errors, "validTo")} />
            </Field>
          </div>
          <div className="flex flex-col gap-2">
            <div className="overflow-x-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="min-w-64">Produk</TableHead>
                    <TableHead className="min-w-36">Bentuk</TableHead>
                    <TableHead className="min-w-40">Nilai</TableHead>
                    <TableHead className="w-12" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r, i) => (
                    <TableRow key={r.key}>
                      <TableCell>
                        {started ? (
                          <span className="text-sm">
                            {r.product?.sku} · {r.product?.name}
                          </span>
                        ) : (
                          <ProductPicker
                            value={r.product}
                            types={["finished_good"]}
                            excludeIds={rows
                              .filter((x) => x.key !== r.key && x.product)
                              .map((x) => x.product?.id ?? "")}
                            aria-invalid={!!errors[`items.${i}.productId`]}
                            onValueChange={(p) => update(r.key, { product: p })}
                          />
                        )}
                        <FieldError
                          errors={fieldError(errors, `items.${i}.productId`)}
                        />
                      </TableCell>
                      <TableCell>
                        <Select
                          value={r.mode}
                          disabled={started}
                          onValueChange={(v) =>
                            v && update(r.key, { mode: v as Mode })
                          }
                        >
                          <SelectTrigger
                            className="w-full"
                            aria-label={`Bentuk promo baris ${i + 1}`}
                          >
                            <SelectValue>
                              {(v: Mode) => MODE_LABELS[v]}
                            </SelectValue>
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="price">Harga promo</SelectItem>
                            <SelectItem value="percent">Potongan %</SelectItem>
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        {r.mode === "price" ? (
                          <MoneyInput
                            aria-label={`Harga promo baris ${i + 1}`}
                            value={r.price}
                            disabled={started}
                            aria-invalid={!!errors[`items.${i}.priceAmount`]}
                            onValueChange={(v) => update(r.key, { price: v })}
                          />
                        ) : (
                          <Input
                            aria-label={`Potongan baris ${i + 1}`}
                            inputMode="decimal"
                            placeholder="mis. 10"
                            value={r.percent}
                            disabled={started}
                            aria-invalid={
                              !!errors[`items.${i}.discountPercent`]
                            }
                            onChange={(e) =>
                              update(r.key, { percent: e.target.value })
                            }
                          />
                        )}
                        <FieldError
                          errors={fieldError(
                            errors,
                            r.mode === "price"
                              ? `items.${i}.priceAmount`
                              : `items.${i}.discountPercent`
                          )}
                        />
                      </TableCell>
                      <TableCell>
                        {!started ? (
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="size-11"
                            aria-label={`Hapus baris ${i + 1}`}
                            onClick={() =>
                              setRows((cur) =>
                                cur.filter((x) => x.key !== r.key)
                              )
                            }
                          >
                            <Trash2Icon />
                          </Button>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <FieldError errors={fieldError(errors, "items")} />
            {!started ? (
              <div>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setRows((cur) => [...cur, newRow()])}
                >
                  <PlusIcon /> Tambah Produk
                </Button>
              </div>
            ) : null}
          </div>
          <Field>
            <FieldLabel htmlFor="promo-description">Keterangan</FieldLabel>
            <Textarea
              id="promo-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </Field>
          <Checkbox label="Aktif" checked={isActive} onChange={setIsActive} />
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
    </>
  )
}
