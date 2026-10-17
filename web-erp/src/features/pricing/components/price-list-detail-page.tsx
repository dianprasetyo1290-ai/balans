import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { ArrowLeftIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
import { StatusBadge } from "@/components/common/status-badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  BASIS_LABELS,
  PRICE_LIST_TYPE_LABELS,
  deletePriceList,
  formatPercent,
  priceListQuery,
  restorePriceList,
} from "../api/pricing-api"
import { PriceListFormDialog } from "./price-list-form-dialog"
import { scopeLabel } from "./price-lists-page"
import { formatPeriod, useChannelNames } from "./pricing-parts"
import { VersionDialog } from "./version-dialog"

const FIELD_LABELS: Record<string, string> = {
  code: "Kode",
  name: "Nama",
  priceListType: "Jenis",
  channelId: "Channel",
  subChannelId: "Sub-channel",
  partnerId: "Mitra",
  basis: "Basis harga",
  referencePriceListId: "Daftar harga acuan",
  description: "Keterangan",
  isActive: "Aktif",
  deletedAt: "Dihapus pada",
}

type Tab = "versions" | "history"

type PriceListDetailPageProps = {
  id: string
  tab: Tab
  onTabChange: (tab: Tab) => void
}

/** Detail daftar harga: riwayat versi (draft → diajukan → berlaku) dan riwayat perubahan (S-116). */
export function PriceListDetailPage({
  id,
  tab,
  onTabChange,
}: PriceListDetailPageProps) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const list = useQuery(priceListQuery(id))
  const { name: channelName } = useChannelNames()
  const [editing, setEditing] = useState(false)
  const [version, setVersion] = useState<{ id?: string } | null>(null)
  const [confirm, setConfirm] = useState<"delete" | "restore" | null>(null)
  const l = list.data

  const act = useIdempotentMutation({
    mutationFn: async (action: "delete" | "restore", key) => {
      if (!l) return
      if (action === "delete") await deletePriceList(l.id, l.version, key)
      else await restorePriceList(l.id, l.version, key)
    },
    onSuccess: async (_, action) => {
      toast.success(
        action === "delete" ? "Daftar harga dihapus" : "Daftar harga dipulihkan"
      )
      setConfirm(null)
      await queryClient.invalidateQueries({ queryKey: ["pricing"] })
      if (action === "delete") void navigate({ to: "/master/price-lists" })
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  if (list.isError) {
    return (
      <p className="text-sm text-destructive">
        Daftar harga tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }

  return (
    <LoadingArea loading={list.isPending}>
      {l ? (
        <div className="flex flex-col gap-4">
          <div>
            <Button
              variant="ghost"
              size="sm"
              className="-ml-2"
              nativeButton={false}
              render={<Link to="/master/price-lists" />}
            >
              <ArrowLeftIcon /> Daftar Harga
            </Button>
          </div>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {l.name}
                </h1>
                <RecordStatusBadge
                  isActive={l.isActive}
                  deletedAt={l.deletedAt}
                />
              </div>
              <p className="text-sm text-muted-foreground">
                {l.code} · {PRICE_LIST_TYPE_LABELS[l.priceListType]} ·{" "}
                {scopeLabel(l, channelName)}
              </p>
              <p className="text-sm text-muted-foreground">
                {BASIS_LABELS[l.basis]}
                {l.basis === "percent"
                  ? ` (${l.referencePriceListName ?? "acuan"})`
                  : ""}
                {l.description ? ` · ${l.description}` : ""}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {l.allowedActions.includes("update") ? (
                <Button variant="outline" onClick={() => setEditing(true)}>
                  Ubah Daftar Harga
                </Button>
              ) : null}
              {l.allowedActions.includes("delete") ? (
                <Button variant="outline" onClick={() => setConfirm("delete")}>
                  Hapus
                </Button>
              ) : null}
              {l.allowedActions.includes("restore") ? (
                <Button variant="outline" onClick={() => setConfirm("restore")}>
                  Pulihkan
                </Button>
              ) : null}
              {l.allowedActions.includes("create_version") ? (
                <Button onClick={() => setVersion({})}>
                  <PlusIcon /> Buat Versi Baru
                </Button>
              ) : null}
            </div>
          </div>
          {l.deletedAt ? (
            <p className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm">
              Dihapus {formatDateTime(l.deletedAt)}. Daftar harga ini tidak
              dipakai transaksi baru.
            </p>
          ) : null}
          <PageTabs
            label="Bagian daftar harga"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "versions", label: "Versi Harga" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "versions" ? (
            <div className="overflow-x-auto rounded-lg border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Versi</TableHead>
                    <TableHead>Periode berlaku</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Isi</TableHead>
                    <TableHead>Diajukan</TableHead>
                    <TableHead>Diputuskan</TableHead>
                    <TableHead>Catatan</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {l.versions.map((v) => (
                    <TableRow key={v.id}>
                      <TableCell className="font-medium whitespace-nowrap">
                        <button
                          type="button"
                          className="min-h-11 underline-offset-4 hover:underline"
                          onClick={() => setVersion({ id: v.id })}
                        >
                          v{v.versionNo}
                        </button>
                        {l.currentVersion?.id === v.id ? (
                          <span className="ml-2 text-xs text-emerald-700 dark:text-emerald-300">
                            berlaku hari ini
                          </span>
                        ) : null}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        {formatPeriod(v.validFrom, v.validTo)}
                      </TableCell>
                      <TableCell>
                        <StatusBadge
                          status={v.status === "active" ? "approved" : v.status}
                        />
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        {l.basis === "percent"
                          ? formatPercent(v.percent)
                          : `${v.itemCount} produk`}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        {v.submittedAt ? formatDateTime(v.submittedAt) : "—"}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        {v.decidedAt ? formatDateTime(v.decidedAt) : "—"}
                      </TableCell>
                      <TableCell className="max-w-64 truncate">
                        {v.notes || "—"}
                      </TableCell>
                    </TableRow>
                  ))}
                  {l.versions.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        className="text-center text-muted-foreground"
                      >
                        Belum ada versi harga. Buat versi baru lalu ajukan untuk
                        persetujuan.
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </div>
          ) : (
            <div className="flex flex-col gap-6">
              <EntityHistory
                entityType="price_list"
                entityId={l.id}
                fieldLabels={FIELD_LABELS}
              />
            </div>
          )}
          <PriceListFormDialog
            open={editing}
            onOpenChange={setEditing}
            priceList={l}
          />
          <VersionDialog
            open={!!version}
            onOpenChange={(o) => !o && setVersion(null)}
            priceList={l}
            versionId={version?.id}
          />
          {confirm ? (
            <ConfirmDialog
              open
              onOpenChange={(o) => !o && setConfirm(null)}
              title={
                confirm === "delete"
                  ? `Hapus daftar harga ${l.code}?`
                  : `Pulihkan daftar harga ${l.code}?`
              }
              description={
                confirm === "delete"
                  ? "Daftar harga yang dihapus tidak dipakai transaksi baru. Transaksi lama tetap menyimpan harga dan versinya; kode tidak bisa dipakai ulang."
                  : "Daftar harga kembali dipakai resolver harga sesuai versi yang berlaku."
              }
              confirmLabel={
                confirm === "delete"
                  ? "Hapus Daftar Harga"
                  : "Pulihkan Daftar Harga"
              }
              destructive={confirm === "delete"}
              loading={act.isPending}
              onConfirm={() => act.mutate(confirm)}
            />
          ) : null}
        </div>
      ) : null}
    </LoadingArea>
  )
}
