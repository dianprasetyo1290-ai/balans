import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { cn } from "@/lib/utils"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteExpenseType,
  EXPENSE_APPROVAL_LABELS,
  expenseTypesQuery,
  WITHHOLDING_LABELS,
  type ExpenseType,
} from "../api/accounting-api"
import { Checkbox } from "./account-form-dialog"
import { ExpenseTypeFormDialog } from "./expense-type-form-dialog"

const route = getRouteApi("/_app/master/expense-types")

/** Data Master › Jenis Biaya: akun beban, PPh, bukti, kampanye, dan approval per jenis biaya (MD-13). */
export function ExpenseTypesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("accounting.expense_type.write")
  const types = useQuery(expenseTypesQuery(search.includeInactive, search.q))
  const [q, setQ] = useState(search.q ?? "")
  const [form, setForm] = useState<{ value?: ExpenseType } | null>(null)
  const [deleting, setDeleting] = useState<ExpenseType | null>(null)

  const remove = useIdempotentMutation({
    mutationFn: (e: ExpenseType, key) =>
      deleteExpenseType(e.id, e.version, key),
    onSuccess: async () => {
      toast.success("Jenis biaya dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({
        queryKey: ["accounting", "expense-types"],
      })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Jenis Biaya</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Dipakai pengajuan biaya, reimbursement, dan kas kecil: menentukan
            akun beban yang didebit, PPh yang dipotong, kewajiban bukti dan
            kampanye, serta alur persetujuan.
          </p>
        </div>
        {canWrite ? (
          <Button onClick={() => setForm({})}>
            <PlusIcon /> Tambah Jenis Biaya
          </Button>
        ) : null}
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
            aria-label="Cari jenis biaya"
            placeholder="Cari kode atau nama"
            className="pl-8"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </form>
        <Checkbox
          label="Tampilkan yang nonaktif"
          checked={search.includeInactive}
          onChange={(v) =>
            void navigate({ search: (s) => ({ ...s, includeInactive: v }) })
          }
        />
      </div>
      <LoadingArea loading={types.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kode</TableHead>
                <TableHead>Nama</TableHead>
                <TableHead>Akun Beban</TableHead>
                <TableHead>PPh</TableHead>
                <TableHead>Wajib</TableHead>
                <TableHead>Persetujuan</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(types.data ?? []).map((e) => (
                <TableRow
                  key={e.id}
                  className={cn(!e.isActive && "text-muted-foreground")}
                >
                  <TableCell className="font-medium whitespace-nowrap">
                    {e.code}
                  </TableCell>
                  <TableCell>{e.name}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {e.expenseAccountCode} {e.expenseAccountName}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {WITHHOLDING_LABELS[e.withholdingType]}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {e.requiresReceipt ? (
                        <Badge variant="outline">Bukti</Badge>
                      ) : null}
                      {e.requiresCampaign ? (
                        <Badge variant="outline">Kampanye</Badge>
                      ) : null}
                      {!e.requiresReceipt && !e.requiresCampaign ? "—" : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    {EXPENSE_APPROVAL_LABELS[e.approvalSubject]}
                  </TableCell>
                  <TableCell>
                    {e.isActive ? (
                      <Badge variant="secondary">Aktif</Badge>
                    ) : (
                      <Badge variant="outline">Nonaktif</Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    {canWrite ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon"
                              aria-label={`Aksi untuk ${e.code}`}
                            />
                          }
                        >
                          <MoreHorizontalIcon />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            onClick={() => setForm({ value: e })}
                          >
                            Ubah
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => setDeleting(e)}
                          >
                            Hapus
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
              {types.data && types.data.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={8}
                    className="text-center text-muted-foreground"
                  >
                    Belum ada jenis biaya.
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <ExpenseTypeFormDialog
        open={!!form}
        onOpenChange={(open) => !open && setForm(null)}
        value={form?.value}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setDeleting(null)}
          title={`Hapus jenis biaya ${deleting.code}?`}
          description="Jenis biaya yang dihapus tidak bisa dipilih di pengajuan biaya dan kas kecil. Kode tidak bisa dipakai ulang; nonaktifkan bila hanya ingin menyembunyikan sementara."
          confirmLabel="Hapus Jenis Biaya"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </div>
  )
}
