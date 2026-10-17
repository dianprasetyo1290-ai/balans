import { useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
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
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { employeeOptionsQuery } from "@/features/hris/api/hris-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"

import {
  createCustomer,
  CUSTOMER_TYPE_LABELS,
  updateCustomer,
  type Customer,
  type CustomerAddressInput,
  type CustomerInput,
  type CustomerType,
} from "../api/party-api"
import {
  ChoiceField,
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "./form-parts"

/** Versi teks persetujuan pemasaran yang ditampilkan di form (TG 6.7). */
export const MARKETING_CONSENT_VERSION = "erp-2026-10"

type CustomerFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = pelanggan baru. */
  customer?: Customer
  /** Karyawan bisa ditautkan jika modul hris aktif. */
  employeesEnabled: boolean
}

type AddressDraft = CustomerAddressInput & { key: string }

const emptyAddress = (isDefault: boolean): AddressDraft => ({
  key: crypto.randomUUID(),
  label: "",
  recipientName: "",
  phone: "",
  addressLine: "",
  district: "",
  city: "",
  province: "",
  postalCode: "",
  isDefault,
})

/** Tambah/ubah pelanggan (MD-06). Nomor HP dinormalisasi server ke +62… dan harus unik. */
export function CustomerFormDialog(props: CustomerFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <CustomerForm {...props} /> : null}
    </Dialog>
  )
}

