import { useQuery, useQueryClient } from "@tanstack/react-query"
import { MoreHorizontalIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Checkbox } from "@/features/accounting/components/account-form-dialog"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  PARTY_TYPE_LABELS,
  deleteDated,
  promotionsQuery,
  specialPricesQuery,
  tieredPricesQuery,
  type DatedResource,
} from "../api/pricing-api"
import { PromotionFormDialog } from "./promotion-form-dialog"
import {
  EndPeriodDialog,
  PeriodBadge,
  formatPeriod,
  useChannelNames,
} from "./pricing-parts"
import {
  SpecialPriceFormDialog,
  TieredPriceFormDialog,
} from "./rule-form-dialogs"

export type PriceRulesTab = "promotions" | "tiered" | "special"

type PriceRulesPageProps = {
  tab: PriceRulesTab
  includeEnded: boolean
  onSearchChange: (patch: {
    tab?: PriceRulesTab
    includeEnded?: boolean
  }) => void
}

/**
 * Data Master › Aturan Harga (BRD 5.18, S-118): promo periode per channel, harga bertingkat per qty minimum, dan
 * harga khusus per pelanggan/mitra. Urutan prioritas: harga khusus → promo → harga bertingkat → daftar harga →
 * harga dasar.
 */
export function PriceRulesPage({
  tab,
  includeEnded,
  onSearchChange,
}: PriceRulesPageProps) {
  const { can } = usePermissions()
  const tabs = [
    can("pricing.promotion.read")
      ? { id: "promotions" as const, label: "Promo" }
      : null,
    can("pricing.promotion.read")
      ? { id: "tiered" as const, label: "Harga Bertingkat" }
      : null,
    can("pricing.special_price.read")
      ? { id: "special" as const, label: "Harga Khusus" }
      : null,
  ].filter((t) => t !== null)
  const active = tabs.some((t) => t.id === tab) ? tab : (tabs[0]?.id ?? tab)

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Aturan Harga</h1>
        <p className="max-w-3xl text-sm text-muted-foreground">
          Harga di transaksi ditentukan berurutan: harga khusus pelanggan/mitra
          → promo aktif → harga bertingkat → daftar harga channel/mitra/karyawan
          → harga dasar. Perubahan berlaku mulai tanggal yang dipilih; transaksi
          lama tidak berubah.
        </p>
      </div>
      <PageTabs
        label="Jenis aturan harga"
        value={active}
        onValueChange={(t) => onSearchChange({ tab: t })}
        tabs={tabs}
      />
      <Checkbox
        label="Tampilkan yang sudah berakhir"
        checked={includeEnded}
        onChange={(v) => onSearchChange({ includeEnded: v })}
      />
      {active === "promotions" ? (
        <PromotionsTab
          includeEnded={includeEnded}
          canWrite={can("pricing.promotion.write")}
        />
      ) : active === "tiered" ? (
        <TieredTab
          includeEnded={includeEnded}
          canWrite={can("pricing.promotion.write")}
        />
      ) : (
        <SpecialTab
          includeEnded={includeEnded}
          canWrite={can("pricing.special_price.write")}
        />
      )}
    </div>
  )
}

type DatedRow = { id: string; version: number; validFrom: string }

