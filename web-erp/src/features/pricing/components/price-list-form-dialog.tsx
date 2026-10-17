import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { LoadingButton } from "@/components/common/loading-button"
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
import { Checkbox } from "@/features/accounting/components/account-form-dialog"
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
  BASIS_LABELS,
  CREATABLE_PRICE_LIST_TYPES,
  PRICE_LIST_TYPE_LABELS,
  createPriceList,
  priceListOptionsQuery,
  updatePriceList,
  type PriceListBasis,
  type PriceListDetail,
  type PriceListInput,
  type PriceListType,
} from "../api/pricing-api"
import { ChannelFields } from "./pricing-parts"

type PriceListFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = daftar harga baru. */
  priceList?: PriceListDetail
}

/** Tambah/Ubah Daftar Harga (header). Harga per produk diisi di versi. */
export function PriceListFormDialog(props: PriceListFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <PriceListForm {...props} /> : null}
    </Dialog>
  )
}

const NONE = "__none"

function PriceListForm({ onOpenChange, priceList }: PriceListFormDialogProps) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const options = useQuery(priceListOptionsQuery())
  const [code, setCode] = useState(priceList?.code ?? "")
  const [type, setType] = useState<PriceListType>(
    priceList?.priceListType ?? "channel"
  )
  const [name, setName] = useState(priceList?.name ?? "")
  const [channelId, setChannelId] = useState(priceList?.channelId ?? null)
  const [subChannelId, setSubChannelId] = useState(
    priceList?.subChannelId ?? null
  )
  const [partner, setPartner] = useState<PickerOption | null>(
    priceList?.partnerId
      ? { id: priceList.partnerId, label: priceList.partnerName ?? "Mitra" }
      : null
  )
  const [basis, setBasis] = useState<PriceListBasis>(
    priceList?.basis ?? "fixed"
  )
  const [referenceId, setReferenceId] = useState(
    priceList?.referencePriceListId ?? null
  )
  const [description, setDescription] = useState(priceList?.description ?? "")
  const [isActive, setIsActive] = useState(priceList?.isActive ?? true)
  const [errors, setErrors] = useState<FieldErrors>({})
  const isBase = type === "base"
  const references = (options.data ?? []).filter(
    (o) => o.basis === "fixed" && o.id !== priceList?.id
  )

  const save = useIdempotentMutation({
    mutationFn: (body: PriceListInput, key) =>
      priceList
        ? updatePriceList(
            priceList.id,
            { ...body, version: priceList.version },
            key
          )
        : createPriceList(body, key),
    onSuccess: async (saved) => {
      toast.success(
        priceList ? "Daftar harga diperbarui" : "Daftar harga dibuat"
      )
      await queryClient.invalidateQueries({ queryKey: ["pricing"] })
      onOpenChange(false)
      if (!priceList) {
        void navigate({
          to: "/master/price-lists/$id",
          params: { id: saved.id },
          search: {},
        })
      }
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const submit = () => {
    const next: FieldErrors = {}
    if (!priceList && !code.trim()) next.code = "Kode daftar harga wajib diisi"
    if (!name.trim()) next.name = "Nama daftar harga wajib diisi"
    if ((type === "channel" || type === "vending") && !channelId)
      next.channelId = "Pilih channel"
    if (type === "partner" && !partner) next.partnerId = "Pilih mitra"
    if (basis === "percent" && !referenceId)
      next.referencePriceListId = "Pilih daftar harga acuan"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    const scoped = type === "channel" || type === "vending"
    save.mutate({
      code,
      priceListType: type,
      name,
      channelId: scoped ? channelId : null,
      subChannelId: scoped ? subChannelId : null,
      partnerId: type === "partner" ? (partner?.id ?? null) : null,
      basis: isBase ? "fixed" : basis,
      referencePriceListId: basis === "percent" ? referenceId : null,
      description,
      isActive,
    })
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          {priceList ? `Ubah ${priceList.name}` : "Tambah Daftar Harga"}
        </DialogTitle>
        <DialogDescription>
          Satu daftar harga per cakupan. Harga per produk diisi di versi lalu
          diajukan untuk persetujuan.
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
              <FieldLabel htmlFor="price-list-code">Kode</FieldLabel>
              <Input
                id="price-list-code"
                value={code}
                disabled={!!priceList}
                placeholder="mis. MT-BELI-PUTUS"
                aria-invalid={!!errors.code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
              />
              <FieldError errors={fieldError(errors, "code")} />
            </Field>
            {priceList ? (
              <Field>
                <FieldLabel>Jenis</FieldLabel>
                <Input
                  value={PRICE_LIST_TYPE_LABELS[priceList.priceListType]}
                  disabled
                  readOnly
                />
              </Field>
            ) : (
              <ChoiceField
                id="price-list-type"
                label="Jenis"
                value={type}
                onValueChange={setType}
                labels={CREATABLE_PRICE_LIST_TYPES}
                error={errors.priceListType}
              >
                <FieldDescription>
                  Level reseller tersedia di Fase 2.
                </FieldDescription>
              </ChoiceField>
            )}
            <Field data-invalid={!!errors.name} className="sm:col-span-2">
              <FieldLabel htmlFor="price-list-name">Nama</FieldLabel>
              <Input
                id="price-list-name"
                value={name}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "name")} />
            </Field>
            {type === "channel" || type === "vending" ? (
              <ChannelFields
                idPrefix="price-list"
                channelId={channelId}
                subChannelId={subChannelId}
                onChange={(c, s) => {
                  setChannelId(c)
                  setSubChannelId(s)
                }}
                errors={{
                  channelId: errors.channelId,
                  subChannelId: errors.subChannelId,
                }}
              />
            ) : null}
            {type === "partner" ? (
              <Field
                data-invalid={!!errors.partnerId}
                className="sm:col-span-2"
              >
                <FieldLabel htmlFor="price-list-partner">Mitra</FieldLabel>
                <SearchPicker
                  id="price-list-partner"
                  value={partner}
                  onValueChange={setPartner}
                  query={partnerOptionsQuery}
                  toOption={(p) => ({
                    id: p.id,
                    label: p.name,
                    description: p.code,
                  })}
                  placeholder="Pilih mitra"
                  searchPlaceholder="Cari kode atau nama mitra"
                  aria-invalid={!!errors.partnerId}
                />
                <FieldDescription>
                  Daftar harga konsinyasi/MT juga bisa ditautkan di PKS mitra.
                </FieldDescription>
                <FieldError errors={fieldError(errors, "partnerId")} />
              </Field>
            ) : null}
            {!isBase ? (
              <ChoiceField
                id="price-list-basis"
                label="Basis harga"
                value={basis}
                onValueChange={setBasis}
                labels={BASIS_LABELS}
                error={errors.basis}
              />
            ) : null}
            {basis === "percent" && !isBase ? (
              <Field data-invalid={!!errors.referencePriceListId}>
                <FieldLabel htmlFor="price-list-reference">
                  Daftar harga acuan
                </FieldLabel>
                <Select
                  value={referenceId ?? NONE}
                  onValueChange={(v) =>
                    setReferenceId(!v || v === NONE ? null : (v as string))
                  }
                >
                  <SelectTrigger
                    id="price-list-reference"
                    className="w-full"
                    aria-invalid={!!errors.referencePriceListId}
                  >
                    <SelectValue>
                      {(v: string) =>
                        references.find((r) => r.id === v)?.name ??
                        priceList?.referencePriceListName ??
                        "Pilih daftar harga"
                      }
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {references.map((r) => (
                      <SelectItem key={r.id} value={r.id}>
                        {r.code} · {r.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldDescription>
                  Mis. harga karyawan = 90% dari daftar harga website.
                </FieldDescription>
                <FieldError
                  errors={fieldError(errors, "referencePriceListId")}
                />
              </Field>
            ) : null}
          </div>
          <Field data-invalid={!!errors.description}>
            <FieldLabel htmlFor="price-list-description">Keterangan</FieldLabel>
            <Textarea
              id="price-list-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "description")} />
          </Field>
          {!isBase ? (
            <Checkbox label="Aktif" checked={isActive} onChange={setIsActive} />
          ) : null}
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
