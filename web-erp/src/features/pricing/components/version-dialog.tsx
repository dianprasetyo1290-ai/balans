import { useQuery, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
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
import {
  Field,
  FieldDescription,
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
import { Textarea } from "@/components/ui/textarea"
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
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createPriceListVersion,
  formatPercent,
  parsePercentInput,
  priceListVersionQuery,
  transitionPriceListVersion,
  updatePriceListVersion,
  type PriceListDetail,
  type PriceListVersion,
  type VersionAction,
} from "../api/pricing-api"
import { formatPeriod } from "./pricing-parts"

type VersionDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  priceList: PriceListDetail
  /** Versi yang dibuka; kosong = menyiapkan versi baru (harga disalin dari versi berlaku/terjadwal). */
  versionId?: string
}

/**
 * Versi daftar harga: editor harga per produk untuk draft, tampilan baca untuk versi yang diajukan/berlaku. Versi
 * baru berlaku setelah disetujui (approval price_list_version).
 */
export function VersionDialog(props: VersionDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <VersionLoader {...props} /> : null}
    </Dialog>
  )
}

function VersionLoader(props: VersionDialogProps) {
  const sourceId =
    props.versionId ??
    props.priceList.nextVersion?.id ??
    props.priceList.currentVersion?.id
  const source = useQuery({
    ...priceListVersionQuery(sourceId ?? ""),
    enabled: !!sourceId,
  })
  const ready = !sourceId || source.data
  return (
    <DialogContent className="max-h-[92svh] overflow-y-auto sm:max-w-4xl">
      <LoadingArea loading={!!sourceId && source.isPending}>
        {ready ? (
          <VersionForm
            {...props}
            version={props.versionId ? source.data : undefined}
            copyFrom={props.versionId ? undefined : source.data}
          />
        ) : null}
      </LoadingArea>
    </DialogContent>
  )
}

type Row = {
  key: string
  product: ProductOption | null
  price: string
  het: string
}

let rowSeq = 0
const newRow = (r?: Partial<Row>): Row => ({
  key: `row-${++rowSeq}`,
  product: null,
  price: "",
  het: "",
  ...r,
})

function rowsFrom(v?: PriceListVersion): Row[] {
  return (v?.items ?? []).map((it) =>
    newRow({
      product: {
        id: it.productId,
        sku: it.productSku,
        name: it.productName,
        variant: "",
        uom: "",
      },
      price: it.priceAmount,
      het: it.maxRetailPriceAmount ?? "",
    })
  )
}

