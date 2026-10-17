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
import { systemInfoQuery } from "@/features/system/api/system-queries"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  CUSTOMER_TYPE_LABELS,
  customersQuery,
  deleteCustomer,
  type Customer,
  type CustomerType,
} from "../api/party-api"
import { CustomerFormDialog } from "./customer-form-dialog"

const route = getRouteApi("/_app/master/customers")

const ALL = "all"

/** Data Master › Pelanggan (S-120, MD-06). */
export function CustomersPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("party.customer.write")
  const info = useQuery(systemInfoQuery)
  const customers = useQuery(customersQuery(search))
  const [form, setForm] = useState<{ customer?: Customer } | null>(null)
  const [deleting, setDeleting] = useState<Customer | null>(null)
  const [q, setQ] = useState(search.q ?? "")

  const remove = useIdempotentMutation({
    mutationFn: (c: Customer, key) => deleteCustomer(c.id, c.version, key),
    onSuccess: async () => {
      toast.success("Pelanggan dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({ queryKey: ["party", "customers"] })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<Customer>[] = [
    {
      id: "code",
      header: "Kode",
      sortKey: "code",
      cell: (c) => <span className="font-mono text-xs">{c.code}</span>,
    },
    {
      id: "name",
      header: "Nama",
      sortKey: "fullName",
      cell: (c) => (
        <div className="flex flex-col">
          <span className="font-medium">{c.fullName}</span>
          {c.email ? (
            <span className="text-xs text-muted-foreground">{c.email}</span>
          ) : null}
        </div>
      ),
    },
    { id: "phone", header: "WA/HP", cell: (c) => c.phone },
    {
      id: "type",
      header: "Tipe",
      cell: (c) => (
        <div className="flex flex-col items-start gap-1">
          <Badge variant="secondary">
            {CUSTOMER_TYPE_LABELS[c.customerType]}
          </Badge>
          {c.customerType === "employee" ? (
            <span
              className={
                c.isEmployeeActive
                  ? "text-xs text-muted-foreground"
                  : "text-xs text-destructive"
              }
            >
              {c.employeeName ?? "Karyawan"}
              {c.isEmployeeActive ? "" : " · sudah resign"}
            </span>
          ) : null}
        </div>
      ),
    },
    {
      id: "address",
      header: "Alamat utama",
      cell: (c) => {
        const a = c.addresses.find((x) => x.isDefault) ?? c.addresses[0]
        return a ? `${a.addressLine}, ${a.city}` : "-"
      },
    },
    {
      id: "consent",
      header: "Promo",
      cell: (c) => (c.marketingConsent ? "Setuju" : "-"),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (c) =>
        canWrite ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-11 sm:size-9"
                  aria-label={`Aksi untuk ${c.fullName}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setForm({ customer: c })}>
                Ubah
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setDeleting(c)}
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
          <h1 className="text-2xl font-semibold tracking-tight">Pelanggan</h1>
          <p className="text-sm text-muted-foreground">
            Data pelanggan bersama untuk penjualan, pengiriman, dan CRM.
          </p>
        </div>
        {canWrite ? (
          <Button className="min-h-11 sm:min-h-9" onClick={() => setForm({})}>
            <PlusIcon /> Tambah Pelanggan
          </Button>
        ) : null}
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
            placeholder="Cari nama, kode, atau nomor HP"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Cari Pelanggan"
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
        <Select
          value={search.customerType ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                customerType: v === ALL ? undefined : (v as CustomerType),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter tipe pelanggan">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua tipe"
                  : CUSTOMER_TYPE_LABELS[v as CustomerType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua tipe</SelectItem>
            {Object.entries(CUSTOMER_TYPE_LABELS).map(([k, label]) => (
              <SelectItem key={k} value={k}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={customers.data?.items}
        getRowId={(c) => c.id}
        loading={customers.isPending}
        fetching={customers.isFetching}
        page={search.page}
        totalPages={customers.data?.totalPages ?? 0}
        totalItems={customers.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          void navigate({ search: (s) => ({ ...s, sort, page: 1 }) })
        }
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada pelanggan yang cocok."
      />
      <CustomerFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        customer={form?.customer}
        employeesEnabled={info.data?.enabledModules.includes("hris") ?? false}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeleting(null)}
          title="Hapus pelanggan?"
          description={`${deleting.code} ${deleting.fullName}. Pelanggan tetap tampil di dokumen lama; nomor HP-nya bisa dipakai pelanggan baru.`}
          confirmLabel="Hapus Pelanggan"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </div>
  )
}
