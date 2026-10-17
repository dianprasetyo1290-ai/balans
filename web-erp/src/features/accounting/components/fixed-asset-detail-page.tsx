import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi, Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { PageTabs } from "@/components/common/page-tabs"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate, todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { accountsQuery } from "../api/accounting-api"
import {
  activateFixedAsset,
  ASSET_SOURCE_LABELS,
  deleteFixedAsset,
  disposeFixedAsset,
  fixedAssetQuery,
  type FixedAsset,
  type FixedAssetDetail,
} from "../api/close-api"
import { AccountSelect } from "./account-select"
import { AssetStatusBadge } from "./asset-status-badge"
import { FixedAssetFormDialog } from "./fixed-asset-form-dialog"

const route = getRouteApi("/_app/finance/fixed-assets/$id")

/** Detail aset tetap: nilai, riwayat penyusutan (A-31), aktifkan, lepas, dan Riwayat. */
export function FixedAssetDetailPage({ id }: { id: string }) {
  const detail = useQuery(fixedAssetQuery(id))
  return (
    <LoadingArea loading={detail.isPending}>
      {detail.data ? <AssetDetail detail={detail.data} /> : null}
    </LoadingArea>
  )
}

function AssetDetail({ detail }: { detail: FixedAssetDetail }) {
  const a = detail.asset
  const search = route.useSearch()
  const routeNavigate = route.useNavigate()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [confirm, setConfirm] = useState<"activate" | "delete" | null>(null)
  const [editing, setEditing] = useState(false)
  const [disposing, setDisposing] = useState(false)
  const can = (x: FixedAsset["allowedActions"][number]) =>
    a.allowedActions.includes(x)

  const action = useIdempotentMutation({
    mutationFn: (kind: "activate" | "delete", key): Promise<unknown> =>
      kind === "activate"
        ? activateFixedAsset(a.id, a.version, key)
        : deleteFixedAsset(a.id, a.version, key),
    onSuccess: async (_, kind) => {
      setConfirm(null)
      await queryClient.invalidateQueries({ queryKey: ["accounting"] })
      if (kind === "delete") {
        toast.success("Aset dihapus")
        void navigate({
          to: "/finance/fixed-assets",
          search: { tab: "assets", page: 1 },
        })
      } else {
        toast.success("Aset diaktifkan")
      }
    },
    onError: (err) => {
      setConfirm(null)
      showErrorDialog(parseProblem(err))
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-sm text-muted-foreground">
            <Link
              to="/finance/fixed-assets"
              search={{ tab: "assets", page: 1 }}
              className="underline-offset-4 hover:underline"
            >
              Aset Tetap
            </Link>
          </p>
          <h1 className="flex flex-wrap items-center gap-2 text-2xl font-semibold tracking-tight">
            {a.code} · {a.name} <AssetStatusBadge status={a.status} />
          </h1>
        </div>
        <div className="flex flex-wrap gap-2">
          {can("edit") ? (
            <Button variant="outline" onClick={() => setEditing(true)}>
              Ubah
            </Button>
          ) : null}
          {can("delete") ? (
            <Button variant="destructive" onClick={() => setConfirm("delete")}>
              Hapus
            </Button>
          ) : null}
          {can("dispose") ? (
            <Button variant="outline" onClick={() => setDisposing(true)}>
              Lepas Aset
            </Button>
          ) : null}
          {can("activate") ? (
            <Button onClick={() => setConfirm("activate")}>
              Aktifkan Aset
            </Button>
          ) : null}
        </div>
      </div>
      <PageTabs
        label="Detail aset"
        tabs={[
          { id: "detail", label: "Detail" },
          { id: "history", label: "Riwayat" },
        ]}
        value={search.tab}
        onValueChange={(tab) => void routeNavigate({ search: { tab } })}
      />
      {search.tab === "history" ? (
        <EntityHistory entityType="fixed_asset" entityId={a.id} />
      ) : (
        <>
          <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <Info label="Kelas" value={a.className} />
            <Info label="Asal" value={ASSET_SOURCE_LABELS[a.source]} />
            <Info label="Lokasi / Mitra" value={a.locationLabel || "—"} />
            <Info
              label="Tanggal Perolehan"
              value={formatDate(a.acquisitionDate)}
            />
            <Info label="Harga Perolehan" value={formatRupiah(a.cost)} />
            <Info label="Nilai Sisa" value={formatRupiah(a.salvage)} />
            <Info label="Umur Manfaat" value={`${a.usefulLifeMonths} bulan`} />
            <Info
              label="Mulai Disusutkan"
              value={formatDate(a.depreciationStart)}
            />
            <Info
              label="Penyusutan per Bulan"
              value={formatRupiah(a.monthlyDepreciation)}
            />
            <Info
              label="Akumulasi Penyusutan"
              value={formatRupiah(a.accumulated)}
            />
            <Info label="Nilai Buku" value={formatRupiah(a.bookValue)} />
            {a.source === "opening" ? (
              <Info
                label="Akumulasi Saldo Awal"
                value={`${formatRupiah(a.openingAccumulated)} (${a.openingDepreciatedMonths} bulan)`}
              />
            ) : null}
            {a.acquisitionJournalId ? (
              <div>
                <dt className="text-muted-foreground">Jurnal Perolehan</dt>
                <dd>
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: a.acquisitionJournalId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                </dd>
              </div>
            ) : null}
            {a.disposalDate ? (
              <>
                <Info
                  label="Tanggal Pelepasan"
                  value={formatDate(a.disposalDate)}
                />
                <Info
                  label="Hasil Pelepasan"
                  value={formatRupiah(a.disposalProceeds ?? "0")}
                />
                <Info label="Alasan Pelepasan" value={a.disposalReason} />
              </>
            ) : null}
            {a.disposalJournalId ? (
              <div>
                <dt className="text-muted-foreground">Jurnal Pelepasan</dt>
                <dd>
                  <Link
                    to="/finance/journals/$id"
                    params={{ id: a.disposalJournalId }}
                    className="text-primary underline-offset-4 hover:underline"
                  >
                    Lihat jurnal
                  </Link>
                </dd>
              </div>
            ) : null}
            {a.note ? <Info label="Catatan" value={a.note} /> : null}
          </dl>
          <h2 className="text-lg font-semibold">Penyusutan</h2>
          <div className="overflow-x-auto rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Periode</TableHead>
                  <TableHead className="text-right">Penyusutan</TableHead>
                  <TableHead className="text-right">Akumulasi</TableHead>
                  <TableHead className="text-right">Nilai Buku</TableHead>
                  <TableHead>Jurnal</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {detail.depreciation.map((l) => (
                  <TableRow key={l.id}>
                    <TableCell>{l.periodLabel}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.amount)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.accumulated)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {formatRupiah(l.bookValue)}
                    </TableCell>
                    <TableCell>
                      <Link
                        to="/finance/journals/$id"
                        params={{ id: l.journalEntryId }}
                        className="text-primary underline-offset-4 hover:underline"
                      >
                        {l.journalNumber}
                      </Link>
                    </TableCell>
                  </TableRow>
                ))}
                {detail.depreciation.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      className="text-center text-muted-foreground"
                    >
                      Belum ada penyusutan.
                    </TableCell>
                  </TableRow>
                ) : null}
              </TableBody>
            </Table>
          </div>
        </>
      )}
      <ConfirmDialog
        open={confirm === "activate"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Aktifkan aset?"
        description={
          a.source === "purchase"
            ? `Jurnal A-31 perolehan ${formatRupiah(a.cost)} dibuat pada ${formatDate(a.acquisitionDate)} dan aset mulai disusutkan.`
            : "Aset saldo awal diaktifkan tanpa jurnal perolehan (nilainya masuk lewat Saldo Awal) dan mulai disusutkan bulan setelah cut-over."
        }
        confirmLabel="Aktifkan Aset"
        loading={action.isPending}
        onConfirm={() => action.mutate("activate")}
      />
      <ConfirmDialog
        open={confirm === "delete"}
        onOpenChange={(o) => !o && setConfirm(null)}
        title="Hapus aset Draft?"
        description="Aset Draft dihapus. Kode aset tidak bisa dipakai ulang."
        confirmLabel="Hapus"
        destructive
        loading={action.isPending}
        onConfirm={() => action.mutate("delete")}
      />
      <FixedAssetFormDialog
        open={editing}
        onOpenChange={setEditing}
        value={a}
      />
      <DisposeDialog asset={a} open={disposing} onOpenChange={setDisposing} />
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}

type DisposeProps = {
  asset: FixedAsset
  open: boolean
  onOpenChange: (open: boolean) => void
}

function DisposeDialog(props: DisposeProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <DisposeForm {...props} /> : null}
    </Dialog>
  )
}