function VersionForm({
  onOpenChange,
  priceList,
  version,
  copyFrom,
}: VersionDialogProps & {
  version?: PriceListVersion
  copyFrom?: PriceListVersion
}) {
  const queryClient = useQueryClient()
  const editable = !version || version.allowedActions.includes("update")
  const isBase = priceList.priceListType === "base"
  const isPercent = priceList.basis === "percent"
  const seed = version ?? copyFrom
  const [validFrom, setValidFrom] = useState(version?.validFrom ?? "")
  const [validTo, setValidTo] = useState(version?.validTo ?? "")
  const [percent, setPercent] = useState(
    seed?.percent ? String(Number(seed.percent)).replace(".", ",") : ""
  )
  const [notes, setNotes] = useState(version?.notes ?? "")
  const [rows, setRows] = useState<Row[]>(() => {
    const r = rowsFrom(seed)
    return r.length > 0 || isPercent ? r : [newRow()]
  })
  const [errors, setErrors] = useState<FieldErrors>({})

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["pricing"] })

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        validFrom,
        validTo: validTo || null,
        percent: isPercent ? parsePercentInput(percent) || null : null,
        notes,
        items: isPercent
          ? []
          : rows
              .filter((r) => r.product || r.price)
              .map((r) => ({
                productId: r.product?.id ?? "",
                priceAmount: r.price,
                maxRetailPriceAmount: isBase && r.het ? r.het : null,
              })),
      }
      return version
        ? updatePriceListVersion(
            version.id,
            { ...body, version: version.version },
            key
          )
        : createPriceListVersion(priceList.id, body, key)
    },
    onSuccess: async (saved) => {
      toast.success(version ? "Versi draft disimpan" : "Versi draft dibuat")
      await invalidate()
      if (!version) {
        onOpenChange(false)
        return
      }
      queryClient.setQueryData(priceListVersionQuery(saved.id).queryKey, saved)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const transition = useIdempotentMutation({
    mutationFn: (action: VersionAction, key) =>
      transitionPriceListVersion(
        version?.id ?? "",
        action,
        version?.version ?? 0,
        key
      ),
    onSuccess: async (_, action) => {
      toast.success(
        action === "submit"
          ? "Versi diajukan untuk persetujuan"
          : action === "withdraw"
            ? "Pengajuan ditarik kembali"
            : "Versi dibatalkan"
      )
      await invalidate()
      onOpenChange(false)
    },
    onError: (err) => {
      const p = parseProblem(err)
      if (p.code === "VALIDATION_FAILED" && p.fields) {
        setErrors(p.fields)
        return
      }
      showErrorDialog(p)
    },
  })

  const submitForm = () => {
    const next: FieldErrors = {}
    if (!validFrom) next.validFrom = "Tanggal mulai berlaku wajib diisi"
    if (validTo && validFrom && validTo < validFrom)
      next.validTo = "Tanggal berakhir tidak boleh sebelum tanggal mulai"
    if (isPercent && !percent.trim()) next.percent = "Persentase wajib diisi"
    rows.forEach((r, i) => {
      if (!r.product && !r.price) return
      if (!r.product) next[`items.${i}.productId`] = "Pilih produk"
      if (!r.price) next[`items.${i}.priceAmount`] = "Harga wajib diisi"
    })
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate()
  }

  const title = version
    ? `${priceList.code} v${version.versionNo}`
    : `Versi baru ${priceList.code}`

  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex flex-wrap items-center gap-2">
          {title}
          {version ? <StatusBadge status={version.status} /> : null}
        </DialogTitle>
        <DialogDescription>
          {version && !editable
            ? `Berlaku ${formatPeriod(version.validFrom, version.validTo)}. Versi yang sudah diajukan atau berlaku tidak bisa diubah; buat versi baru untuk perubahan harga.`
            : copyFrom
              ? `Harga disalin dari v${copyFrom.versionNo}; ubah baris yang berbeda saja. Versi berlaku setelah disetujui Head Sales dan Head Finance & Accounting.`
              : "Isi harga per produk. Versi berlaku setelah disetujui Head Sales dan Head Finance & Accounting."}
        </DialogDescription>
      </DialogHeader>
      {editable ? (
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            submitForm()
          }}
        >
          <FieldGroup>
            <div className="grid gap-4 sm:grid-cols-3">
              <Field data-invalid={!!errors.validFrom}>
                <FieldLabel htmlFor="version-from">Berlaku mulai</FieldLabel>
                <DateInput
                  id="version-from"
                  value={validFrom}
                  onValueChange={setValidFrom}
                  aria-invalid={!!errors.validFrom}
                />
                <FieldError errors={fieldError(errors, "validFrom")} />
              </Field>
              <Field data-invalid={!!errors.validTo}>
                <FieldLabel htmlFor="version-to">
                  Berlaku sampai (opsional)
                </FieldLabel>
                <DateInput
                  id="version-to"
                  value={validTo}
                  onValueChange={setValidTo}
                  aria-invalid={!!errors.validTo}
                />
                <FieldError errors={fieldError(errors, "validTo")} />
              </Field>
              {isPercent ? (
                <Field data-invalid={!!errors.percent}>
                  <FieldLabel htmlFor="version-percent">
                    Persen dari {priceList.referencePriceListName ?? "acuan"}
                  </FieldLabel>
                  <Input
                    id="version-percent"
                    inputMode="decimal"
                    placeholder="mis. 90"
                    value={percent}
                    aria-invalid={!!errors.percent}
                    onChange={(e) => setPercent(e.target.value)}
                  />
                  <FieldDescription>
                    Harga dibulatkan ke rupiah penuh.
                  </FieldDescription>
                  <FieldError errors={fieldError(errors, "percent")} />
                </Field>
              ) : null}
            </div>
            {!isPercent ? (
              <div className="flex flex-col gap-2">
                <div className="overflow-x-auto rounded-lg border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="min-w-64">Produk</TableHead>
                        <TableHead className="min-w-40">
                          {isBase ? "Harga dasar" : "Harga"}
                        </TableHead>
                        {isBase ? (
                          <TableHead className="min-w-40">HET</TableHead>
                        ) : null}
                        <TableHead className="w-12" />
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {rows.map((r, i) => (
                        <TableRow key={r.key}>
                          <TableCell>
                            <ProductPicker
                              value={r.product}
                              types={["finished_good"]}
                              excludeIds={rows
                                .filter((x) => x.key !== r.key && x.product)
                                .map((x) => x.product?.id ?? "")}
                              aria-invalid={!!errors[`items.${i}.productId`]}
                              onValueChange={(p) =>
                                setRows((cur) =>
                                  cur.map((x) =>
                                    x.key === r.key ? { ...x, product: p } : x
                                  )
                                )
                              }
                            />
                            <FieldError
                              errors={fieldError(
                                errors,
                                `items.${i}.productId`
                              )}
                            />
                          </TableCell>
                          <TableCell>
                            <MoneyInput
                              aria-label={`Harga baris ${i + 1}`}
                              value={r.price}
                              aria-invalid={!!errors[`items.${i}.priceAmount`]}
                              onValueChange={(v) =>
                                setRows((cur) =>
                                  cur.map((x) =>
                                    x.key === r.key ? { ...x, price: v } : x
                                  )
                                )
                              }
                            />
                            <FieldError
                              errors={fieldError(
                                errors,
                                `items.${i}.priceAmount`
                              )}
                            />
                          </TableCell>
                          {isBase ? (
                            <TableCell>
                              <MoneyInput
                                aria-label={`HET baris ${i + 1}`}
                                value={r.het}
                                aria-invalid={
                                  !!errors[`items.${i}.maxRetailPriceAmount`]
                                }
                                onValueChange={(v) =>
                                  setRows((cur) =>
                                    cur.map((x) =>
                                      x.key === r.key ? { ...x, het: v } : x
                                    )
                                  )
                                }
                              />
                              <FieldError
                                errors={fieldError(
                                  errors,
                                  `items.${i}.maxRetailPriceAmount`
                                )}
                              />
                            </TableCell>
                          ) : null}
                          <TableCell>
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
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
                <FieldError errors={fieldError(errors, "items")} />
                <div>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setRows((cur) => [...cur, newRow()])}
                  >
                    <PlusIcon /> Tambah Produk
                  </Button>
                </div>
              </div>
            ) : null}
            <Field data-invalid={!!errors.notes}>
              <FieldLabel htmlFor="version-notes">Catatan perubahan</FieldLabel>
              <Textarea
                id="version-notes"
                value={notes}
                placeholder="mis. Kenaikan harga 3% mulai November"
                onChange={(e) => setNotes(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "notes")} />
            </Field>
          </FieldGroup>
          <DialogFooter className="mt-6 flex-wrap">
            {version?.allowedActions.includes("cancel") ? (
              <LoadingButton
                type="button"
                variant="outline"
                className="mr-auto"
                loading={
                  transition.isPending && transition.variables === "cancel"
                }
                loadingText="Membatalkan…"
                onClick={() => transition.mutate("cancel")}
              >
                Batalkan Versi
              </LoadingButton>
            ) : null}
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Tutup
            </Button>
            <LoadingButton
              type="submit"
              variant={version ? "outline" : "default"}
              loading={save.isPending}
              loadingText="Menyimpan…"
            >
              Simpan Draft
            </LoadingButton>
            {version?.allowedActions.includes("submit") ? (
              <LoadingButton
                type="button"
                loading={
                  transition.isPending && transition.variables === "submit"
                }
                loadingText="Mengajukan…"
                onClick={() => transition.mutate("submit")}
              >
                Ajukan Persetujuan
              </LoadingButton>
            ) : null}
          </DialogFooter>
        </form>
      ) : version ? (
        <ReadOnlyVersion
          version={version}
          isBase={isBase}
          isPercent={isPercent}
          referenceName={priceList.referencePriceListName}
          onClose={() => onOpenChange(false)}
          withdrawing={transition.isPending}
          onWithdraw={
            version.allowedActions.includes("withdraw")
              ? () => transition.mutate("withdraw")
              : undefined
          }
        />
      ) : null}
    </>
  )
}

