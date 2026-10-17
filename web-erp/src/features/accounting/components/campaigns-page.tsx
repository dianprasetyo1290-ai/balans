import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { usePermissions } from "@/features/auth/api/auth-api"
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import { campaignsQuery, type Dimension } from "../api/accounting-api"
import { Checkbox } from "./account-form-dialog"
import { DimensionFormDialog } from "./dimension-form-dialog"
import { DimensionTable } from "./dimension-table"

const route = getRouteApi("/_app/master/campaigns")

/** Data Master › Kampanye: kode kampanye dengan periode berlaku (MD-13, dimensi K). */
export function CampaignsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const canWrite = can("accounting.dimension.write")
  const campaigns = useQuery(campaignsQuery(search.includeInactive, search.q))
  const [q, setQ] = useState(search.q ?? "")
  const [form, setForm] = useState<{ value?: Dimension } | null>(null)

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Kampanye</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Kode kampanye wajib di pemakaian internal marketing dan biaya
            endorsement/event/iklan; jurnal hanya menerima kampanye yang berlaku
            pada tanggal transaksi.
          </p>
        </div>
        {canWrite ? (
          <Button onClick={() => setForm({})}>
            <PlusIcon /> Tambah Kampanye
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
            aria-label="Cari kampanye"
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
      <DimensionTable
        rows={campaigns.data}
        loading={campaigns.isPending}
        canWrite={canWrite}
        extraColumns={[
          {
            header: "Periode",
            cell: (d) => (
              <span className="whitespace-nowrap">
                {formatDate(d.validFrom)} – {formatDate(d.validTo)}
              </span>
            ),
          },
          {
            header: "Anggaran",
            cell: (d) => (d.budgetAmount ? formatRupiah(d.budgetAmount) : "—"),
          },
        ]}
        onEdit={(value) => setForm({ value })}
        emptyMessage="Belum ada kampanye."
      />
      <DimensionFormDialog
        open={!!form}
        onOpenChange={(open) => !open && setForm(null)}
        kind="campaign"
        value={form?.value}
      />
    </div>
  )
}
