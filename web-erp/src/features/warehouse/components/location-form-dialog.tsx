import { useQueryClient } from "@tanstack/react-query"
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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createLocation,
  EXTERNAL_LOCATION_TYPES,
  INTERNAL_LOCATION_TYPES,
  PARTY_TYPE_LABELS,
  updateLocation,
  type ExternalPartyType,
  type Location,
  type LocationType,
  type Warehouse,
} from "../api/warehouse-api"

type LocationFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Ubah lokasi yang ada (nama & status aktif). */
  location?: Location
  /** Lokasi tambahan di gudang ini; kosong = lokasi eksternal per pihak. */
  warehouse?: Warehouse
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

/** Tambah lokasi di gudang, lokasi eksternal per pihak, atau ubah nama/status lokasi (S-123). */
export function LocationFormDialog(props: LocationFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <LocationForm {...props} /> : null}
    </Dialog>
  )
}

function LocationForm({
  onOpenChange,
  location,
  warehouse,
}: LocationFormDialogProps) {
  const queryClient = useQueryClient()
  const external = !location && !warehouse
  const types = external ? EXTERNAL_LOCATION_TYPES : INTERNAL_LOCATION_TYPES
  const [code, setCode] = useState(
    location?.code ?? (warehouse ? `${warehouse.code}-` : "")
  )
  const [name, setName] = useState(location?.name ?? "")
  const [type, setType] = useState<LocationType>(
    location?.locationType ?? (external ? "consignment" : "event")
  )
  const allowedParties =
    EXTERNAL_LOCATION_TYPES.find((t) => t.type === type)?.partyTypes ?? []
  const [partyType, setPartyType] = useState<ExternalPartyType>(
    allowedParties[0] ?? "partner"
  )
  const [partyId, setPartyId] = useState("")
  const [isActive, setIsActive] = useState(location?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      location
        ? updateLocation(
            location.id,
            { name, isActive, version: location.version },
            key
          )
        : createLocation(
            {
              code,
              name,
              locationType: type,
              isActive,
              warehouseId: warehouse?.id ?? null,
              externalPartyType: external ? partyType : null,
              externalPartyId: external ? partyId.trim() : null,
            },
            key
          ),
    onSuccess: async () => {
      toast.success(location ? "Lokasi diperbarui" : "Lokasi ditambahkan")
      await queryClient.invalidateQueries({ queryKey: ["warehouse"] })
      onOpenChange(false)
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
    if (!location && !code.trim()) next.code = "Kode wajib diisi"
    if (!name.trim()) next.name = "Nama wajib diisi"
    if (external && !UUID.test(partyId.trim()))
      next.externalPartyId = "ID pihak harus berupa UUID"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  const title = location
    ? `Ubah Lokasi ${location.code}`
    : external
      ? "Tambah Lokasi Eksternal"
      : `Tambah Lokasi di ${warehouse?.name ?? "Gudang"}`

  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>
          {location
            ? "Kode, jenis, gudang, dan pihak lokasi tidak bisa diubah karena dipakai mutasi stok."
            : external
              ? "Lokasi stok di luar gudang Balans, satu lokasi per jenis per pihak."
              : "Lokasi tambahan selain lokasi standar gudang, mis. lokasi event."}
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
          {location ? null : (
            <div className="grid gap-4 sm:grid-cols-2">
              <Field data-invalid={!!errors.code}>
                <FieldLabel htmlFor="location-code">Kode</FieldLabel>
                <Input
                  id="location-code"
                  value={code}
                  aria-invalid={!!errors.code}
                  onChange={(e) => setCode(e.target.value.toUpperCase())}
                />
                <FieldError errors={err("code")} />
              </Field>
              <Field data-invalid={!!errors.locationType}>
                <FieldLabel htmlFor="location-type">Jenis</FieldLabel>
                <Select
                  value={type}
                  onValueChange={(v) => {
                    const t = v as LocationType
                    setType(t)
                    const parties =
                      EXTERNAL_LOCATION_TYPES.find((x) => x.type === t)
                        ?.partyTypes ?? []
                    if (parties.length > 0) setPartyType(parties[0])
                  }}
                >
                  <SelectTrigger id="location-type" className="w-full">
                    <SelectValue>
                      {(v: string) => types.find((t) => t.type === v)?.label}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {types.map((t) => (
                      <SelectItem key={t.type} value={t.type}>
                        {t.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError errors={err("locationType")} />
              </Field>
            </div>
          )}
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="location-name">Nama</FieldLabel>
            <Input
              id="location-name"
              value={name}
              placeholder={external ? "mis. Konsinyasi – Toko Sinar Jaya" : ""}
              aria-invalid={!!errors.name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          {external ? (
            <div className="grid gap-4 sm:grid-cols-[10rem_1fr]">
              <Field data-invalid={!!errors.externalPartyType}>
                <FieldLabel htmlFor="location-party-type">
                  Jenis pihak
                </FieldLabel>
                <Select
                  value={partyType}
                  onValueChange={(v) => setPartyType(v as ExternalPartyType)}
                >
                  <SelectTrigger id="location-party-type" className="w-full">
                    <SelectValue>
                      {(v: ExternalPartyType) => PARTY_TYPE_LABELS[v]}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {allowedParties.map((p) => (
                      <SelectItem key={p} value={p}>
                        {PARTY_TYPE_LABELS[p]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FieldError errors={err("externalPartyType")} />
              </Field>
              <Field data-invalid={!!errors.externalPartyId}>
                <FieldLabel htmlFor="location-party-id">ID pihak</FieldLabel>
                <Input
                  id="location-party-id"
                  value={partyId}
                  className="font-mono text-xs"
                  aria-invalid={!!errors.externalPartyId}
                  onChange={(e) => setPartyId(e.target.value)}
                />
                <FieldDescription>
                  ID pemasok/maklon, mitra, atau mesin dari master terkait.
                </FieldDescription>
                <FieldError errors={err("externalPartyId")} />
              </Field>
            </div>
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
