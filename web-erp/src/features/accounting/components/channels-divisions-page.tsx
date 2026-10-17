import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { PlusIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { usePermissions } from "@/features/auth/api/auth-api"
import { cn } from "@/lib/utils"

import {
  channelsQuery,
  divisionsQuery,
  subChannelsQuery,
  type Dimension,
  type DimensionKind,
} from "../api/accounting-api"
import { Checkbox } from "./account-form-dialog"
import { DimensionFormDialog } from "./dimension-form-dialog"
import { DimensionTable } from "./dimension-table"

const route = getRouteApi("/_app/master/channels")

const TABS: {
  tab: "channels" | "sub-channels" | "divisions"
  label: string
  kind: DimensionKind
}[] = [
  { tab: "channels", label: "Channel", kind: "channel" },
  { tab: "sub-channels", label: "Sub-channel", kind: "sub_channel" },
  { tab: "divisions", label: "Divisi", kind: "division" },
]

/** Data Master › Channel & Divisi: dimensi jurnal C, S, D (S-129). */
export function ChannelsDivisionsPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const { can } = usePermissions()
  const canWrite = can("accounting.dimension.write")
  const channels = useQuery(channelsQuery(search.includeInactive))
  const subChannels = useQuery(subChannelsQuery(search.includeInactive))
  const divisions = useQuery(divisionsQuery(search.includeInactive))
  const [form, setForm] = useState<{
    kind: DimensionKind
    value?: Dimension
  } | null>(null)
  const current = TABS.find((t) => t.tab === search.tab) ?? TABS[0]
  const channelName = (id?: string | null) =>
    (channels.data ?? []).find((c) => c.id === id)?.name ?? "—"

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Channel & Divisi
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Dimensi yang dibawa setiap baris jurnal untuk laporan laba rugi per
            channel dan divisi. Penjualan wajib channel + sub-channel; biaya
            wajib divisi.
          </p>
        </div>
        {canWrite ? (
          <Button onClick={() => setForm({ kind: current.kind })}>
            <PlusIcon /> Tambah {current.label}
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="tablist" className="flex gap-1 rounded-lg bg-muted p-1">
          {TABS.map((t) => (
            <button
              key={t.tab}
              type="button"
              role="tab"
              aria-selected={t.tab === current.tab}
              className={cn(
                "min-h-11 rounded-md px-3 text-sm font-medium sm:min-h-9",
                t.tab === current.tab
                  ? "bg-background shadow-xs"
                  : "text-muted-foreground"
              )}
              onClick={() =>
                void navigate({ search: (s) => ({ ...s, tab: t.tab }) })
              }
            >
              {t.label}
            </button>
          ))}
        </div>
        <Checkbox
          label="Tampilkan yang nonaktif"
          checked={search.includeInactive}
          onChange={(v) =>
            void navigate({ search: (s) => ({ ...s, includeInactive: v }) })
          }
        />
      </div>
      {current.tab === "channels" ? (
        <DimensionTable
          rows={channels.data}
          loading={channels.isPending}
          canWrite={canWrite}
          onEdit={(value) => setForm({ kind: "channel", value })}
          emptyMessage="Belum ada channel."
        />
      ) : null}
      {current.tab === "sub-channels" ? (
        <DimensionTable
          rows={subChannels.data}
          loading={subChannels.isPending}
          canWrite={canWrite}
          extraColumns={[
            { header: "Channel", cell: (d) => channelName(d.channelId) },
          ]}
          onEdit={(value) => setForm({ kind: "sub_channel", value })}
          emptyMessage="Belum ada sub-channel."
        />
      ) : null}
      {current.tab === "divisions" ? (
        <DimensionTable
          rows={divisions.data}
          loading={divisions.isPending}
          canWrite={canWrite}
          onEdit={(value) => setForm({ kind: "division", value })}
          emptyMessage="Belum ada divisi."
        />
      ) : null}
      <DimensionFormDialog
        open={!!form}
        onOpenChange={(open) => !open && setForm(null)}
        kind={form?.kind ?? "channel"}
        value={form?.value}
        channels={channels.data}
      />
    </div>
  )
}
