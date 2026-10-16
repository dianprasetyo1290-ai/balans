import { zodResolver } from "@hookform/resolvers/zod"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useEffect } from "react"
import { useForm, useWatch } from "react-hook-form"
import { toast } from "sonner"
import { z } from "zod"

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
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createUser,
  rolesQuery,
  updateUser,
  type User,
} from "../api/identity-api"

const schema = z.object({
  email: z.email("Alamat email tidak valid"),
  fullName: z.string().trim().min(1).max(120, "Maksimal 120 karakter"),
  roles: z.array(z.string()).min(1, "Pilih minimal satu role"),
})

type Values = z.infer<typeof schema>

type UserFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = pengguna baru. */
  user?: User
}

/** Tambah/ubah pengguna (S-102). Pemberian role meminta konfirmasi identitas otomatis. */
export function UserFormDialog({
  open,
  onOpenChange,
  user,
}: UserFormDialogProps) {
  const queryClient = useQueryClient()
  const roles = useQuery({ ...rolesQuery, enabled: open })
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", fullName: "", roles: [] },
  })

  useEffect(() => {
    if (open) {
      form.reset({
        email: user?.email ?? "",
        fullName: user?.fullName ?? "",
        roles: user?.roles.map((r) => r.code) ?? [],
      })
    }
  }, [open, user, form])

  const save = useIdempotentMutation({
    mutationFn: (values: Values, key) => {
      const assignments = values.roles.map((code) => ({ code }))
      return user
        ? updateUser(
            user.id,
            {
              fullName: values.fullName,
              roles: assignments,
              version: user.version,
            },
            key
          )
        : createUser(
            {
              email: values.email,
              fullName: values.fullName,
              roles: assignments,
            },
            key
          )
    },
    onSuccess: async () => {
      toast.success(
        user
          ? "Pengguna diperbarui"
          : "Pengguna dibuat. Undangan atur password sudah dikirim ke email."
      )
      await queryClient.invalidateQueries({ queryKey: ["identity", "users"] })
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.code === "VALIDATION_FAILED" && problem.fields) {
        for (const [field, message] of Object.entries(problem.fields)) {
          form.setError(field as keyof Values, { message })
        }
        return
      }
      showErrorDialog(problem)
    },
  })

  const { errors } = form.formState
  const selected = useWatch({ control: form.control, name: "roles" })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {user ? "Ubah Pengguna" : "Tambah Pengguna"}
          </DialogTitle>
          <DialogDescription>
            {user
              ? "Perubahan role memutus sesi pengguna; pengguna perlu masuk ulang."
              : "Pengguna menerima email undangan untuk mengatur password."}
          </DialogDescription>
        </DialogHeader>
        <form noValidate onSubmit={form.handleSubmit((v) => save.mutate(v))}>
          <FieldGroup>
            <Field data-invalid={!!errors.email}>
              <FieldLabel htmlFor="user-email">Email</FieldLabel>
              <Input
                id="user-email"
                type="email"
                disabled={!!user}
                aria-invalid={!!errors.email}
                {...form.register("email")}
              />
              {user ? (
                <FieldDescription>Email tidak bisa diubah.</FieldDescription>
              ) : null}
              <FieldError errors={[errors.email]} />
            </Field>
            <Field data-invalid={!!errors.fullName}>
              <FieldLabel htmlFor="user-name">Nama lengkap</FieldLabel>
              <Input
                id="user-name"
                aria-invalid={!!errors.fullName}
                {...form.register("fullName")}
              />
              <FieldError errors={[errors.fullName]} />
            </Field>
            <FieldSet data-invalid={!!errors.roles}>
              <FieldLegend variant="label">Role</FieldLegend>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                {(roles.data ?? []).map((role) => {
                  const checked = selected.includes(role.code)
                  return (
                    <label
                      key={role.id}
                      className="flex min-h-11 cursor-pointer items-start gap-2 rounded-md border p-2 text-sm has-checked:border-primary/40 has-checked:bg-primary/5"
                    >
                      <input
                        type="checkbox"
                        className="mt-0.5 size-4 accent-(--color-primary)"
                        checked={checked}
                        onChange={(e) =>
                          form.setValue(
                            "roles",
                            e.target.checked
                              ? [...selected, role.code]
                              : selected.filter((c) => c !== role.code),
                            {
                              shouldValidate: true,
                              shouldDirty: true,
                            }
                          )
                        }
                      />
                      <span className="flex flex-col">
                        <span className="font-medium">{role.name}</span>
                        {role.requiresMfa ? (
                          <span className="text-xs text-muted-foreground">
                            Wajib autentikasi dua langkah
                          </span>
                        ) : null}
                      </span>
                    </label>
                  )
                })}
              </div>
              <FieldError errors={[errors.roles]} />
            </FieldSet>
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
    </Dialog>
  )
}
