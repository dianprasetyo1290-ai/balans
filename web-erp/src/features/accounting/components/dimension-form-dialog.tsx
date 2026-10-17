import { useQueryClient } from "@tanstack/react-query"
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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createCampaign,
  createSimpleDimension,
  createSubChannel,
  DIMENSION_LABELS,
  updateCampaign,
  updateSimpleDimension,
  updateSubChannel,
  type Dimension,
  type DimensionKind,
} from "../api/accounting-api"
import { Checkbox } from "./account-form-dialog"
import { DimensionSelect } from "./dimension-select"

type DimensionFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  kind: DimensionKind
  /** Kosong = data baru. */
  value?: Dimension
  /** Pilihan channel untuk sub-channel baru. */
  channels?: Dimension[]
}

const CODE_HINTS: Record<DimensionKind, string> = {
  channel: "Huruf kecil dan garis bawah, mis. modern_trade",
  sub_channel: "Huruf kecil, angka, garis bawah, tanda hubung, mis. shopee",
  division: "Huruf kapital 2–10 karakter, mis. MKT",
  campaign: "Format KMP-YYYYMM-SINGKATAN, mis. KMP-202701-RAMADAN",
}

/** Tambah/ubah channel, sub-channel, divisi, atau kampanye (S-129). Kode tidak bisa diubah. */
export function DimensionFormDialog(props: DimensionFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <DimensionForm {...props} /> : null}
    </Dialog>
  )
}

function DimensionForm({
  onOpenChange,
  kind,
  value,
  channels = [],
}: DimensionFormDialogProps) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [channelId, setChannelId] = useState<string | null>(
    value?.channelId ?? null
  )
  const [validFrom, setValidFrom] = useState(value?.validFrom ?? "")
  const [validTo, setValidTo] = useState(value?.validTo ?? "")
  const [budget, setBudget] = useState(value?.budgetAmount ?? "")
  const [description, setDescription] = useState(value?.description ?? "")
  const [isActive, setIsActive] = useState(value?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const label = DIMENSION_LABELS[kind]

  const save = useIdempotentMutation({
    mutationFn: (_: void, key): Promise<unknown> => {
      if (kind === "campaign") {
        const body = {
          name,
          validFrom,
          validTo,
          budgetAmount: budget || null,
          description,
          isActive,
        }
        return value
          ? updateCampaign(value.id, { ...body, version: value.version }, key)
          : createCampaign({ ...body, code }, key)
      }
      if (kind === "sub_channel") {
        return value
          ? updateSubChannel(
              value.id,
              { name, isActive, version: value.version },
              key
            )
          : createSubChannel(
              { channelId: channelId ?? "", code, name, isActive },
              key
            )
      }
      return value
        ? updateSimpleDimension(
            kind,
            value.id,
            { name, isActive, version: value.version },
            key
          )
        : createSimpleDimension(kind, { code, name, isActive }, key)
    },
    onSuccess: async () => {
      toast.success(value ? `${label} diperbarui` : `${label} ditambahkan`)
      await queryClient.invalidateQueries({ queryKey: ["accounting"] })
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
    if (!value && !code.trim()) next.code = "Kode wajib diisi"
    if (!name.trim()) next.name = "Nama wajib diisi"
    if (kind === "sub_channel" && !value && !channelId)
      next.channelId = "Channel wajib dipilih"
    if (kind === "campaign") {
      if (!validFrom) next.validFrom = "Tanggal mulai wajib diisi"
      if (!validTo) next.validTo = "Tanggal selesai wajib diisi"
      else if (validFrom && validTo < validFrom)
        next.validTo = "Tanggal selesai tidak boleh sebelum tanggal mulai"
    }
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {value ? `Ubah ${label} ${value.code}` : `Tambah ${label}`}
        </DialogTitle>
        <DialogDescription>
          Kode dipakai di jurnal dan laporan, tidak bisa diubah atau dipakai
          ulang.
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
          {kind === "sub_channel" ? (
            <Field data-invalid={!!errors.channelId}>
              <FieldLabel htmlFor="dimension-channel">Channel</FieldLabel>
              <DimensionSelect
                id="dimension-channel"
                value={channelId}
                options={channels}
                placeholder="Pilih channel"
                disabled={!!value}
                aria-invalid={!!errors.channelId}
                onValueChange={setChannelId}
              />
              <FieldError errors={err("channelId")} />
            </Field>
          ) : null}
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="dimension-code">Kode</FieldLabel>
            <Input
              id="dimension-code"
              value={code}
              disabled={!!value}
              aria-invalid={!!errors.code}
              onChange={(e) => setCode(e.target.value)}
            />
            <FieldDescription>{CODE_HINTS[kind]}</FieldDescription>
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="dimension-name">Nama</FieldLabel>
            <Input
              id="dimension-name"
              value={name}
              maxLength={150}
              aria-invalid={!!errors.name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          {kind === "campaign" ? (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field data-invalid={!!errors.validFrom}>
                  <FieldLabel htmlFor="campaign-from">Mulai</FieldLabel>
                  <DateInput
                    id="campaign-from"
                    value={validFrom}
                    onValueChange={setValidFrom}
                    aria-invalid={!!errors.validFrom}
                  />
                  <FieldError errors={err("validFrom")} />
                </Field>
                <Field data-invalid={!!errors.validTo}>
                  <FieldLabel htmlFor="campaign-to">Selesai</FieldLabel>
                  <DateInput
                    id="campaign-to"
                    value={validTo}
                    onValueChange={setValidTo}
                    aria-invalid={!!errors.validTo}
                  />
                  <FieldError errors={err("validTo")} />
                </Field>
              </div>
              <Field data-invalid={!!errors.budgetAmount}>
                <FieldLabel htmlFor="campaign-budget">
                  Anggaran (opsional)
                </FieldLabel>
                <MoneyInput
                  id="campaign-budget"
                  value={budget}
                  onValueChange={setBudget}
                  aria-invalid={!!errors.budgetAmount}
                />
                <FieldError errors={err("budgetAmount")} />
              </Field>
              <Field data-invalid={!!errors.description}>
                <FieldLabel htmlFor="campaign-description">
                  Keterangan (opsional)
                </FieldLabel>
                <Textarea
                  id="campaign-description"
                  value={description}
                  maxLength={500}
                  onChange={(e) => setDescription(e.target.value)}
                />
                <FieldError errors={err("description")} />
              </Field>
            </>
          ) : null}
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
    </DialogContent>
  )
}
