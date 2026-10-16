import { useQuery, useQueryClient } from "@tanstack/react-query"
import { ShieldAlertIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { toast } from "sonner"

import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  permissionsQuery,
  rolesQuery,
  setRolePermissions,
  type Permission,
  type Role,
} from "../api/identity-api"

const MODULE_LABELS: Record<string, string> = {
  accounting: "Keuangan & Akuntansi",
  approval: "Persetujuan",
  audit: "Riwayat Perubahan",
  catalog: "Produk",
  dataexchange: "Impor/Ekspor",
  document: "Dokumen",
  hris: "SDM",
  identity: "Pengguna & Role",
  inventory: "Persediaan",
  logistics: "Logistik",
  party: "Pelanggan, Mitra & Pemasok",
  pricing: "Harga",
  procurement: "Pengadaan",
  sales: "Penjualan",
  setting: "Pengaturan",
  warehouse: "Gudang",
}

/** Pengaturan › Hak Akses (S-103). Perubahan berlaku langsung dan meminta konfirmasi identitas. */
export function RolesPage() {
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const roles = useQuery(rolesQuery)
  const perms = useQuery(permissionsQuery)
  const [roleId, setRoleId] = useState<string | null>(null)
  const [drafts, setDrafts] = useState<Record<string, Set<string>>>({})
  const canWrite = can("identity.role.write")

  const role: Role | undefined =
    roles.data?.find((r) => r.id === roleId) ?? roles.data?.[0]

  const draft = role
    ? (drafts[role.id] ?? new Set(role.permissions))
    : new Set<string>()
  const setDraft = (next: Set<string>) => {
    if (role) {
      setDrafts((d) => ({ ...d, [role.id]: next }))
    }
  }

  const grouped = useMemo(() => {
    const out = new Map<string, Permission[]>()
    for (const p of perms.data ?? []) {
      out.set(p.module, [...(out.get(p.module) ?? []), p])
    }
    return [...out.entries()].sort(([a], [b]) =>
      (MODULE_LABELS[a] ?? a).localeCompare(MODULE_LABELS[b] ?? b, "id")
    )
  }, [perms.data])

  const dirty = role
    ? role.permissions.length !== draft.size ||
      role.permissions.some((p) => !draft.has(p))
    : false

  const save = useIdempotentMutation({
    mutationFn: (r: Role, key) =>
      setRolePermissions(r.id, [...draft], r.version, key),
    onSuccess: async (_, r) => {
      toast.success(`Hak akses ${r.name} disimpan dan langsung berlaku`)
      setDrafts((d) => {
        const rest = { ...d }
        delete rest[r.id]
        return rest
      })
      await queryClient.invalidateQueries({ queryKey: ["identity"] })
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  const toggle = (code: string, on: boolean) => {
    const next = new Set(draft)
    if (on) {
      next.add(code)
    } else {
      next.delete(code)
    }
    setDraft(next)
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Hak Akses</h1>
        <p className="text-sm text-muted-foreground">
          Atur permission setiap role. Approval tetap mengikuti Matriks
          Persetujuan.
        </p>
      </div>
      <LoadingArea loading={roles.isPending || perms.isPending}>
        <div className="grid gap-4 lg:grid-cols-[16rem_1fr]">
          <nav
            aria-label="Daftar role"
            className="flex flex-row gap-1 overflow-x-auto lg:flex-col"
          >
            {(roles.data ?? []).map((r) => (
              <button
                key={r.id}
                type="button"
                onClick={() => setRoleId(r.id)}
                className={cn(
                  "flex min-h-10 shrink-0 items-center justify-between gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-muted",
                  role?.id === r.id && "bg-muted font-medium"
                )}
              >
                <span>{r.name}</span>
                <span className="text-xs text-muted-foreground">
                  {r.permissions.length}
                </span>
              </button>
            ))}
          </nav>
          {role ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex flex-wrap items-center gap-2">
                  {role.name}
                  {role.requiresMfa ? (
                    <Badge variant="secondary">Wajib dua langkah</Badge>
                  ) : null}
                  {role.hasDivisionScope ? (
                    <Badge variant="secondary">Terbatas divisi</Badge>
                  ) : null}
                </CardTitle>
                <CardDescription>{role.description}</CardDescription>
              </CardHeader>
              <CardContent className="flex flex-col gap-5">
                {grouped.map(([module, items]) => (
                  <fieldset key={module} className="flex flex-col gap-2">
                    <legend className="mb-1 text-sm font-semibold">
                      {MODULE_LABELS[module] ?? module}
                    </legend>
                    <div className="grid grid-cols-1 gap-x-4 gap-y-1 sm:grid-cols-2">
                      {items.map((p) => (
                        <label
                          key={p.code}
                          className="flex min-h-9 items-center gap-2 text-sm"
                        >
                          <input
                            type="checkbox"
                            className="size-4 accent-(--color-primary)"
                            checked={draft.has(p.code)}
                            disabled={!canWrite}
                            onChange={(e) => toggle(p.code, e.target.checked)}
                          />
                          <span>{p.description}</span>
                          {p.requiresReauth ? (
                            <ShieldAlertIcon
                              className="size-3.5 text-muted-foreground"
                              aria-label="Butuh konfirmasi identitas"
                            />
                          ) : null}
                        </label>
                      ))}
                    </div>
                  </fieldset>
                ))}
                {canWrite ? (
                  <div className="sticky bottom-0 flex justify-end gap-2 border-t bg-card pt-4">
                    <Button
                      variant="outline"
                      disabled={!dirty || save.isPending}
                      onClick={() => setDraft(new Set(role.permissions))}
                    >
                      Batalkan perubahan
                    </Button>
                    <LoadingButton
                      disabled={!dirty}
                      loading={save.isPending}
                      loadingText="Menyimpan…"
                      onClick={() => save.mutate(role)}
                    >
                      Simpan Hak Akses
                    </LoadingButton>
                  </div>
                ) : null}
              </CardContent>
            </Card>
          ) : null}
        </div>
      </LoadingArea>
    </div>
  )
}
