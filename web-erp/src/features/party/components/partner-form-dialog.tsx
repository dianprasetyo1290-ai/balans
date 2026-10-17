import { useQueryClient } from "@tanstack/react-query"
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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"

import {
  createPartner,
  PARTNER_TYPE_LABELS,
  partyUserOptionsQuery,
  updatePartner,
  type Partner,
  type PartnerInput,
  type PartnerType,
} from "../api/party-api"
import {
  ChoiceField,
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "./form-parts"

type PartnerFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = mitra baru. */
  partner?: Partner
}

const userOption = (
  id: string | null | undefined,
  name: string | null | undefined
): PickerOption | null => (id ? { id, label: name ?? "Pengguna" } : null)

/** Tambah/ubah mitra MT, konsinyasi, atau B2B (MD-07). */
export function PartnerFormDialog(props: PartnerFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <PartnerForm {...props} /> : null}
    </Dialog>
  )
}

function UserField({
  id,
  label,
  userRole,
  value,
  onValueChange,
  error,
  description,
}: {
  id: string
  label: string
  userRole: string
  value: PickerOption | null
  onValueChange: (v: PickerOption | null) => void
  error?: string
  description?: string
}) {
  return (
    <Field data-invalid={!!error}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <SearchPicker
        id={id}
        value={value}
        onValueChange={onValueChange}
        query={(q) => partyUserOptionsQuery(q, userRole)}
        toOption={(u) => ({
          id: u.id,
          label: u.fullName,
          description: u.email,
        })}
        placeholder="Belum ditugaskan"
        searchPlaceholder="Cari nama atau email"
        emptyMessage="Tidak ada pengguna dengan role ini."
        clearable
        aria-invalid={!!error}
      />
      {description ? <FieldDescription>{description}</FieldDescription> : null}
      <FieldError errors={error ? [{ message: error }] : []} />
    </Field>
  )
}

function PartnerForm({ onOpenChange, partner }: PartnerFormDialogProps) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(partner?.code ?? "")
  const [name, setName] = useState(partner?.name ?? "")
  const [type, setType] = useState<PartnerType>(
    partner?.partnerType ?? "consignment"
  )
  const [npwp, setNpwp] = useState("")
  const [address, setAddress] = useState(partner?.address ?? "")
  const [picName, setPicName] = useState(partner?.picName ?? "")
  const [picPhone, setPicPhone] = useState(partner?.picPhone ?? "")
  const [sales, setSales] = useState(() =>
    userOption(partner?.salesUserId, partner?.salesUserName)
  )
  const [merch, setMerch] = useState(() =>
    userOption(partner?.merchandiserUserId, partner?.merchandiserUserName)
  )
  const [notes, setNotes] = useState(partner?.notes ?? "")
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (body: PartnerInput, key) =>
      partner
        ? updatePartner(partner.id, { ...body, version: partner.version }, key)
        : createPartner(body, key),
    onSuccess: async () => {
      toast.success(partner ? "Mitra diperbarui" : "Mitra ditambahkan")
      await queryClient.invalidateQueries({ queryKey: ["party", "partners"] })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const submit = () => {
    const next: FieldErrors = {}
    if (!code.trim()) next.code = "Kode mitra wajib diisi"
    if (!name.trim()) next.name = "Nama mitra wajib diisi"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate({
      code,
      name,
      partnerType: type,
      // Kosong saat ubah = NPWP lama dipertahankan.
      npwp: partner && !npwp ? undefined : npwp,
      address,
      picName,
      picPhone,
      salesUserId: sales?.id ?? null,
      merchandiserUserId: merch?.id ?? null,
      notes,
    })
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          {partner ? `Ubah Mitra ${partner.code}` : "Tambah Mitra"}
        </DialogTitle>
        <DialogDescription>
          Kode mitra permanen dan tidak bisa dipakai ulang. NPWP disimpan
          terenkripsi.
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
              <FieldLabel htmlFor="partner-code">Kode mitra</FieldLabel>
              <Input
                id="partner-code"
                value={code}
                disabled={!!partner}
                className="uppercase"
                aria-invalid={!!errors.code}
                onChange={(e) => setCode(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "code")} />
            </Field>
            <Field data-invalid={!!errors.name}>
              <FieldLabel htmlFor="partner-name">Nama mitra</FieldLabel>
              <Input
                id="partner-name"
                value={name}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "name")} />
            </Field>
            <ChoiceField
              id="partner-type"
              label="Jenis mitra"
              value={type}
              onValueChange={setType}
              labels={PARTNER_TYPE_LABELS}
              error={errors.partnerType}
            />
            <Field data-invalid={!!errors.npwp}>
              <FieldLabel htmlFor="partner-npwp">NPWP</FieldLabel>
              <Input
                id="partner-npwp"
                inputMode="numeric"
                autoComplete="off"
                placeholder={
                  partner?.npwpMasked
                    ? `${partner.npwpMasked} (kosongkan agar tetap)`
                    : "15 atau 16 digit"
                }
                value={npwp}
                aria-invalid={!!errors.npwp}
                onChange={(e) => setNpwp(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "npwp")} />
            </Field>
            <Field data-invalid={!!errors.picName}>
              <FieldLabel htmlFor="partner-pic">Nama PIC mitra</FieldLabel>
              <Input
                id="partner-pic"
                value={picName}
                onChange={(e) => setPicName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "picName")} />
            </Field>
            <Field data-invalid={!!errors.picPhone}>
              <FieldLabel htmlFor="partner-pic-phone">HP PIC mitra</FieldLabel>
              <Input
                id="partner-pic-phone"
                type="tel"
                inputMode="tel"
                value={picPhone}
                aria-invalid={!!errors.picPhone}
                onChange={(e) => setPicPhone(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "picPhone")} />
            </Field>
            <UserField
              id="partner-sales"
              label="Sales MT"
              userRole="sales_mt"
              value={sales}
              onValueChange={setSales}
              error={errors.salesUserId}
            />
            <UserField
              id="partner-merchandiser"
              label="Merchandiser"
              userRole="merchandiser"
              value={merch}
              onValueChange={setMerch}
              error={errors.merchandiserUserId}
              description="Merchandiser hanya bisa stock count di mitra yang ditugaskan."
            />
          </div>
          <Field data-invalid={!!errors.address}>
            <FieldLabel htmlFor="partner-address">Alamat</FieldLabel>
            <Textarea
              id="partner-address"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "address")} />
          </Field>
          <Field data-invalid={!!errors.notes}>
            <FieldLabel htmlFor="partner-notes">Catatan</FieldLabel>
            <Textarea
              id="partner-notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "notes")} />
          </Field>
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
