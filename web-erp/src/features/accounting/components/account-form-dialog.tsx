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
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createAccount,
  updateAccount,
  type Account,
} from "../api/accounting-api"

type AccountFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  accounts: Account[]
  /** Kosong = akun baru. */
  account?: Account
  /** Induk awal untuk akun baru. */
  defaultParent?: Account
}

type Field =
  | "code"
  | "name"
  | "normalBalance"
  | "parentId"
  | "isPostable"
  | "allowManualPosting"
  | "description"

const NO_PARENT = "none"

/** Default saldo normal per kelompok: liabilitas, ekuitas, pendapatan bersaldo kredit. */
function defaultNormalBalance(code: string): "debit" | "credit" {
  return ["2", "3", "4"].includes(code.charAt(0)) ? "credit" : "debit"
}

/** Tambah atau ubah akun Chart of Accounts (S-126). Kode akun tidak bisa diubah. */
export function AccountFormDialog(props: AccountFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <AccountForm {...props} /> : null}
    </Dialog>
  )
}

function AccountForm({
  onOpenChange,
  accounts,
  account,
  defaultParent,
}: AccountFormDialogProps) {
  const queryClient = useQueryClient()
  const [code, setCode] = useState(account?.code ?? "")
  const [name, setName] = useState(account?.name ?? "")
  const [normalBalance, setNormalBalance] = useState<"debit" | "credit">(
    account?.normalBalance ??
      (defaultParent ? defaultParent.normalBalance : "debit")
  )
  const [parentId, setParentId] = useState(
    account?.parentId ?? defaultParent?.id ?? NO_PARENT
  )
  const [isPostable, setIsPostable] = useState(account?.isPostable ?? true)
  const [allowManual, setAllowManual] = useState(
    account?.allowManualPosting ?? false
  )
  const [isActive, setIsActive] = useState(account?.isActive ?? true)
  const [description, setDescription] = useState(account?.description ?? "")
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({})

  const group = (account?.code ?? code).charAt(0)
  const parents = accounts.filter(
    (a) => !a.isPostable && a.id !== account?.id && a.code.charAt(0) === group
  )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const fields = {
        name,
        normalBalance,
        parentId: parentId === NO_PARENT ? null : parentId,
        isPostable,
        allowManualPosting: isPostable && allowManual,
        isActive,
        description,
      }
      return account
        ? updateAccount(
            account.id,
            { ...fields, version: account.version },
            key
          )
        : createAccount({ ...fields, code }, key)
    },
    onSuccess: async () => {
      toast.success(account ? "Akun diperbarui" : "Akun ditambahkan")
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "accounts"],
      })
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields as Partial<Record<Field, string>>)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    const next: Partial<Record<Field, string>> = {}
    if (!account && !/^[1-7][0-9]{3}$/.test(code)) {
      next.code = "Kode akun 4 digit, diawali kelompok 1–7"
    }
    if (!name.trim()) next.name = "Nama akun wajib diisi"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  const err = (k: Field) => (errors[k] ? [{ message: errors[k] }] : [])
  const parentLabel = (id: string) => {
    const p = accounts.find((a) => a.id === id)
    return p ? `${p.code} ${p.name}` : "Tanpa induk"
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {account ? `Ubah Akun ${account.code}` : "Tambah Akun"}
        </DialogTitle>
        <DialogDescription>
          Kode akun tidak bisa diubah atau dipakai ulang walau akun dihapus.
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
          <div className="grid gap-4 sm:grid-cols-[8rem_1fr]">
            <Field data-invalid={!!errors.code}>
              <FieldLabel htmlFor="account-code">Kode</FieldLabel>
              <Input
                id="account-code"
                inputMode="numeric"
                maxLength={4}
                value={code}
                disabled={!!account}
                aria-invalid={!!errors.code}
                onChange={(e) => {
                  setCode(e.target.value)
                  if (!defaultParent && e.target.value.length === 1) {
                    setNormalBalance(defaultNormalBalance(e.target.value))
                  }
                }}
              />
              <FieldError errors={err("code")} />
            </Field>
            <Field data-invalid={!!errors.name}>
              <FieldLabel htmlFor="account-name">Nama akun</FieldLabel>
              <Input
                id="account-name"
                value={name}
                maxLength={150}
                aria-invalid={!!errors.name}
                onChange={(e) => setName(e.target.value)}
              />
              <FieldError errors={err("name")} />
            </Field>
          </div>
          <Field data-invalid={!!errors.parentId}>
            <FieldLabel htmlFor="account-parent">Akun induk</FieldLabel>
            <Select
              value={parentId}
              onValueChange={(v) => setParentId(String(v ?? NO_PARENT))}
            >
              <SelectTrigger
                id="account-parent"
                className="w-full"
                aria-invalid={!!errors.parentId}
              >
                <SelectValue>
                  {(v: string) =>
                    v === NO_PARENT ? "Tanpa induk" : parentLabel(v)
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NO_PARENT}>Tanpa induk</SelectItem>
                {parents.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.code} {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              Hanya akun induk (judul) di kelompok yang sama.
            </FieldDescription>
            <FieldError errors={err("parentId")} />
          </Field>
          <Field data-invalid={!!errors.normalBalance}>
            <FieldLabel htmlFor="account-normal">Saldo normal</FieldLabel>
            <Select
              value={normalBalance}
              onValueChange={(v) =>
                setNormalBalance(v === "credit" ? "credit" : "debit")
              }
            >
              <SelectTrigger id="account-normal" className="w-full">
                <SelectValue>
                  {(v: string) => (v === "credit" ? "Kredit" : "Debit")}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="debit">Debit</SelectItem>
                <SelectItem value="credit">Kredit</SelectItem>
              </SelectContent>
            </Select>
            <FieldError errors={err("normalBalance")} />
          </Field>
          <div className="flex flex-col gap-2">
            <Checkbox
              label="Bisa diposting (bukan akun induk)"
              checked={isPostable}
              onChange={setIsPostable}
            />
            <FieldError errors={err("isPostable")} />
            <Checkbox
              label="Boleh dipakai di Jurnal Umum"
              checked={isPostable && allowManual}
              disabled={!isPostable}
              onChange={setAllowManual}
            />
            <p className="text-xs text-muted-foreground">
              Akun kontrol (piutang, utang, persediaan) hanya diisi jurnal
              otomatis.
            </p>
            <Checkbox label="Aktif" checked={isActive} onChange={setIsActive} />
          </div>
          <Field data-invalid={!!errors.description}>
            <FieldLabel htmlFor="account-description">
              Keterangan (opsional)
            </FieldLabel>
            <Textarea
              id="account-description"
              value={description}
              maxLength={500}
              onChange={(e) => setDescription(e.target.value)}
            />
            <FieldError errors={err("description")} />
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

export function Checkbox({
  label,
  checked,
  onChange,
  disabled,
}: {
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
}) {
  return (
    <label className="flex min-h-11 w-fit items-center gap-2 text-sm has-disabled:opacity-60">
      <input
        type="checkbox"
        className="size-4 accent-(--color-primary)"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      {label}
    </label>
  )
}