function CustomerForm({
  onOpenChange,
  customer,
  employeesEnabled,
}: CustomerFormDialogProps) {
  const queryClient = useQueryClient()
  const [fullName, setFullName] = useState(customer?.fullName ?? "")
  const [phone, setPhone] = useState(customer?.phone ?? "")
  const [email, setEmail] = useState(customer?.email ?? "")
  const [type, setType] = useState<CustomerType>(
    customer?.customerType ?? "retail"
  )
  const [employee, setEmployee] = useState<PickerOption | null>(
    customer?.employeeId
      ? {
          id: customer.employeeId,
          label: customer.employeeName ?? "Karyawan",
          description: customer.employeeNumber ?? undefined,
        }
      : null
  )
  const [consent, setConsent] = useState(customer?.marketingConsent ?? false)
  const [notes, setNotes] = useState(customer?.notes ?? "")
  const [addresses, setAddresses] = useState<AddressDraft[]>(() =>
    customer && customer.addresses.length > 0
      ? customer.addresses.map((a) => ({ ...a, key: a.id }))
      : [emptyAddress(true)]
  )
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (body: CustomerInput, key) =>
      customer
        ? updateCustomer(
            customer.id,
            { ...body, version: customer.version },
            key
          )
        : createCustomer(body, key),
    onSuccess: async () => {
      toast.success(customer ? "Pelanggan diperbarui" : "Pelanggan ditambahkan")
      await queryClient.invalidateQueries({ queryKey: ["party", "customers"] })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const setAddress = (key: string, patch: Partial<AddressDraft>) =>
    setAddresses((list) =>
      list.map((a) =>
        a.key === key
          ? { ...a, ...patch }
          : patch.isDefault
            ? { ...a, isDefault: false }
            : a
      )
    )

  const submit = () => {
    const next: FieldErrors = {}
    if (!fullName.trim()) next.fullName = "Nama lengkap wajib diisi"
    if (!phone.trim()) next.phone = "Nomor HP wajib diisi"
    if (type === "employee" && !employee)
      next.employeeId = "Pilih karyawan untuk pelanggan bertipe karyawan"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate({
      fullName,
      phone,
      email,
      customerType: type,
      employeeId: type === "employee" ? (employee?.id ?? null) : null,
      marketingConsent: consent,
      marketingConsentVersion: consent ? MARKETING_CONSENT_VERSION : "",
      notes,
      addresses: addresses
        .filter((a) => a.addressLine.trim() || a.recipientName.trim())
        .map((a) => ({
          id: a.id,
          label: a.label,
          recipientName: a.recipientName,
          phone: a.phone,
          addressLine: a.addressLine,
          district: a.district,
          city: a.city,
          province: a.province,
          postalCode: a.postalCode,
          latitude: a.latitude || null,
          longitude: a.longitude || null,
          isDefault: a.isDefault,
        })),
    })
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          {customer ? `Ubah Pelanggan ${customer.code}` : "Tambah Pelanggan"}
        </DialogTitle>
        <DialogDescription>
          Nomor HP 08…, 628…, atau +628… disimpan dalam satu format dan tidak
          boleh dipakai pelanggan lain.
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
            <Field data-invalid={!!errors.fullName}>
              <FieldLabel htmlFor="customer-name">Nama lengkap</FieldLabel>
              <Input
                id="customer-name"
                value={fullName}
                aria-invalid={!!errors.fullName}
                onChange={(e) => setFullName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "fullName")} />
            </Field>
            <Field data-invalid={!!errors.phone}>
              <FieldLabel htmlFor="customer-phone">Nomor WA/HP</FieldLabel>
              <Input
                id="customer-phone"
                type="tel"
                inputMode="tel"
                placeholder="0812-3456-7890"
                value={phone}
                aria-invalid={!!errors.phone}
                onChange={(e) => setPhone(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "phone")} />
            </Field>
            <Field data-invalid={!!errors.email}>
              <FieldLabel htmlFor="customer-email">Email (opsional)</FieldLabel>
              <Input
                id="customer-email"
                type="email"
                value={email}
                aria-invalid={!!errors.email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "email")} />
            </Field>
            <ChoiceField
              id="customer-type"
              label="Tipe pelanggan"
              value={type}
              onValueChange={setType}
              labels={
                employeesEnabled
                  ? CUSTOMER_TYPE_LABELS
                  : {
                      retail: CUSTOMER_TYPE_LABELS.retail,
                      b2b: CUSTOMER_TYPE_LABELS.b2b,
                      reseller: CUSTOMER_TYPE_LABELS.reseller,
                    }
              }
              error={errors.customerType}
            />
          </div>
          {type === "employee" ? (
            <Field data-invalid={!!errors.employeeId}>
              <FieldLabel htmlFor="customer-employee">Karyawan</FieldLabel>
              <SearchPicker
                id="customer-employee"
                value={employee}
                onValueChange={setEmployee}
                query={employeeOptionsQuery}
                toOption={(e) => ({
                  id: e.id,
                  label: e.fullName,
                  description: e.employeeNumber,
                })}
                placeholder="Pilih karyawan aktif"
                searchPlaceholder="Cari nama atau nomor karyawan"
                aria-invalid={!!errors.employeeId}
              />
              <FieldDescription>
                Harga karyawan berlaku otomatis selama karyawan aktif dan hilang
                saat resign.
              </FieldDescription>
              <FieldError errors={fieldError(errors, "employeeId")} />
            </Field>
          ) : null}
          <Field orientation="horizontal">
            <input
              id="customer-consent"
              type="checkbox"
              className="mt-0.5 size-5 accent-(--color-primary)"
              checked={consent}
              onChange={(e) => setConsent(e.target.checked)}
            />
            <FieldLabel htmlFor="customer-consent" className="font-normal">
              Pelanggan setuju menerima informasi promo dan produk via
              WhatsApp/email
              {customer?.marketingConsentAt && consent
                ? ` (disetujui ${formatDateTime(customer.marketingConsentAt)})`
                : ""}
            </FieldLabel>
          </Field>
          <FieldSet data-invalid={!!errors.addresses}>
            <FieldLegend variant="label">Alamat pengiriman</FieldLegend>
            <FieldError errors={fieldError(errors, "addresses")} />
            <div className="flex flex-col gap-3">
              {addresses.map((a, i) => (
                <AddressEditor
                  key={a.key}
                  index={i}
                  address={a}
                  errors={errors}
                  onChange={(patch) => setAddress(a.key, patch)}
                  onRemove={() =>
                    setAddresses((list) => list.filter((x) => x.key !== a.key))
                  }
                />
              ))}
            </div>
            <Button
              type="button"
              variant="outline"
              className="min-h-11 self-start sm:min-h-9"
              onClick={() =>
                setAddresses((list) => [
                  ...list,
                  emptyAddress(list.length === 0),
                ])
              }
            >
              <PlusIcon /> Tambah Alamat
            </Button>
          </FieldSet>
          <Field data-invalid={!!errors.notes}>
            <FieldLabel htmlFor="customer-notes">Catatan</FieldLabel>
            <Textarea
              id="customer-notes"
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

type AddressEditorProps = {
  index: number
  address: AddressDraft
  errors: FieldErrors
  onChange: (patch: Partial<AddressDraft>) => void
  onRemove: () => void
}

function AddressEditor({
  index,
  address,
  errors,
  onChange,
  onRemove,
}: AddressEditorProps) {
  const p = `addresses.${index}.`
  const field = (
    name: keyof CustomerAddressInput,
    label: string,
    opts: { className?: string; inputMode?: "numeric" | "tel" } = {}
  ) => {
    const id = `address-${address.key}-${name}`
    const err = errors[p + name]
    return (
      <Field data-invalid={!!err} className={opts.className}>
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        <Input
          id={id}
          inputMode={opts.inputMode}
          value={(address[name] as string | null | undefined) ?? ""}
          aria-invalid={!!err}
          onChange={(e) => onChange({ [name]: e.target.value })}
        />
        <FieldError errors={fieldError(errors, p + name)} />
      </Field>
    )
  }
  return (
    <div className="rounded-lg border p-3">
      <div className="mb-3 flex items-center justify-between gap-2">
        <label className="flex min-h-11 items-center gap-2 text-sm sm:min-h-0">
          <input
            type="radio"
            name="default-address"
            className="size-4 accent-(--color-primary)"
            checked={address.isDefault}
            onChange={() => onChange({ isDefault: true })}
          />
          Alamat utama
        </label>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`Hapus alamat ${index + 1}`}
          onClick={onRemove}
        >
          <Trash2Icon />
        </Button>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {field("label", "Label (mis. Rumah)")}
        {field("recipientName", "Nama penerima")}
        {field("phone", "HP penerima", { inputMode: "tel" })}
        {field("addressLine", "Alamat", { className: "sm:col-span-2" })}
        {field("district", "Kecamatan")}
        {field("city", "Kota/Kabupaten")}
        {field("province", "Provinsi")}
        {field("postalCode", "Kode pos", { inputMode: "numeric" })}
      </div>
    </div>
  )
}