function ReadOnlyVersion({
  version,
  isBase,
  isPercent,
  referenceName,
  onClose,
  onWithdraw,
  withdrawing,
}: {
  version: PriceListVersion
  isBase: boolean
  isPercent: boolean
  referenceName?: string | null
  onClose: () => void
  onWithdraw?: () => void
  withdrawing: boolean
}) {
  return (
    <div className="flex flex-col gap-4">
      {isPercent ? (
        <p className="text-sm">
          Harga = {formatPercent(version.percent)} dari{" "}
          {referenceName ?? "daftar harga acuan"} (dibulatkan ke rupiah penuh).
        </p>
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>SKU</TableHead>
                <TableHead>Produk</TableHead>
                <TableHead className="text-right">
                  {isBase ? "Harga dasar" : "Harga"}
                </TableHead>
                {isBase ? (
                  <TableHead className="text-right">HET</TableHead>
                ) : null}
              </TableRow>
            </TableHeader>
            <TableBody>
              {version.items.map((it) => (
                <TableRow key={it.productId}>
                  <TableCell className="whitespace-nowrap">
                    {it.productSku || "—"}
                  </TableCell>
                  <TableCell>{it.productName || it.productId}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(it.priceAmount)}
                  </TableCell>
                  {isBase ? (
                    <TableCell className="text-right tabular-nums">
                      {it.maxRetailPriceAmount
                        ? formatRupiah(it.maxRetailPriceAmount)
                        : "—"}
                    </TableCell>
                  ) : null}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      {version.notes ? (
        <p className="text-sm text-muted-foreground">
          Catatan: {version.notes}
        </p>
      ) : null}
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Tutup
        </Button>
        {onWithdraw ? (
          <LoadingButton
            loading={withdrawing}
            loadingText="Menarik…"
            onClick={onWithdraw}
          >
            Tarik Pengajuan
          </LoadingButton>
        ) : null}
      </DialogFooter>
    </div>
  )
}