function DisposeForm({ asset, onOpenChange }: DisposeProps) {
  const queryClient = useQueryClient()
  const accounts = useQuery(accountsQuery(false))
  const bankAccounts = (accounts.data ?? []).filter(
    (x) => x.isPostable && x.code.startsWith("11")
  )
  const [date, setDate] = useState(todayBusinessDate())
  const [proceeds, setProceeds] = useState("0")
  const [accountId, setAccountId] = useState("")
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<Record<string, string>>({})
  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      disposeFixedAsset(
        asset.id,
        {
          version: asset.version,
          disposalDate: date,
          proceeds: proceeds || "0",
          disposalAccountId: accountId || null,
          reason,
        },
        key
      ),
    onSuccess: async () => {
      toast.success("Aset dilepas")
      await queryClient.invalidateQueries({ queryKey: ["accounting"] })
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields)
        return
      }
      showErrorDialog(problem)
    },
  })
  const err = (k: string) => (errors[k] ? [{ message: errors[k] }] : [])
  return (
    <DialogContent className="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>Lepas Aset {asset.code}</DialogTitle>
        <DialogDescription>
          Nilai buku saat ini {formatRupiah(asset.bookValue)}. Jurnal A-31
          pelepasan membalik harga perolehan dan akumulasi; selisih dengan hasil
          pelepasan dicatat sebagai laba (7101) atau rugi (7201).
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          save.mutate()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.disposalDate}>
            <FieldLabel htmlFor="dispose-date">Tanggal Pelepasan</FieldLabel>
            <DateInput id="dispose-date" value={date} onValueChange={setDate} />
            <FieldError errors={err("disposalDate")} />
          </Field>
          <Field data-invalid={!!errors.proceeds}>
            <FieldLabel htmlFor="dispose-proceeds">Hasil Pelepasan</FieldLabel>
            <MoneyInput
              id="dispose-proceeds"
              value={proceeds}
              onValueChange={setProceeds}
            />
            <FieldError errors={err("proceeds")} />
          </Field>
          {Number(proceeds) > 0 ? (
            <Field data-invalid={!!errors.disposalAccountId}>
              <FieldLabel htmlFor="dispose-account">Akun Penerimaan</FieldLabel>
              <AccountSelect
                id="dispose-account"
                value={accountId}
                onValueChange={setAccountId}
                accounts={bankAccounts}
              />
              <FieldError errors={err("disposalAccountId")} />
            </Field>
          ) : null}
          <Field data-invalid={!!errors.reason}>
            <FieldLabel htmlFor="dispose-reason">Alasan</FieldLabel>
            <Textarea
              id="dispose-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <FieldError errors={err("reason")} />
          </Field>
        </FieldGroup>
        <DialogFooter className="mt-6">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Memproses…"
          >
            Lepas Aset
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
