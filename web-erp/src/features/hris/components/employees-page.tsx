import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon, SearchIcon, XIcon } from "lucide-react"
import { useState } from "react"

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
import { formatDate } from "@/lib/format/date"

import {
  employeesQuery,
  type Employee,
  type EmploymentStatus,
} from "../api/hris-api"
import { EmployeeFormDialog, ResignDialog } from "./employee-form-dialog"

const route = getRouteApi("/_app/hris/employees")

const ALL = "all"

const STATUS_LABELS: Record<EmploymentStatus, string> = {
  active: "Aktif",
  resigned: "Resign",
}

/** SDM › Data Karyawan (S-131, HR-01): data dasar, NIK terenkripsi, resign. */
export function EmployeesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const canWrite = can("hris.employee.write")
  // NIK hanya disimpan di state halaman, tidak di URL/riwayat browser (NFR-04).
  const [nik, setNik] = useState("")
  const [nikInput, setNikInput] = useState("")
  const employees = useQuery(employeesQuery({ ...search, nik }))
  const [form, setForm] = useState<{ employee?: Employee } | null>(null)
  const [resigning, setResigning] = useState<Employee | null>(null)
  const [q, setQ] = useState(search.q ?? "")

  const columns: DataTableColumn<Employee>[] = [
    {
      id: "number",
      header: "Nomor",
      sortKey: "employeeNumber",
      cell: (e) => (
        <span className="font-mono text-xs">{e.employeeNumber}</span>
      ),
    },
    {
      id: "name",
      header: "Nama",
      sortKey: "fullName",
      cell: (e) => (
        <div className="flex flex-col">
          <span className="font-medium">{e.fullName}</span>
          <span className="text-xs text-muted-foreground">
            {e.positionTitle}
          </span>
        </div>
      ),
    },
    {
      id: "join",
      header: "Bergabung",
      sortKey: "joinDate",
      cell: (e) => formatDate(e.joinDate),
    },
    {
      id: "status",
      header: "Status",
      cell: (e) =>
        e.isActive ? (
          <div className="flex flex-col items-start gap-1">
            <Badge className="bg-emerald-600/15 text-emerald-700 dark:text-emerald-300">
              Aktif
            </Badge>
            {e.resignDate ? (
              <span className="text-xs text-muted-foreground">
                Resign {formatDate(e.resignDate)}
              </span>
            ) : null}
          </div>
        ) : (
          <div className="flex flex-col items-start gap-1">
            <Badge variant="outline">Nonaktif</Badge>
            {e.resignDate ? (
              <span className="text-xs text-muted-foreground">
                Resign {formatDate(e.resignDate)}
              </span>
            ) : null}
          </div>
        ),
    },
    { id: "nik", header: "NIK", cell: (e) => e.nikMasked ?? "-" },
    {
      id: "contact",
      header: "Kontak",
      cell: (e) => (
        <div className="flex flex-col text-xs">
          <span>{e.workEmail || "-"}</span>
          <span className="text-muted-foreground">{e.phone}</span>
        </div>
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (e) =>
        canWrite ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-11 sm:size-9"
                  aria-label={`Aksi untuk ${e.fullName}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setForm({ employee: e })}>
                Ubah
              </DropdownMenuItem>
              {e.employmentStatus === "active" ? (
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setResigning(e)}
                >
                  Catat Resign
                </DropdownMenuItem>
              ) : null}
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
            Data Karyawan
          </h1>
          <p className="text-sm text-muted-foreground">
            Data dasar karyawan untuk akun ERP, harga karyawan, dan HRIS.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons kind="hris.employee" />
          {canWrite ? (
            <Button className="min-h-11 sm:min-h-9" onClick={() => setForm({})}>
              <PlusIcon /> Tambah Karyawan
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
            placeholder="Cari nomor atau nama"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Cari Karyawan"
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            setNik(nikInput.replace(/\D/g, ""))
          }}
        >
          <Input
            className="w-48"
            inputMode="numeric"
            autoComplete="off"
            placeholder="Cari NIK persis"
            value={nikInput}
            onChange={(e) => setNikInput(e.target.value)}
            aria-label="Cari NIK"
          />
          {nik ? (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Hapus filter NIK"
              onClick={() => {
                setNik("")
                setNikInput("")
              }}
            >
              <XIcon />
            </Button>
          ) : (
            <Button
              type="submit"
              variant="outline"
              size="icon"
              aria-label="Cari NIK"
            >
              <SearchIcon />
            </Button>
          )}
        </form>
        <Select
          value={search.status ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                status: v === ALL ? undefined : (v as EmploymentStatus),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-40" aria-label="Filter status karyawan">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua status"
                  : STATUS_LABELS[v as EmploymentStatus]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua status</SelectItem>
            {Object.entries(STATUS_LABELS).map(([k, label]) => (
              <SelectItem key={k} value={k}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={employees.data?.items}
        getRowId={(e) => e.id}
        loading={employees.isPending}
        fetching={employees.isFetching}
        page={search.page}
        totalPages={employees.data?.totalPages ?? 0}
        totalItems={employees.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          void navigate({ search: (s) => ({ ...s, sort, page: 1 }) })
        }
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada karyawan yang cocok."
      />
      <EmployeeFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        employee={form?.employee}
      />
      <ResignDialog
        employee={resigning}
        onOpenChange={(open) => !open && setResigning(null)}
      />
    </div>
  )
}
