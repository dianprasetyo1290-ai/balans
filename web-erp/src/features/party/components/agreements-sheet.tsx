import { useQuery, useQueryClient } from "@tanstack/react-query"
import { FileTextIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { StatusBadge } from "@/components/common/status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDate } from "@/lib/format/date"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  AGREEMENT_STATUS_LABELS,
  AGREEMENT_TYPE_LABELS,
  BORNE_BY_LABELS,
  deleteAgreement,
  DISCREPANCY_RULES,
  openFile,
  partnerAgreementsQuery,
  type Partner,
  type PartnerAgreement,
} from "../api/party-api"
import { AgreementFormDialog } from "./agreement-form-dialog"

const STATUS_TONE: Record<PartnerAgreement["status"], string> = {
  scheduled: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  active: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  expired: "",
}

export function AgreementStatusBadge({
  status,
}: {
  status: PartnerAgreement["status"]
}) {
  if (status === "expired") {
    return <StatusBadge status="expired" />
  }
  return (
    <Badge
      variant="outline"
      className={`border-transparent ${STATUS_TONE[status]}`}
    >
      {AGREEMENT_STATUS_LABELS[status]}
    </Badge>
  )
}

type AgreementsSheetProps = {
  partner: Partner | null
  onOpenChange: (open: boolean) => void
}

/** Riwayat PKS mitra beserta PDF, masa berlaku, termin, dan ketentuan selisih (MD-07). */
export function AgreementsSheet({
  partner,
  onOpenChange,
}: AgreementsSheetProps) {
  return (
    <Sheet open={!!partner} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        {partner ? <AgreementsContent partner={partner} /> : null}
      </SheetContent>
    </Sheet>
  )
}

function AgreementsContent({ partner }: { partner: Partner }) {
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("party.partner_agreement.write")
  const agreements = useQuery(partnerAgreementsQuery(partner.id))
  const [form, setForm] = useState<{ agreement?: PartnerAgreement } | null>(
    null
  )
  const [deleting, setDeleting] = useState<PartnerAgreement | null>(null)

  const remove = useIdempotentMutation({
    mutationFn: (a: PartnerAgreement, key) =>
      deleteAgreement(a.id, a.version, key),
    onSuccess: async () => {
      toast.success("PKS dihapus")
      setDeleting(null)
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ["party", "agreements", partner.id],
        }),
        queryClient.invalidateQueries({ queryKey: ["party", "partners"] }),
      ])
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const open = (fileId: string) =>
    void openFile(fileId).catch((err) => showErrorDialog(parseProblem(err)))

  return (
    <>
      <SheetHeader>
        <SheetTitle>PKS {partner.name}</SheetTitle>
        <SheetDescription>
          {partner.code} · PKS kedaluwarsa memblokir DO konsinyasi.
        </SheetDescription>
      </SheetHeader>
      <div className="flex flex-col gap-4 px-4 pb-6">
        {canWrite ? (
          <Button
            className="min-h-11 self-start sm:min-h-9"
            onClick={() => setForm({})}
          >
            <PlusIcon /> Catat PKS
          </Button>
        ) : null}
        <LoadingArea loading={agreements.isPending}>
          {agreements.data?.length === 0 ? (
            <p className="text-sm text-muted-foreground">Belum ada PKS.</p>
          ) : null}
          <ul className="flex flex-col gap-3">
            {agreements.data?.map((a) => (
              <li key={a.id} className="rounded-lg border p-3 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{a.agreementNumber}</span>
                    <Badge variant="secondary">
                      {AGREEMENT_TYPE_LABELS[a.agreementType]}
                    </Badge>
                    <AgreementStatusBadge status={a.status} />
                  </div>
                  {canWrite ? (
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => setForm({ agreement: a })}
                      >
                        Ubah
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-destructive"
                        onClick={() => setDeleting(a)}
                      >
                        Hapus
                      </Button>
                    </div>
                  ) : null}
                </div>
                <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
                  <dt className="text-muted-foreground">Masa berlaku</dt>
                  <dd>
                    {formatDate(a.validFrom)} – {formatDate(a.validTo)}
                  </dd>
                  <dt className="text-muted-foreground">Termin</dt>
                  <dd>{a.paymentTermDays} hari</dd>
                  {a.consignmentMarginPercent ? (
                    <>
                      <dt className="text-muted-foreground">Margin mitra</dt>
                      <dd>{Number(a.consignmentMarginPercent)}%</dd>
                    </>
                  ) : null}
                  <dt className="text-muted-foreground">Selisih</dt>
                  <dd>
                    {DISCREPANCY_RULES.map(
                      (r) => `${r.label}: ${BORNE_BY_LABELS[a[r.field]]}`
                    ).join(" · ")}
                  </dd>
                  <dt className="text-muted-foreground">Dokumen</dt>
                  <dd>
                    {a.fileId ? (
                      <Button
                        variant="link"
                        className="h-auto p-0"
                        onClick={() => a.fileId && open(a.fileId)}
                      >
                        <FileTextIcon /> {a.fileName ?? "Lihat PDF"}
                      </Button>
                    ) : (
                      <span className="text-amber-700 dark:text-amber-300">
                        PDF belum diunggah
                      </span>
                    )}
                  </dd>
                </dl>
              </li>
            ))}
          </ul>
        </LoadingArea>
      </div>
      <AgreementFormDialog
        open={form !== null}
        onOpenChange={(o) => !o && setForm(null)}
        partner={partner}
        agreement={form?.agreement}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setDeleting(null)}
          title="Hapus PKS?"
          description={`${deleting.agreementNumber}. Hapus hanya untuk PKS yang salah input; perubahan ketentuan dicatat sebagai PKS baru.`}
          confirmLabel="Hapus PKS"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </>
  )
}
