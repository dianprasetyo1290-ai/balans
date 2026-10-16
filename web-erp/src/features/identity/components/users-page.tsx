import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  resendInvitation,
  resetUserMfa,
  setUserActive,
  usersQuery,
  type User,
} from "../api/identity-api"
import { UserFormDialog } from "./user-form-dialog"

const route = getRouteApi("/_app/settings/users")

type PendingAction = {
  kind: "deactivate" | "activate" | "reset-mfa" | "invite"
  user: User
}

/** Pengaturan › Pengguna & Role (S-102). */
export function UsersPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const users = useQuery(usersQuery(search))
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<User | undefined>()
  const [pending, setPending] = useState<PendingAction | null>(null)
  const [q, setQ] = useState(search.q ?? "")
  const canWrite = can("identity.user.write")

  const action = useIdempotentMutation({
    mutationFn: (a: PendingAction, key) => {
      switch (a.kind) {
        case "deactivate":
          return setUserActive(a.user.id, false, a.user.version, key)
        case "activate":
          return setUserActive(a.user.id, true, a.user.version, key)
        case "reset-mfa":
          return resetUserMfa(a.user.id, key)
        case "invite":
          return resendInvitation(a.user.id, key)
      }
    },
    onSuccess: async (_, a) => {
      const messages = {
        deactivate: "Pengguna dinonaktifkan dan semua sesinya diputus",
        activate: "Pengguna diaktifkan",
        "reset-mfa": "Autentikasi dua langkah direset",
        invite: "Tautan dikirim ulang ke email pengguna",
      }
      toast.success(messages[a.kind])
      setPending(null)
      await queryClient.invalidateQueries({ queryKey: ["identity", "users"] })
    },
    onError: (err) => {
      setPending(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<User>[] = [
    {
      id: "name",
      header: "Nama",
      sortKey: "fullName",
      cell: (u) => (
        <div className="flex flex-col">
          <span className="font-medium">{u.fullName}</span>
          <span className="text-xs text-muted-foreground">{u.email}</span>
        </div>
      ),
    },
    {
      id: "roles",
      header: "Role",
      cell: (u) => (
        <div className="flex flex-wrap gap-1">
          {u.roles.map((r) => (
            <Badge key={r.code} variant="secondary">
              {r.name ?? r.code}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (u) =>
        !u.isActive ? (
          <Badge variant="outline">Nonaktif</Badge>
        ) : !u.hasPassword ? (
          <Badge
            variant="outline"
            className="border-amber-500/40 text-amber-700 dark:text-amber-300"
          >
            Menunggu undangan
          </Badge>
        ) : (
          <Badge className="bg-emerald-600/15 text-emerald-700 dark:text-emerald-300">
            Aktif
          </Badge>
        ),
    },
    {
      id: "mfa",
      header: "Dua langkah",
      cell: (u) => (u.isMfaEnabled ? "Aktif" : "—"),
    },
    {
      id: "lastLogin",
      header: "Terakhir masuk",
      sortKey: "lastLoginAt",
      cell: (u) => formatDateTime(u.lastLoginAt) || "—",
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (u) =>
        canWrite ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Aksi untuk ${u.fullName}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onClick={() => {
                  setEditing(u)
                  setFormOpen(true)
                }}
              >
                Ubah
              </DropdownMenuItem>
              {u.isActive ? (
                <DropdownMenuItem
                  onClick={() => setPending({ kind: "invite", user: u })}
                >
                  {u.hasPassword
                    ? "Kirim tautan atur ulang password"
                    : "Kirim ulang undangan"}
                </DropdownMenuItem>
              ) : null}
              {u.isMfaEnabled ? (
                <DropdownMenuItem
                  onClick={() => setPending({ kind: "reset-mfa", user: u })}
                >
                  Reset dua langkah
                </DropdownMenuItem>
              ) : null}
              {u.isActive ? (
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setPending({ kind: "deactivate", user: u })}
                >
                  Nonaktifkan
                </DropdownMenuItem>
              ) : (
                <DropdownMenuItem
                  onClick={() => setPending({ kind: "activate", user: u })}
                >
                  Aktifkan
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null,
    },
  ]

  const confirmText: Record<
    PendingAction["kind"],
    { title: string; description: string; confirm: string }
  > = {
    deactivate: {
      title: "Nonaktifkan pengguna?",
      description:
        "Pengguna tidak bisa masuk dan semua sesinya langsung diputus.",
      confirm: "Nonaktifkan",
    },
    activate: {
      title: "Aktifkan pengguna?",
      description: "Pengguna bisa masuk kembali dengan password lamanya.",
      confirm: "Aktifkan",
    },
    "reset-mfa": {
      title: "Reset autentikasi dua langkah?",
      description:
        "Pakai jika HP pengguna hilang. Pengguna mendaftar ulang saat masuk berikutnya.",
      confirm: "Reset",
    },
    invite: {
      title: "Kirim tautan ke email?",
      description: "Tautan sebelumnya tidak berlaku lagi.",
      confirm: "Kirim",
    },
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Pengguna & Role
          </h1>
          <p className="text-sm text-muted-foreground">
            Kelola akun, role, dan akses pengguna ERP.
          </p>
        </div>
        {canWrite ? (
          <Button
            onClick={() => {
              setEditing(undefined)
              setFormOpen(true)
            }}
          >
            <PlusIcon /> Tambah Pengguna
          </Button>
        ) : null}
      </div>
      <form
        className="flex max-w-md gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          void navigate({
            search: (s) => ({ ...s, q: q || undefined, page: 1 }),
          })
        }}
      >
        <Input
          placeholder="Cari nama atau email"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          aria-label="Cari pengguna"
        />
        <Button type="submit" variant="outline" size="icon" aria-label="Cari">
          <SearchIcon />
        </Button>
      </form>
      <DataTable
        columns={columns}
        rows={users.data?.items}
        getRowId={(u) => u.id}
        loading={users.isPending}
        fetching={users.isFetching}
        page={search.page}
        totalPages={users.data?.totalPages ?? 0}
        totalItems={users.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          void navigate({ search: (s) => ({ ...s, sort, page: 1 }) })
        }
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada pengguna yang cocok."
      />
      <UserFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        user={editing}
      />
      {pending ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setPending(null)}
          title={confirmText[pending.kind].title}
          description={`${pending.user.fullName} (${pending.user.email}). ${confirmText[pending.kind].description}`}
          confirmLabel={confirmText[pending.kind].confirm}
          destructive={pending.kind === "deactivate"}
          loading={action.isPending}
          onConfirm={() => action.mutate(pending)}
        />
      ) : null}
    </div>
  )
}
