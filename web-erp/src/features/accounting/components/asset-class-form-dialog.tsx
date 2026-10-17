import { useQuery, useQueryClient } from "@tanstack/react-query"
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
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { accountsQuery } from "../api/accounting-api"
import {
  createAssetClass,
  updateAssetClass,
  type FixedAssetClass,
} from "../api/close-api"
import { Checkbox } from "./account-form-dialog"
import { AccountSelect } from "./account-select"

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  value?: FixedAssetClass
}

/** Tambah/ubah kelas aset: umur manfaat default dan akun aset & akumulasi penyusutan (A-31). */
export function AssetClassFormDialog(props: Props) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <AssetClassForm {...props} /> : null}
    </Dialog>
  )
}

function AssetClassForm({ onOpenChange, value }: Props) {
  const queryClient = useQueryClient()
  const accounts = useQuery(accountsQuery(false))
  const assetAccounts = (accounts.data ?? []).filter(
    (a) => a.group === 1 && a.isPostable && a.normalBalance === "debit"
  )
  const accumulatedAccounts = (accounts.data ?? []).filter(
    (a) => a.group === 1 && a.isPostable && a.normalBalance === "credit"
  )
  const [code, setCode] = useState(value?.code ?? "")
  const [name, setName] = useState(value?.name ?? "")
  const [life, setLife] = useState(String(value?.usefulLifeMonths ?? 48))
  const [assetAccountId, setAssetAccountId] = useState(
    value?.assetAccountId ?? ""
  )
  const [accumulatedAccountId, setAccumulatedAccountId] = useState(
    value?.accumulatedAccountId ?? ""
  )
  const [isActive, setIsActive] = useState(value?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key): Promise<unknown> => {
      const body = {
        name,
        usefulLifeMonths: Number(life),
        assetAccountId,
        accumulatedAccountId,
        isActive,
      }
      return value
        ? updateAssetClass(value.id, { ...body, version: value.version }, key)
        : createAssetClass({ ...body, code }, key)
    },
    onSuccess: async () => {
      toast.success(value ? "Kelas aset diperbarui" : "Kelas aset ditambahkan")
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "asset-classes"],
      })
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
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {value ? `Ubah Kelas Aset ${value.name}` : "Tambah Kelas Aset"}
        </DialogTitle>
        <DialogDescription>
          Umur manfaat baru dipakai aset yang ditambahkan setelahnya. Beban
          penyusutan dibukukan ke akun 6305 sesuai posting rule A-31.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="class-code">Kode</FieldLabel>
            <Input
              id="class-code"
              value={code}
              disabled={!!value}
              onChange={(e) => setCode(e.target.value.toLowerCase())}
            />
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="class-name">Nama</FieldLabel>
            <Input
              id="class-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          <Field data-invalid={!!errors.usefulLifeMonths}>
            <FieldLabel htmlFor="class-life">Umur Manfaat (bulan)</FieldLabel>
            <Input
              id="class-life"
              inputMode="numeric"
              value={life}
              onChange={(e) => setLife(e.target.value.replace(/\D/g, ""))}
            />
            <FieldError errors={err("usefulLifeMonths")} />
          </Field>
          <Field data-invalid={!!errors.assetAccountId}>
            <FieldLabel htmlFor="class-asset">Akun Aset</FieldLabel>
            <AccountSelect
              id="class-asset"
              value={assetAccountId}
              onValueChange={setAssetAccountId}
              accounts={assetAccounts}
            />
            <FieldError errors={err("assetAccountId")} />
          </Field>
          <Field data-invalid={!!errors.accumulatedAccountId}>
            <FieldLabel htmlFor="class-accumulated">
              Akun Akumulasi Penyusutan
            </FieldLabel>
            <AccountSelect
              id="class-accumulated"
              value={accumulatedAccountId}
              onValueChange={setAccumulatedAccountId}
              accounts={accumulatedAccounts}
            />
            <FieldError errors={err("accumulatedAccountId")} />
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
    </DialogContent>
  )
}