/** Aksi Akhiri & Hapus untuk data effective-dated. */
export function useDatedActions(
  resource: DatedResource | "promotions",
  queryKey: readonly unknown[]
) {
  const queryClient = useQueryClient()
  const [ending, setEnding] = useState<DatedRow | null>(null)
  const [deleting, setDeleting] = useState<DatedRow | null>(null)
  const remove = useIdempotentMutation({
    mutationFn: (r: DatedRow, key) =>
      deleteDated(resource, r.id, r.version, key),
    onSuccess: async () => {
      toast.success("Data dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({ queryKey })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })
  return { ending, setEnding, deleting, setDeleting, remove }
}

export function RowMenu({
  label,
  items,
}: {
  label: string
  items: { label: string; onClick: () => void; destructive?: boolean }[]
}) {
  if (items.length === 0) return null
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className="size-11"
            aria-label={label}
          />
        }
      >
        <MoreHorizontalIcon />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {items.map((it) => (
          <DropdownMenuItem
            key={it.label}
            variant={it.destructive ? "destructive" : undefined}
            onClick={it.onClick}
          >
            {it.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function EmptyRow({ colSpan, text }: { colSpan: number; text: string }) {
  return (
    <TableRow>
      <TableCell
        colSpan={colSpan}
        className="text-center text-muted-foreground"
      >
        {text}
      </TableCell>
    </TableRow>
  )
}

function PromotionsTab({
  includeEnded,
  canWrite,
}: {
  includeEnded: boolean
  canWrite: boolean
}) {
  const list = useQuery(promotionsQuery(includeEnded))
  const { name: channelName } = useChannelNames()
  const [form, setForm] = useState<{ id?: string } | null>(null)
  const actions = useDatedActions("promotions", ["pricing", "promotions"])
  return (
    <div className="flex flex-col gap-3">
      {canWrite ? (
        <div>
          <Button onClick={() => setForm({})}>
            <PlusIcon /> Tambah Promo
          </Button>
        </div>
      ) : null}
      <LoadingArea loading={list.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Kode</TableHead>
                <TableHead>Nama</TableHead>
                <TableHead>Channel</TableHead>
                <TableHead>Periode</TableHead>
                <TableHead>Produk</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(list.data ?? []).map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="font-medium whitespace-nowrap">
                    {p.code}
                  </TableCell>
                  <TableCell>{p.name}</TableCell>
                  <TableCell>
                    {p.channelId
                      ? [channelName(p.channelId), channelName(p.subChannelId)]
                          .filter(Boolean)
                          .join(" › ")
                      : "Semua channel"}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {formatPeriod(p.validFrom, p.validTo)}
                  </TableCell>
                  <TableCell>{p.itemCount}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    <PeriodBadge status={p.status} />
                    {!p.isActive ? (
                      <Badge variant="outline" className="ml-2">
                        Nonaktif
                      </Badge>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <RowMenu
                      label={`Aksi untuk ${p.code}`}
                      items={[
                        ...(p.allowedActions.includes("update")
                          ? [
                              {
                                label: "Ubah",
                                onClick: () => setForm({ id: p.id }),
                              },
                            ]
                          : []),
                        ...(p.allowedActions.includes("delete")
                          ? [
                              {
                                label: "Hapus",
                                destructive: true,
                                onClick: () => actions.setDeleting(p),
                              },
                            ]
                          : []),
                      ]}
                    />
                  </TableCell>
                </TableRow>
              ))}
              {list.data?.length === 0 ? (
                <EmptyRow colSpan={7} text="Belum ada promo." />
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <PromotionFormDialog
        open={!!form}
        onOpenChange={(o) => !o && setForm(null)}
        promotionId={form?.id}
      />
      {actions.deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && actions.setDeleting(null)}
          title="Hapus promo?"
          description="Hanya promo yang belum berjalan yang bisa dihapus."
          confirmLabel="Hapus Promo"
          destructive
          loading={actions.remove.isPending}
          onConfirm={() =>
            actions.deleting && actions.remove.mutate(actions.deleting)
          }
        />
      ) : null}
    </div>
  )
}

function TieredTab({
  includeEnded,
  canWrite,
}: {
  includeEnded: boolean
  canWrite: boolean
}) {
  const list = useQuery(tieredPricesQuery(includeEnded))
  const [creating, setCreating] = useState(false)
  const actions = useDatedActions("tiered-prices", ["pricing", "tiered-prices"])
  const today = todayBusinessDate()
  return (
    <div className="flex flex-col gap-3">
      {canWrite ? (
        <div>
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Tambah Harga Bertingkat
          </Button>
        </div>
      ) : null}
      <LoadingArea loading={list.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Produk</TableHead>
                <TableHead>Daftar harga</TableHead>
                <TableHead className="text-right">Qty minimum</TableHead>
                <TableHead className="text-right">Harga satuan</TableHead>
                <TableHead>Periode</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(list.data ?? []).map((t) => (
                <TableRow key={t.id}>
                  <TableCell>
                    <span className="font-medium">{t.productSku}</span>{" "}
                    {t.productName}
                  </TableCell>
                  <TableCell>
                    {t.priceListName ?? "Semua daftar harga"}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatQuantity(t.minQty)}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(t.priceAmount)}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {formatPeriod(t.validFrom, t.validTo)}
                  </TableCell>
                  <TableCell>
                    <PeriodBadge status={t.status} />
                  </TableCell>
                  <TableCell>
                    {canWrite ? (
                      <RowMenu
                        label={`Aksi untuk ${t.productSku}`}
                        items={datedMenu(t, today, actions)}
                      />
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
              {list.data?.length === 0 ? (
                <EmptyRow colSpan={7} text="Belum ada harga bertingkat." />
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <TieredPriceFormDialog open={creating} onOpenChange={setCreating} />
      <DatedDialogs
        actions={actions}
        resource="tiered-prices"
        what="harga bertingkat"
        queryKey={["pricing", "tiered-prices"]}
      />
    </div>
  )
}

export function datedMenu(
  row: DatedRow & { status: string },
  today: string,
  actions: ReturnType<typeof useDatedActions>
) {
  const items: { label: string; onClick: () => void; destructive?: boolean }[] =
    []
  if (row.status !== "expired") {
    items.push({ label: "Akhiri", onClick: () => actions.setEnding(row) })
  }
  if (row.validFrom >= today) {
    items.push({
      label: "Hapus",
      destructive: true,
      onClick: () => actions.setDeleting(row),
    })
  }
  return items
}

/** Dialog Akhiri dan Hapus untuk data effective-dated. */
export function DatedDialogs({
  actions,
  resource,
  what,
  queryKey,
}: {
  actions: ReturnType<typeof useDatedActions>
  resource: DatedResource
  what: string
  queryKey: readonly unknown[]
}) {
  return (
    <>
      {actions.ending ? (
        <EndPeriodDialog
          open
          onOpenChange={(o) => !o && actions.setEnding(null)}
          resource={resource}
          item={actions.ending}
          title={`Akhiri ${what}`}
          queryKey={queryKey}
        />
      ) : null}
      {actions.deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && actions.setDeleting(null)}
          title={`Hapus ${what}?`}
          description="Hanya data yang belum pernah berlaku yang bisa dihapus; data yang sudah berlaku diakhiri saja."
          confirmLabel="Hapus"
          destructive
          loading={actions.remove.isPending}
          onConfirm={() =>
            actions.deleting && actions.remove.mutate(actions.deleting)
          }
        />
      ) : null}
    </>
  )
}

function SpecialTab({
  includeEnded,
  canWrite,
}: {
  includeEnded: boolean
  canWrite: boolean
}) {
  const list = useQuery(specialPricesQuery(includeEnded))
  const [creating, setCreating] = useState(false)
  const actions = useDatedActions("special-prices", [
    "pricing",
    "special-prices",
  ])
  const today = todayBusinessDate()
  return (
    <div className="flex flex-col gap-3">
      {canWrite ? (
        <div>
          <Button onClick={() => setCreating(true)}>
            <PlusIcon /> Tambah Harga Khusus
          </Button>
        </div>
      ) : null}
      <LoadingArea loading={list.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Untuk</TableHead>
                <TableHead>Produk</TableHead>
                <TableHead className="text-right">Harga khusus</TableHead>
                <TableHead>Periode</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {(list.data ?? []).map((s) => (
                <TableRow key={s.id}>
                  <TableCell>
                    <span className="text-muted-foreground">
                      {PARTY_TYPE_LABELS[s.partyType]}:
                    </span>{" "}
                    {s.partyName || s.partyId}
                    {s.partyCode ? (
                      <span className="text-muted-foreground">
                        {" "}
                        · {s.partyCode}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <span className="font-medium">{s.productSku}</span>{" "}
                    {s.productName}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {formatRupiah(s.priceAmount)}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {formatPeriod(s.validFrom, s.validTo)}
                  </TableCell>
                  <TableCell>
                    <PeriodBadge status={s.status} />
                  </TableCell>
                  <TableCell>
                    {canWrite ? (
                      <RowMenu
                        label={`Aksi untuk harga khusus ${s.productSku}`}
                        items={datedMenu(s, today, actions)}
                      />
                    ) : null}
                  </TableCell>
                </TableRow>
              ))}
              {list.data?.length === 0 ? (
                <EmptyRow colSpan={6} text="Belum ada harga khusus." />
              ) : null}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
      <SpecialPriceFormDialog open={creating} onOpenChange={setCreating} />
      <DatedDialogs
        actions={actions}
        resource="special-prices"
        what="harga khusus"
        queryKey={["pricing", "special-prices"]}
      />
    </div>
  )
}
