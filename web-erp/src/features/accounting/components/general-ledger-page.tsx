import { useQuery } from "@tanstack/react-query"
import { getRouteApi, Link } from "@tanstack/react-router"
import { DownloadIcon } from "lucide-react"
import { toast } from "sonner"

import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { usePermissions } from "@/features/auth/api/auth-api"
import { createExportJob } from "@/features/dataexchange/api/dataexchange-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  accountsQuery,
  channelsQuery,
  divisionsQuery,
  ENTRY_TYPE_LABELS,
  generalLedgerQuery,
  type GeneralLedger,
} from "../api/accounting-api"
import { DimensionSelect } from "./dimension-select"
import { SourceDocumentLink } from "./source-document-link"

const route = getRouteApi("/_app/finance/general-ledger")

type LedgerLine = GeneralLedger["items"][number]

/** Keuangan & Akuntansi › Buku Besar: mutasi per akun dengan saldo berjalan (S-127). */
export function GeneralLedgerPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const accounts = useQuery(accountsQuery(true))
  const channels = useQuery(channelsQuery(true))
  const divisions = useQuery(divisionsQuery(true))
  const ledger = useQuery(generalLedgerQuery(search))
  const { can } = usePermissions()
  const exportJob = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const params: Record<string, string> = {
        dateFrom: search.dateFrom,
        dateTo: search.dateTo,
      }
      if (search.accountId) params.accountId = search.accountId
      if (search.divisionId) params.divisionId = search.divisionId
      if (search.channelId) params.channelId = search.channelId
      return createExportJob("accounting.general_ledger", key, params)
    },
    onSuccess: () =>
      toast.success(
        "Ekspor Buku Besar diproses. Tautan unduhan dikirim lewat notifikasi."
      ),
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const postable = (accounts.data ?? []).filter((a) => a.isPostable)
  const setSearch = (patch: Partial<typeof search>) =>
    void navigate({ search: (s) => ({ ...s, ...patch, page: 1 }) })

  const columns: DataTableColumn<LedgerLine>[] = [
    {
      id: "date",
      header: "Tanggal",
      cell: (l) => (
        <span className="whitespace-nowrap">{formatDate(l.businessDate)}</span>
      ),
    },
    {
      id: "number",
      header: "Nomor",
      cell: (l) => (
        <Link
          to="/finance/journals/$id"
          params={{ id: l.entryId }}
          className="whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {l.number}
        </Link>
      ),
    },
    {
      id: "description",
      header: "Keterangan",
      cell: (l) => (
        <div className="flex max-w-md flex-col">
          <span>{l.memo || l.description}</span>
          <span className="text-xs text-muted-foreground">
            {ENTRY_TYPE_LABELS[l.entryType]}
            {l.sourceLabel ? (
              <>
                {" · "}
                <SourceDocumentLink
                  sourceType={l.sourceType}
                  sourceId={l.sourceId}
                  label={l.sourceLabel}
                />
              </>
            ) : null}
            {l.division ? ` · ${l.division.name}` : ""}
            {l.channel ? ` · ${l.channel.name}` : ""}
          </span>
        </div>
      ),
    },
    {
      id: "debit",
      header: "Debit",
      align: "right",
      cell: (l) => (Number(l.debit) ? formatRupiah(l.debit) : ""),
    },
    {
      id: "credit",
      header: "Kredit",
      align: "right",
      cell: (l) => (Number(l.credit) ? formatRupiah(l.credit) : ""),
    },
    {
      id: "balance",
      header: "Saldo",
      align: "right",
      cell: (l) => formatRupiah(l.balance),
    },
  ]

  const accountLabel = (id: string) => {
    const a = postable.find((x) => x.id === id)
    return a ? `${a.code} ${a.name}` : "Pilih akun"
  }
  const data = ledger.data

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Buku Besar</h1>
          <p className="text-sm text-muted-foreground">
            Mutasi per akun dari jurnal yang sudah diposting, dengan saldo
            berjalan sesuai saldo normal akun. Ekspor tanpa akun berisi semua
            akun pada rentang tanggal.
          </p>
        </div>
        {can("dataexchange.transaction.export") ? (
          <LoadingButton
            variant="outline"
            loading={exportJob.isPending}
            loadingText="Memproses…"
            onClick={() => exportJob.mutate()}
          >
            <DownloadIcon /> Ekspor Buku Besar
          </LoadingButton>
        ) : null}
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <Field className="lg:col-span-2">
          <FieldLabel htmlFor="ledger-account">Akun</FieldLabel>
          <Select
            value={search.accountId ?? ""}
            onValueChange={(v) =>
              setSearch({ accountId: v ? String(v) : undefined })
            }
          >
            <SelectTrigger id="ledger-account" className="w-full">
              <SelectValue>
                {(v: string) => (v ? accountLabel(v) : "Pilih akun")}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {postable.map((a) => (
                <SelectItem key={a.id} value={a.id}>
                  {a.code} {a.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="ledger-from">Dari tanggal</FieldLabel>
          <DateInput
            id="ledger-from"
            value={search.dateFrom}
            onValueChange={(v) => setSearch({ dateFrom: v })}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="ledger-to">Sampai tanggal</FieldLabel>
          <DateInput
            id="ledger-to"
            value={search.dateTo}
            onValueChange={(v) => setSearch({ dateTo: v })}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="ledger-division">Divisi</FieldLabel>
          <DimensionSelect
            id="ledger-division"
            value={search.divisionId}
            options={divisions.data ?? []}
            placeholder="Semua divisi"
            onValueChange={(v) => setSearch({ divisionId: v ?? undefined })}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="ledger-channel">Channel</FieldLabel>
          <DimensionSelect
            id="ledger-channel"
            value={search.channelId}
            options={channels.data ?? []}
            placeholder="Semua channel"
            onValueChange={(v) => setSearch({ channelId: v ?? undefined })}
          />
        </Field>
      </div>
      {!search.accountId ? (
        <p className="text-sm text-muted-foreground">
          Pilih akun untuk melihat Buku Besar.
        </p>
      ) : (
        <>
          {data ? (
            <dl className="grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-4">
              <Summary label="Saldo awal" value={data.openingBalance} />
              <Summary label="Total debit" value={data.periodDebit} />
              <Summary label="Total kredit" value={data.periodCredit} />
              <Summary label="Saldo akhir" value={data.closingBalance} strong />
            </dl>
          ) : null}
          <DataTable
            columns={columns}
            rows={data?.items}
            getRowId={(l) => `${l.entryId}-${l.lineNo}`}
            loading={ledger.isPending}
            fetching={ledger.isFetching}
            page={search.page}
            totalPages={data?.totalPages ?? 0}
            totalItems={data?.totalItems ?? 0}
            onSortChange={() => undefined}
            onPageChange={(page) =>
              void navigate({ search: (s) => ({ ...s, page }) })
            }
            emptyMessage="Tidak ada mutasi pada rentang tanggal ini."
          />
        </>
      )}
    </div>
  )
}

function Summary({
  label,
  value,
  strong = false,
}: {
  label: string
  value: string
  strong?: boolean
}) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={strong ? "text-lg font-semibold" : "text-lg"}>
        {formatRupiah(value)}
      </dd>
    </div>
  )
}
