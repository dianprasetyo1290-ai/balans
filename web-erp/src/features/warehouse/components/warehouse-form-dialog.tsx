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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createWarehouse,
  updateWarehouse,
  type Warehouse,
} from "../api/warehouse-api"

type WarehouseFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = gudang baru. */
  warehouse?: Warehouse
}

/** Tambah/ubah gudang fisik (S-123). Gudang baru otomatis mendapat lokasi standar. */
export function WarehouseFormDialog(props: WarehouseFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <WarehouseForm {...props} /> : null}
    </Dialog>
  )
}

function WarehouseForm({ onOpenChange, warehouse }: WarehouseFormDialogProps) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(warehouse?.code ?? "")
  const [name, setName] = useState(warehouse?.name ?? "")
  const [address, setAddress] = useState(warehouse?.address ?? "")
  const [isActive, setIsActive] = useState(warehouse?.isActive ?? true)
  const [errors, setErrors] = useState<Record<string, string>>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      warehouse
        ? updateWarehouse(
            warehouse.id,
            { name, address, isActive, version: warehouse.version },
            key
          )
        : createWarehouse({ code, name, address, isActive }, key),
    onSuccess: async () => {
      toast.success(
        warehouse
          ? "Gudang diperbarui"
          : "Gudang dibuat beserta lokasi standarnya"
      )
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
    if (!warehouse && !code.trim()) next.code = "Kode wajib diisi"
    if (!name.trim()) next.name = "Nama wajib diisi"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])

  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>{warehouse ? "Ubah Gudang" : "Tambah Gudang"}</DialogTitle>
        <DialogDescription>
          {warehouse
            ? "Kode gudang tidak bisa diubah."
            : "Lokasi Inbound, FG Non-Ecommerce, E-commerce Stock (Petty Stock), RM, GM, dan Gudang Retur dibuat otomatis."}
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
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="warehouse-code">Kode</FieldLabel>
            <Input
              id="warehouse-code"
              value={code}
              disabled={!!warehouse}
              aria-invalid={!!errors.code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            {warehouse ? null : (
              <FieldDescription>
                Dipakai sebagai awalan kode lokasi, mis. UTAMA-INB. Tidak bisa
                dipakai ulang.
              </FieldDescription>
            )}
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="warehouse-name">Nama</FieldLabel>
            <Input
              id="warehouse-name"
              value={name}
              aria-invalid={!!errors.name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          <Field data-invalid={!!errors.address}>
            <FieldLabel htmlFor="warehouse-address">Alamat</FieldLabel>
            <Textarea
              id="warehouse-address"
              value={address}
              aria-invalid={!!errors.address}
              onChange={(e) => setAddress(e.target.value)}
            />
            <FieldError errors={err("address")} />
          </Field>
          <label className="flex min-h-11 items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="size-4 accent-(--color-primary)"
              checked={isActive}
              onChange={(e) => setIsActive(e.target.checked)}
            />
            Aktif
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
