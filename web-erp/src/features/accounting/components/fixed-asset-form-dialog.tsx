import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { accountsQuery, divisionsQuery } from "../api/accounting-api"
import {
  ASSET_SOURCE_LABELS,
  assetClassesQuery,
  createFixedAsset,
  updateFixedAsset,
  type FixedAsset,
  type FixedAssetSource,
} from "../api/close-api"
import { AccountSelect } from "./account-select"
import { DimensionSelect } from "./dimension-select"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = aset baru. */
  value?: FixedAsset
}

/** Tambah/ubah aset tetap Draft (S-408). */
export function FixedAssetFormDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <FixedAssetForm {...props} /> : null}
    </Dialog>
  )
}

function FixedAssetForm({ onOpenChange, value }: Props) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const classes = useQuery(assetClassesQuery(false))
  const divisions = useQuery(divisionsQuery(false))
  const accounts = useQuery(accountsQuery(false))
  const paymentAccounts = (accounts.data ?? []).filter(
    (a) => a.isPostable && (a.code.startsWith("11") || a.group === 2)
  )
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [classId, setClassId] = useState(value?.classId ?? "")
  const [source, setSource] = useState<FixedAssetSource>(
    value?.source ?? "purchase"
  )
  const [acquisitionDate, setAcquisitionDate] = useState(
    value?.acquisitionDate ?? todayBusinessDate()
  )
  const [cost, setCost] = useState(value?.cost ?? "")
  const [salvage, setSalvage] = useState(value?.salvage ?? "0")
  const [life, setLife] = useState(value ? String(value.usefulLifeMonths) : "")
  const [openingAccumulated, setOpeningAccumulated] = useState(
    value?.openingAccumulated ?? "0"
  )
  const [paymentAccountId, setPaymentAccountId] = useState(
    value?.paymentAccountId ?? ""
  )
  const [divisionId, setDivisionId] = useState<string | null>(
    value?.divisionId ?? null
  )
  const [locationLabel, setLocationLabel] = useState(value?.locationLabel ?? "")
  const [note, setNote] = useState(value?.note ?? "")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const selectedClass = (classes.data ?? []).find((c) => c.id === classId)

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body = {
        name,
        classId,
        divisionId: divisionId ?? "",
        source,
        acquisitionDate,
        cost: cost || "0",
        salvage: salvage || "0",
        usefulLifeMonths: life ? Number(life) : undefined,
        openingAccumulated:
          source === "opening" ? openingAccumulated || "0" : "0",
        paymentAccountId:
          source === "purchase" && paymentAccountId ? paymentAccountId : null,
        locationLabel,
        note,
      }
      return value
        ? updateFixedAsset(value.id, { ...body, version: value.version }, key)
        : createFixedAsset({ ...body, code }, key)
    },
    onSuccess: async (d) => {
      toast.success(value ? "Aset tetap diperbarui" : "Aset tetap ditambahkan")
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "fixed-assets"],
      })
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "fixed-asset", d.asset.id],
      })
      onOpenChange(false)
      if (!value) {
        void navigate({
          to: "/finance/fixed-assets/$id",
          params: { id: d.asset.id },
          search: { tab: "detail" },
        })
      }
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
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          {value ? `Ubah Aset ${value.code}` : "Tambah Aset Tetap"}
        </DialogTitle>
        <DialogDescription>
          Aset disimpan sebagai Draft. Saat diaktifkan, aset pembelian dijurnal
          A-31 (akun aset kelas ← akun pembayaran) dan mulai disusutkan pada
          bulan perolehan.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <FieldGroup className="grid gap-4 sm:grid-cols-2">
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="asset-code">Kode Aset</FieldLabel>
            <Input
              id="asset-code"
              value={code}
              disabled={!!value}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="asset-name">Nama Aset</FieldLabel>
            <Input
              id="asset-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          <Field data-invalid={!!errors.classId}>
            <FieldLabel htmlFor="asset-class">Kelas Aset</FieldLabel>
            <Select
              value={classId || null}
              onValueChange={(v) => setClassId(v ? String(v) : "")}
            >
              <SelectTrigger id="asset-class" className="w-full">
                <SelectValue>
                  {(v: string | null) =>
                    (classes.data ?? []).find((c) => c.id === v)?.name ??
                    "Pilih kelas"
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(classes.data ?? []).map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name} ({c.usefulLifeMonths} bulan)
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError errors={err("classId")} />
          </Field>
          <Field data-invalid={!!errors.source}>
            <FieldLabel htmlFor="asset-source">Asal Aset</FieldLabel>
            <Select
              value={source}
              onValueChange={(v) => setSource(v as FixedAssetSource)}
            >
              <SelectTrigger id="asset-source" className="w-full">
                <SelectValue>
                  {(v: FixedAssetSource) => ASSET_SOURCE_LABELS[v]}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(["purchase", "opening"] as const).map((s) => (
                  <SelectItem key={s} value={s}>
                    {ASSET_SOURCE_LABELS[s]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError errors={err("source")} />
          </Field>
          <Field data-invalid={!!errors.acquisitionDate}>
            <FieldLabel htmlFor="asset-date">Tanggal Perolehan</FieldLabel>
            <DateInput
              id="asset-date"
              value={acquisitionDate}
              onValueChange={setAcquisitionDate}
            />
            <FieldError errors={err("acquisitionDate")} />
          </Field>
          <Field data-invalid={!!errors.cost}>
            <FieldLabel htmlFor="asset-cost">Harga Perolehan</FieldLabel>
            <MoneyInput id="asset-cost" value={cost} onValueChange={setCost} />
            <FieldError errors={err("cost")} />
          </Field>
          <Field data-invalid={!!errors.salvage}>
            <FieldLabel htmlFor="asset-salvage">Nilai Sisa</FieldLabel>
            <MoneyInput
              id="asset-salvage"
              value={salvage}
              onValueChange={setSalvage}
            />
            <FieldError errors={err("salvage")} />
          </Field>
          <Field data-invalid={!!errors.usefulLife}>
            <FieldLabel htmlFor="asset-life">Umur Manfaat (bulan)</FieldLabel>
            <Input
              id="asset-life"
              inputMode="numeric"
              placeholder={
                selectedClass
                  ? `${selectedClass.usefulLifeMonths} (umur kelas)`
                  : "Umur kelas"
              }
              value={life}
              onChange={(e) => setLife(e.target.value.replace(/\D/g, ""))}
            />
            <FieldError errors={err("usefulLife")} />
          </Field>
          {source === "opening" ? (
            <Field data-invalid={!!errors.openingAccumulated}>
              <FieldLabel htmlFor="asset-opening">
                Akumulasi Penyusutan per Cut-over
              </FieldLabel>
              <MoneyInput
                id="asset-opening"
                value={openingAccumulated}
                onValueChange={setOpeningAccumulated}
              />
              <FieldDescription>
                Penyusutan sistem mulai bulan setelah tanggal cut-over.
              </FieldDescription>
              <FieldError errors={err("openingAccumulated")} />
            </Field>
          ) : (
            <Field data-invalid={!!errors.paymentAccountId}>
              <FieldLabel htmlFor="asset-payment">Akun Pembayaran</FieldLabel>
              <AccountSelect
                id="asset-payment"
                value={paymentAccountId}
                onValueChange={setPaymentAccountId}
                accounts={paymentAccounts}
                placeholder="Kas & Bank atau Utang"
              />
              <FieldError errors={err("paymentAccountId")} />
            </Field>
          )}
          <Field data-invalid={!!errors.divisionId}>
            <FieldLabel htmlFor="asset-division">Divisi</FieldLabel>
            <DimensionSelect
              id="asset-division"
              value={divisionId}
              options={divisions.data ?? []}
              placeholder="Pilih divisi"
              onValueChange={setDivisionId}
            />
            <FieldError errors={err("divisionId")} />
          </Field>
          <Field data-invalid={!!errors.locationLabel}>
            <FieldLabel htmlFor="asset-location">Lokasi / Mitra</FieldLabel>
            <Input
              id="asset-location"
              value={locationLabel}
              placeholder="Mis. kode mitra lokasi mesin vending"
              onChange={(e) => setLocationLabel(e.target.value)}
            />
            <FieldError errors={err("locationLabel")} />
          </Field>
          <Field className="sm:col-span-2" data-invalid={!!errors.note}>
            <FieldLabel htmlFor="asset-note">Catatan</FieldLabel>
            <Textarea
              id="asset-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
            <FieldError errors={err("note")} />
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
