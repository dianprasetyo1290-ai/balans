import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  categoriesQuery,
  createCategory,
  deleteCategory,
  restoreCategory,
  updateCategory,
  type ProductCategory,
} from "../api/catalog-api"

type CategoriesDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Kelola kategori produk: tambah, ubah nama, hapus (soft delete), dan pulihkan oleh Admin Sistem. */
export function CategoriesDialog({
  open,
  onOpenChange,
}: CategoriesDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <CategoriesPanel /> : null}
    </Dialog>
  )
}

type Action =
  | { kind: "create"; code: string; name: string }
  | { kind: "rename"; category: ProductCategory; name: string }
  | { kind: "delete" | "restore"; category: ProductCategory }

function CategoriesPanel() {
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("catalog.product.write")
  const [showDeleted, setShowDeleted] = useState(false)
  const categories = useQuery(categoriesQuery(showDeleted))
  const [code, setCode] = useState("")
  const [name, setName] = useState("")
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(
    null
  )
  const [errors, setErrors] = useState<Record<string, string>>({})

  const run = useIdempotentMutation({
    mutationFn: (a: Action, key) => {
      switch (a.kind) {
        case "create":
          return createCategory(a.code, a.name, key)
        case "rename":
          return updateCategory(a.category.id, a.name, a.category.version, key)
        case "delete":
          return deleteCategory(a.category.id, key)
        case "restore":
          return restoreCategory(a.category.id, key)
      }
    },
    onSuccess: async (_, a) => {
      const messages = {
        create: "Kategori ditambahkan",
        rename: "Kategori diperbarui",
        delete: "Kategori dihapus",
        restore: "Kategori dipulihkan",
      }
      toast.success(messages[a.kind])
      if (a.kind === "create") {
        setCode("")
        setName("")
      }
      setEditing(null)
      setErrors({})
      await queryClient.invalidateQueries({ queryKey: ["catalog"] })
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
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>Kategori Produk</DialogTitle>
        <DialogDescription>
          Kategori dipakai untuk laporan margin per kategori. Kode tidak bisa
          diubah dan tidak bisa dipakai ulang.
        </DialogDescription>
      </DialogHeader>
      {canWrite && !showDeleted ? (
        <form
          noValidate
          className="grid gap-2 sm:grid-cols-[8rem_1fr_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            run.mutate({ kind: "create", code, name })
          }}
        >
          <Field data-invalid={!!errors.code}>
            <FieldLabel htmlFor="category-code">Kode</FieldLabel>
            <Input
              id="category-code"
              value={code}
              aria-invalid={!!errors.code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
            />
            <FieldError errors={err("code")} />
          </Field>
          <Field data-invalid={!!errors.name}>
            <FieldLabel htmlFor="category-name">Nama</FieldLabel>
            <Input
              id="category-name"
              value={name}
              aria-invalid={!!errors.name}
              onChange={(e) => setName(e.target.value)}
            />
            <FieldError errors={err("name")} />
          </Field>
          <LoadingButton
            type="submit"
            loading={run.isPending && run.variables?.kind === "create"}
            loadingText="Menyimpan…"
          >
            Tambah Kategori
          </LoadingButton>
        </form>
      ) : null}
      <label className="flex min-h-11 items-center gap-2 text-sm">
        <input
          type="checkbox"
          className="size-4 accent-(--color-primary)"
          checked={showDeleted}
          onChange={(e) => setShowDeleted(e.target.checked)}
        />
        Tampilkan kategori yang dihapus
      </label>
      <LoadingArea loading={categories.isPending}>
        <ul className="flex flex-col divide-y rounded-lg border">
          {(categories.data ?? []).map((c) => (
            <li key={c.id} className="flex flex-wrap items-center gap-2 p-2">
              <span className="w-24 font-mono text-xs">{c.code}</span>
              {editing?.id === c.id ? (
                <form
                  className="flex flex-1 gap-2"
                  onSubmit={(e) => {
                    e.preventDefault()
                    run.mutate({
                      kind: "rename",
                      category: c,
                      name: editing.name,
                    })
                  }}
                >
                  <Input
                    aria-label={`Nama kategori ${c.code}`}
                    value={editing.name}
                    onChange={(e) =>
                      setEditing({ id: c.id, name: e.target.value })
                    }
                  />
                  <LoadingButton
                    type="submit"
                    size="sm"
                    loading={run.isPending}
                    loadingText="Menyimpan…"
                  >
                    Simpan
                  </LoadingButton>
                </form>
              ) : (
                <span className="flex-1 text-sm">{c.name}</span>
              )}
              {editing?.id !== c.id && c.allowedActions.includes("update") ? (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setEditing({ id: c.id, name: c.name })}
                >
                  Ubah
                </Button>
              ) : null}
              {c.allowedActions.includes("delete") ? (
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive"
                  disabled={run.isPending}
                  onClick={() => run.mutate({ kind: "delete", category: c })}
                >
                  Hapus
                </Button>
              ) : null}
              {c.allowedActions.includes("restore") ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={run.isPending}
                  onClick={() => run.mutate({ kind: "restore", category: c })}
                >
                  Pulihkan
                </Button>
              ) : null}
            </li>
          ))}
          {categories.data && categories.data.length === 0 ? (
            <li className="p-3 text-sm text-muted-foreground">
              {showDeleted
                ? "Tidak ada kategori yang dihapus."
                : "Belum ada kategori."}
            </li>
          ) : null}
        </ul>
      </LoadingArea>
    </DialogContent>
  )
}
