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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteSupplier,
  SUPPLIER_TYPE_LABELS,
  suppliersQuery,
  WITHHOLDING_LABELS,
  type Supplier,
  type SupplierType,
} from "../api/party-api"
import { SupplierFormDialog } from "./supplier-form-dialog"

const route = getRouteApi("/_app/master/suppliers")

const ALL = "all"

/** Data Master › Pemasok & Maklon (S-122, MD-08). */
export function SuppliersPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("party.supplier.write")
  const suppliers = useQuery(suppliersQuery(search))
  const [form, setForm] = useState<{ supplier?: Supplier } | null>(null)
  const [deleting, setDeleting] = useState<Supplier | null>(null)
  const [q, setQ] = useState(search.q ?? "")

  const remove = useIdempotentMutation({
    mutationFn: (s: Supplier, key) => deleteSupplier(s.id, s.version, key),
    onSuccess: async () => {
      toast.success("Pemasok dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({ queryKey: ["party", "suppliers"] })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<Supplier>[] = [
    {
      id: "code",
      header: "Kode",
      sortKey: "code",
      cell: (s) => <span className="font-mono text-xs">{s.code}</span>,
    },
    {
      id: "name",
      header: "Nama",
      sortKey: "name",
      cell: (s) => (
        <div className="flex flex-col">
          <span className="font-medium">{s.name}</span>
          <span className="text-xs text-muted-foreground">
            {s.isBusinessEntity ? "Badan usaha" : "Perorangan"}
            {s.npwpMasked ? ` · NPWP ${s.npwpMasked}` : ""}
          </span>
        </div>
      ),
    },
    {
      id: "type",
      header: "Jenis",
      cell: (s) => (
        <Badge variant="secondary">
          {SUPPLIER_TYPE_LABELS[s.supplierType]}
        </Badge>
      ),
    },
    {
      id: "tax",
      header: "PPh default",
      cell: (s) => WITHHOLDING_LABELS[s.defaultWithholding],
    },
    {
      id: "term",
      header: "Termin",
      align: "right",
      cell: (s) => `${s.paymentTermDays} hari`,
    },
    {
      id: "bank",
      header: "Rekening utama",
      cell: (s) => {
        const b = s.bankAccounts.find((x) => x.isDefault) ?? s.bankAccounts[0]
        return b ? `${b.bankName} ${b.accountNumberMasked}` : "-"
      },
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (s) =>
        canWrite ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-11 sm:size-9"
                  aria-label={`Aksi untuk ${s.name}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setForm({ supplier: s })}>
                Ubah
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setDeleting(s)}
              >
                Hapus
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Pemasok & Maklon
          </h1>
          <p className="text-sm text-muted-foreground">
            Pemasok barang dan maklon beserta PPh default dan rekening
            pembayaran.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons kind="party.supplier" />
          {canWrite ? (
            <Button className="min-h-11 sm:min-h-9" onClick={() => setForm({})}>
              <PlusIcon /> Tambah Pemasok
            </Button>
          ) : null}
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <form
          className="flex max-w-md flex-1 gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            void navigate({
              search: (s) => ({ ...s, q: q || undefined, page: 1 }),
            })
          }}
        >
          <Input
            placeholder="Cari kode atau nama"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Cari Pemasok"
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
        <Select
          value={search.supplierType ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                supplierType: v === ALL ? undefined : (v as SupplierType),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter jenis pemasok">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Pemasok & maklon"
                  : SUPPLIER_TYPE_LABELS[v as SupplierType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Pemasok & maklon</SelectItem>
            {Object.entries(SUPPLIER_TYPE_LABELS).map(([k, label]) => (
              <SelectItem key={k} value={k}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={suppliers.data?.items}
        getRowId={(s) => s.id}
        loading={suppliers.isPending}
        fetching={suppliers.isFetching}
        page={search.page}
        totalPages={suppliers.data?.totalPages ?? 0}
        totalItems={suppliers.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          void navigate({ search: (s) => ({ ...s, sort, page: 1 }) })
        }
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada pemasok yang cocok."
      />
      <SupplierFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        supplier={form?.supplier}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeleting(null)}
          title="Hapus pemasok?"
          description={`${deleting.code} ${deleting.name}. Kode pemasok tidak bisa dipakai ulang.`}
          confirmLabel="Hapus Pemasok"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </div>
  )
}
