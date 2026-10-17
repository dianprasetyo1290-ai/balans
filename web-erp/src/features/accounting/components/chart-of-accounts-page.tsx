import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useMemo, useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  accountsQuery,
  deleteAccount,
  type Account,
} from "../api/accounting-api"
import { accountTree } from "../lib/account-tree"
import { AccountFormDialog, Checkbox } from "./account-form-dialog"

const route = getRouteApi("/_app/finance/accounts")

/** Keuangan & Akuntansi › Chart of Accounts (S-126). */
export function ChartOfAccountsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("accounting.account.write")
  const accounts = useQuery(accountsQuery(search.includeInactive))
  const [q, setQ] = useState(search.q ?? "")
  const [form, setForm] = useState<{
    account?: Account
    parent?: Account
  } | null>(null)
  const [deleting, setDeleting] = useState<Account | null>(null)

  const remove = useIdempotentMutation({
    mutationFn: (a: Account, key) => deleteAccount(a.id, a.version, key),
    onSuccess: async () => {
      toast.success("Akun dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "accounts"],
      })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const rows = useMemo(() => {
    const all = accountTree(accounts.data ?? [])
    const term = (search.q ?? "").trim().toLowerCase()
    if (!term) return all
    return all.filter(
      (r) =>
        r.account.code.includes(term) ||
        r.account.name.toLowerCase().includes(term)
    )
  }, [accounts.data, search.q])

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Chart of Accounts
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Kode 4 digit, digit pertama kelompok akun. Akun induk tidak bisa
            diposting; akun yang sudah dipakai jurnal tidak bisa dihapus.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons kind="accounting.account" />
          {canWrite ? (
            <Button onClick={() => setForm({})}>
              <PlusIcon /> Tambah Akun
            </Button>
          ) : null}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <form
          className="relative w-full sm:w-72"
          onSubmit={(e) => {
            e.preventDefault()
            void navigate({ search: (s) => ({ ...s, q: q || undefined }) })
          }}
        >
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            aria-label="Cari akun"
            placeholder="Cari kode atau nama akun"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Checkbox
          label="Tampilkan akun nonaktif"
          checked={search.includeInactive}
          onChange={(v) =>
            void navigate({ search: (s) => ({ ...s, includeInactive: v }) })
          }
        />
      </div>
      <LoadingArea loading={accounts.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-24">Kode</TableHead>
                <TableHead>Nama Akun</TableHead>
                <TableHead>Kelompok</TableHead>
                <TableHead>Saldo Normal</TableHead>
                <TableHead>Posting</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map(({ account: a, depth }) => (
                <TableRow
                  key={a.id}
                  className={cn(!a.isActive && "text-muted-foreground")}
                >
                  <TableCell
                    className={cn(
                      "tabular-nums",
                      !a.isPostable && "font-semibold"
                    )}
                  >
                    {a.code}
                  </TableCell>
                  <TableCell>
                    <span
                      className={cn(!a.isPostable && "font-semibold")}
                      style={{ paddingLeft: `${depth * 1.25}rem` }}
                    >
                      {a.name}
                    </span>
                    {!a.isActive ? (
                      <Badge variant="outline" className="ml-2">
                        Nonaktif
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {a.groupLabel}
                  </TableCell>
                  <TableCell>
                    {a.normalBalance === "debit" ? "Debit" : "Kredit"}
                  </TableCell>
                  <TableCell className="text-xs whitespace-nowrap">
                    {!a.isPostable
                      ? "Akun induk"
                      : a.allowManualPosting
                        ? "Manual & otomatis"
                        : "Hanya otomatis"}
                  </TableCell>
                  <TableCell className="text-right">
                    {canWrite ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon"
                              aria-label={`Aksi untuk akun ${a.code}`}
                            />
                          }
                        >
                          <MoreHorizontalIcon />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => setForm({ account: a })}
                          >
                            Ubah
                          </DropdownMenuItem>
                          {!a.isPostable ? (
                            <DropdownMenuItem
                              onClick={() => setForm({ parent: a })}
                            >
                              Tambah Sub-akun
                            </DropdownMenuItem>
                          ) : null}
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => setDeleting(a)}
                          >
                            Hapus
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
              {rows.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={6}
                    className="text-center text-muted-foreground"
                  >
                    Tidak ada akun yang cocok.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <AccountFormDialog
        open={!!form}
        onOpenChange={(open) => !open && setForm(null)}
        accounts={accounts.data ?? []}
        account={form?.account}
        defaultParent={form?.parent}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setDeleting(null)}
          title={`Hapus akun ${deleting.code} ${deleting.name}?`}
          description="Akun yang sudah dipakai jurnal tidak bisa dihapus; nonaktifkan akun tersebut. Kode akun tidak bisa dipakai ulang."
          confirmLabel="Hapus Akun"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </div>
  )
}
