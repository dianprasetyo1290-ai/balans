import { useQueryClient } from "@tanstack/react-query"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { LoadingButton } from "@/components/common/loading-button"
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
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"

import {
  createSupplier,
  SUPPLIER_TYPE_LABELS,
  updateSupplier,
  WITHHOLDING_LABELS,
  type Supplier,
  type SupplierInput,
  type SupplierType,
  type WithholdingType,
} from "../api/party-api"
import {
  ChoiceField,
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "./form-parts"

type SupplierFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = pemasok baru. */
  supplier?: Supplier
}

type BankDraft = {
  key: string
  id?: string
  bankName: string
  accountNumber: string
  /** Nomor tersamar rekening lama; nomor baru kosong = tetap. */
  masked?: string
  accountName: string
  isDefault: boolean
}

const ENTITY_LABELS = { yes: "Badan usaha (PT/CV)", no: "Perorangan" } as const

/** Tambah/ubah pemasok atau maklon (MD-08). NPWP dan nomor rekening disimpan terenkripsi. */
export function SupplierFormDialog(props: SupplierFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <SupplierForm {...props} /> : null}
    </Dialog>
  )
}

function SupplierForm({ onOpenChange, supplier }: SupplierFormDialogProps) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(supplier?.code ?? "")
  const [name, setName] = useState(supplier?.name ?? "")
  const [type, setType] = useState<SupplierType>(
    supplier?.supplierType ?? "supplier"
  )
  const [entity, setEntity] = useState<keyof typeof ENTITY_LABELS>(
    supplier && !supplier.isBusinessEntity ? "no" : "yes"
  )
  const [npwp, setNpwp] = useState("")
  const [withholding, setWithholding] = useState<WithholdingType>(
    supplier?.defaultWithholding ?? "none"
  )
  const [term, setTerm] = useState(String(supplier?.paymentTermDays ?? 30))
  const [address, setAddress] = useState(supplier?.address ?? "")
  const [phone, setPhone] = useState(supplier?.phone ?? "")
  const [email, setEmail] = useState(supplier?.email ?? "")
  const [picName, setPicName] = useState(supplier?.picName ?? "")
  const [notes, setNotes] = useState(supplier?.notes ?? "")
  const [banks, setBanks] = useState<BankDraft[]>(
    () =>
      supplier?.bankAccounts.map((b) => ({
        key: b.id,
        id: b.id,
        bankName: b.bankName,
        accountNumber: "",
        masked: b.accountNumberMasked,
        accountName: b.accountName,
        isDefault: b.isDefault,
      })) ?? []
  )
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (body: SupplierInput, key) =>
      supplier
        ? updateSupplier(
            supplier.id,
            { ...body, version: supplier.version },
            key
          )
        : createSupplier(body, key),
    onSuccess: async () => {
      toast.success(supplier ? "Pemasok diperbarui" : "Pemasok ditambahkan")
      await queryClient.invalidateQueries({ queryKey: ["party", "suppliers"] })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const setBank = (key: string, patch: Partial<BankDraft>) =>
    setBanks((list) =>
      list.map((b) =>
        b.key === key
          ? { ...b, ...patch }
          : patch.isDefault
            ? { ...b, isDefault: false }
            : b
      )
    )

  const submit = () => {
    const next: FieldErrors = {}
    if (!code.trim()) next.code = "Kode pemasok wajib diisi"
    if (!name.trim()) next.name = "Nama pemasok wajib diisi"
    const days = Number(term)
    if (!Number.isInteger(days) || days < 0 || days > 365)
      next.paymentTermDays = "Termin pembayaran 0 sampai 365 hari"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate({
      code,
      name,
      supplierType: type,
      isBusinessEntity: entity === "yes",
      npwp: supplier && !npwp ? undefined : npwp,
      defaultWithholding: withholding,
      paymentTermDays: days,
      address,
      phone,
      email,
      picName,
      notes,
      bankAccounts: banks.map((b) => ({
        id: b.id,
        bankName: b.bankName,
        accountNumber: b.accountNumber || undefined,
        accountName: b.accountName,
        isDefault: b.isDefault,
      })),
    })
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          {supplier ? `Ubah Pemasok ${supplier.code}` : "Tambah Pemasok"}
        </DialogTitle>
        <DialogDescription>
          Kode pemasok permanen. NPWP dan nomor rekening disimpan terenkripsi
          dan ditampilkan tersamar.
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
              <FieldLabel htmlFor="supplier-code">Kode pemasok</FieldLabel>
              <Input
                id="supplier-code"
                value={code}
                disabled={!!supplier}
                className="uppercase"
                aria-invalid={!!errors.code}
                onChange={(e) => setCode(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "code")} />
            </Field>
            <Field data-invalid={!!errors.name}>
              <FieldLabel htmlFor="supplier-name">Nama</FieldLabel>
              <Input
                id="supplier-name"
                value={name}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "name")} />
            </Field>
            <ChoiceField
              id="supplier-type"
              label="Jenis"
              value={type}
              onValueChange={setType}
              labels={SUPPLIER_TYPE_LABELS}
              error={errors.supplierType}
            />
            <ChoiceField
              id="supplier-entity"
              label="Bentuk usaha"
              value={entity}
              onValueChange={setEntity}
              labels={ENTITY_LABELS}
              error={errors.isBusinessEntity}
            />
            <Field data-invalid={!!errors.npwp}>
              <FieldLabel htmlFor="supplier-npwp">NPWP</FieldLabel>
              <Input
                id="supplier-npwp"
                inputMode="numeric"
                autoComplete="off"
                placeholder={
                  supplier?.npwpMasked
                    ? `${supplier.npwpMasked} (kosongkan agar tetap)`
                    : "15 atau 16 digit"
                }
                value={npwp}
                aria-invalid={!!errors.npwp}
                onChange={(e) => setNpwp(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "npwp")} />
            </Field>
            <ChoiceField
              id="supplier-withholding"
              label="PPh default"
              value={withholding}
              onValueChange={setWithholding}
              labels={WITHHOLDING_LABELS}
              error={errors.defaultWithholding}
            />
            <Field data-invalid={!!errors.paymentTermDays}>
              <FieldLabel htmlFor="supplier-term">
                Termin pembayaran (hari)
              </FieldLabel>
              <Input
                id="supplier-term"
                type="number"
                inputMode="numeric"
                min={0}
                max={365}
                value={term}
                aria-invalid={!!errors.paymentTermDays}
                onChange={(e) => setTerm(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "paymentTermDays")} />
            </Field>
            <Field data-invalid={!!errors.picName}>
              <FieldLabel htmlFor="supplier-pic">Nama PIC</FieldLabel>
              <Input
                id="supplier-pic"
                value={picName}
                onChange={(e) => setPicName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "picName")} />
            </Field>
            <Field data-invalid={!!errors.phone}>
              <FieldLabel htmlFor="supplier-phone">Telepon</FieldLabel>
              <Input
                id="supplier-phone"
                type="tel"
                inputMode="tel"
                value={phone}
                aria-invalid={!!errors.phone}
                onChange={(e) => setPhone(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "phone")} />
            </Field>
            <Field data-invalid={!!errors.email}>
              <FieldLabel htmlFor="supplier-email">Email</FieldLabel>
              <Input
                id="supplier-email"
                type="email"
                value={email}
                aria-invalid={!!errors.email}
                onChange={(e) => setEmail(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "email")} />
            </Field>
          </div>
          <Field data-invalid={!!errors.address}>
            <FieldLabel htmlFor="supplier-address">Alamat</FieldLabel>
            <Textarea
              id="supplier-address"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "address")} />
          </Field>
          <FieldSet data-invalid={!!errors.bankAccounts}>
            <FieldLegend variant="label">Rekening bank</FieldLegend>
            <FieldError errors={fieldError(errors, "bankAccounts")} />
            {banks.map((b, i) => {
              const p = `bankAccounts.${i}.`
              return (
                <div key={b.key} className="rounded-lg border p-3">
                  <div className="mb-3 flex items-center justify-between gap-2">
                    <label className="flex min-h-11 items-center gap-2 text-sm sm:min-h-0">
                      <input
                        type="radio"
                        name="default-bank"
                        className="size-4 accent-(--color-primary)"
                        checked={b.isDefault}
                        onChange={() => setBank(b.key, { isDefault: true })}
                      />
                      Rekening utama
                    </label>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`Hapus rekening ${i + 1}`}
                      onClick={() =>
                        setBanks((list) => list.filter((x) => x.key !== b.key))
                      }
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-3">
                    <Field data-invalid={!!errors[p + "bankName"]}>
                      <FieldLabel htmlFor={`bank-${b.key}-name`}>
                        Bank
                      </FieldLabel>
                      <Input
                        id={`bank-${b.key}-name`}
                        value={b.bankName}
                        aria-invalid={!!errors[p + "bankName"]}
                        onChange={(e) =>
                          setBank(b.key, { bankName: e.target.value })
                        }
                      />
                      <FieldError errors={fieldError(errors, p + "bankName")} />
                    </Field>
                    <Field data-invalid={!!errors[p + "accountNumber"]}>
                      <FieldLabel htmlFor={`bank-${b.key}-number`}>
                        Nomor rekening
                      </FieldLabel>
                      <Input
                        id={`bank-${b.key}-number`}
                        inputMode="numeric"
                        autoComplete="off"
                        placeholder={b.masked ?? ""}
                        value={b.accountNumber}
                        aria-invalid={!!errors[p + "accountNumber"]}
                        onChange={(e) =>
                          setBank(b.key, { accountNumber: e.target.value })
                        }
                      />
                      <FieldError
                        errors={fieldError(errors, p + "accountNumber")}
                      />
                    </Field>
                    <Field data-invalid={!!errors[p + "accountName"]}>
                      <FieldLabel htmlFor={`bank-${b.key}-holder`}>
                        Atas nama
                      </FieldLabel>
                      <Input
                        id={`bank-${b.key}-holder`}
                        value={b.accountName}
                        aria-invalid={!!errors[p + "accountName"]}
                        onChange={(e) =>
                          setBank(b.key, { accountName: e.target.value })
                        }
                      />
                      <FieldError
                        errors={fieldError(errors, p + "accountName")}
                      />
                    </Field>
                  </div>
                </div>
              )
            })}
            <Button
              type="button"
              variant="outline"
              className="min-h-11 self-start sm:min-h-9"
              onClick={() =>
                setBanks((list) => [
                  ...list,
                  {
                    key: crypto.randomUUID(),
                    bankName: "",
                    accountNumber: "",
                    accountName: name,
                    isDefault: list.length === 0,
                  },
                ])
              }
            >
              <PlusIcon /> Tambah Rekening
            </Button>
          </FieldSet>
          <Field data-invalid={!!errors.notes}>
            <FieldLabel htmlFor="supplier-notes">Catatan</FieldLabel>
            <Textarea
              id="supplier-notes"
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
