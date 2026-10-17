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
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  AGREEMENT_TYPE_LABELS,
  deletePartner,
  PARTNER_TYPE_LABELS,
  partnersQuery,
  type Partner,
  type PartnerType,
} from "../api/party-api"
import { AgreementsSheet } from "./agreements-sheet"
import { PartnerFormDialog } from "./partner-form-dialog"

const route = getRouteApi("/_app/master/partners")

const ALL = "all"

/** Data Master › Mitra Konsinyasi: mitra MT/konsinyasi/B2B beserta PKS (S-121, MD-07). */
export function PartnersPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("party.partner.write")
  const canReadAgreements = can("party.partner_agreement.read")
  const partners = useQuery(partnersQuery(search))
  const [form, setForm] = useState<{ partner?: Partner } | null>(null)
  const [agreementsOf, setAgreementsOf] = useState<Partner | null>(null)
  const [deleting, setDeleting] = useState<Partner | null>(null)
  const [q, setQ] = useState(search.q ?? "")

  const remove = useIdempotentMutation({
    mutationFn: (p: Partner, key) => deletePartner(p.id, p.version, key),
    onSuccess: async () => {
      toast.success("Mitra dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({ queryKey: ["party", "partners"] })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<Partner>[] = [
    {
      id: "code",
      header: "Kode",
      sortKey: "code",
      cell: (p) => <span className="font-mono text-xs">{p.code}</span>,
    },
    {
      id: "name",
      header: "Nama mitra",
      sortKey: "name",
      cell: (p) => (
        <div className="flex flex-col">
          <span className="font-medium">{p.name}</span>
          <span className="text-xs text-muted-foreground">
            {PARTNER_TYPE_LABELS[p.partnerType]}
          </span>
        </div>
      ),
    },
    {
      id: "agreements",
      header: "PKS berlaku",
      cell: (p) =>
        p.activeAgreements && p.activeAgreements.length > 0 ? (
          <div className="flex flex-col gap-1">
            {p.activeAgreements.map((a) => (
              <span key={a.id} className="text-xs">
                <Badge variant="secondary" className="mr-1">
                  {AGREEMENT_TYPE_LABELS[a.agreementType]}
                </Badge>
                s.d. {formatDate(a.validTo)}
              </span>
            ))}
          </div>
        ) : canReadAgreements ? (
          <span className="text-xs text-destructive">
            Tidak ada PKS berlaku
          </span>
        ) : (
          "-"
        ),
    },
    {
      id: "people",
      header: "Sales / Merchandiser",
      cell: (p) => (
        <div className="flex flex-col text-xs">
          <span>{p.salesUserName ?? "-"}</span>
          <span className="text-muted-foreground">
            {p.merchandiserUserName ?? "-"}
          </span>
        </div>
      ),
    },
    {
      id: "pic",
      header: "PIC mitra",
      cell: (p) => (
        <div className="flex flex-col text-xs">
          <span>{p.picName || "-"}</span>
          <span className="text-muted-foreground">{p.picPhone}</span>
        </div>
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (p) => (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon"
                className="size-11 sm:size-9"
                aria-label={`Aksi untuk ${p.name}`}
              />
            }
          >
            <MoreHorizontalIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {canReadAgreements ? (
              <DropdownMenuItem onClick={() => setAgreementsOf(p)}>
                Lihat PKS
              </DropdownMenuItem>
            ) : null}
            {canWrite ? (
              <>
                <DropdownMenuItem onClick={() => setForm({ partner: p })}>
                  Ubah
                </DropdownMenuItem>
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => setDeleting(p)}
                >
                  Hapus
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Mitra Konsinyasi
          </h1>
          <p className="text-sm text-muted-foreground">
            Mitra Modern Trade, konsinyasi, dan B2B beserta PKS, Sales, dan
            Merchandiser.
          </p>
        </div>
        {canWrite ? (
          <Button className="min-h-11 sm:min-h-9" onClick={() => setForm({})}>
            <PlusIcon /> Tambah Mitra
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
            placeholder="Cari kode atau nama mitra"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Cari Mitra"
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
        <Select
          value={search.partnerType ?? ALL}
          onValueChange={(v) =>
            void navigate({
              search: (s) => ({
                ...s,
                partnerType: v === ALL ? undefined : (v as PartnerType),
                page: 1,
              }),
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Filter jenis mitra">
            <SelectValue>
              {(v: string) =>
                v === ALL
                  ? "Semua jenis"
                  : PARTNER_TYPE_LABELS[v as PartnerType]
              }
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>Semua jenis</SelectItem>
            {Object.entries(PARTNER_TYPE_LABELS).map(([k, label]) => (
              <SelectItem key={k} value={k}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={partners.data?.items}
        getRowId={(p) => p.id}
        loading={partners.isPending}
        fetching={partners.isFetching}
        page={search.page}
        totalPages={partners.data?.totalPages ?? 0}
        totalItems={partners.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) =>
          void navigate({ search: (s) => ({ ...s, sort, page: 1 }) })
        }
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada mitra yang cocok."
      />
      <PartnerFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        partner={form?.partner}
      />
      <AgreementsSheet
        partner={agreementsOf}
        onOpenChange={(open) => !open && setAgreementsOf(null)}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeleting(null)}
          title="Hapus mitra?"
          description={`${deleting.code} ${deleting.name}. Kode mitra tidak bisa dipakai ulang.`}
          confirmLabel="Hapus Mitra"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </div>
  )
}
