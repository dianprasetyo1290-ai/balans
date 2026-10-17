import { useQueryClient } from "@tanstack/react-query"
import { FileTextIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { FileUpload } from "@/components/common/file-upload"
import { LoadingButton } from "@/components/common/loading-button"
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
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  AGREEMENT_TYPE_LABELS,
  BORNE_BY_LABELS,
  createAgreement,
  DISCREPANCY_RULES,
  MAX_AGREEMENT_FILE_BYTES,
  openFile,
  requestAgreementUpload,
  updateAgreement,
  type AgreementType,
  type BorneBy,
  type Partner,
  type PartnerAgreement,
  type PartnerAgreementInput,
} from "../api/party-api"
import {
  ChoiceField,
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "./form-parts"

type AgreementFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  partner: Partner
  /** Kosong = PKS baru. */
  agreement?: PartnerAgreement
}

type Rules = Record<(typeof DISCREPANCY_RULES)[number]["field"], BorneBy>

/**
 * Catat atau koreksi PKS mitra (MD-07). Perubahan ketentuan di tengah masa berlaku dicatat sebagai PKS baru
 * setelah tanggal berakhir PKS lama dipersingkat (Technical Guideline 5.14).
 */
export function AgreementFormDialog(props: AgreementFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <AgreementForm {...props} /> : null}
    </Dialog>
  )
}

function AgreementForm({
  onOpenChange,
  partner,
  agreement,
}: AgreementFormDialogProps) {
  const queryClient = useQueryClient()
  const [number, setNumber] = useState(agreement?.agreementNumber ?? "")
  const [type, setType] = useState<AgreementType>(
    agreement?.agreementType ??
      (partner.partnerType === "modern_trade" ? "modern_trade" : "consignment")
  )
  const [validFrom, setValidFrom] = useState(agreement?.validFrom ?? "")
  const [validTo, setValidTo] = useState(agreement?.validTo ?? "")
  const [fileId, setFileId] = useState(agreement?.fileId ?? null)
  const [margin, setMargin] = useState(
    agreement?.consignmentMarginPercent
      ? String(Number(agreement.consignmentMarginPercent))
      : ""
  )
  const [term, setTerm] = useState(String(agreement?.paymentTermDays ?? 30))
  const [rules, setRules] = useState<Rules>(() => {
    const out = {} as Rules
    for (const r of DISCREPANCY_RULES) {
      out[r.field] = agreement?.[r.field] ?? r.fallback
    }
    return out
  })
  const [notes, setNotes] = useState(agreement?.notes ?? "")
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (body: PartnerAgreementInput, key) =>
      agreement
        ? updateAgreement(
            agreement.id,
            { ...body, version: agreement.version },
            key
          )
        : createAgreement(partner.id, body, key),
    onSuccess: async () => {
      toast.success(agreement ? "PKS diperbarui" : "PKS dicatat")
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ["party", "agreements", partner.id],
        }),
        queryClient.invalidateQueries({ queryKey: ["party", "partners"] }),
      ])
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const submit = () => {
    const next: FieldErrors = {}
    if (!number.trim()) next.agreementNumber = "Nomor PKS wajib diisi"
    if (!validFrom) next.validFrom = "Tanggal mulai wajib diisi"
    if (!validTo) next.validTo = "Tanggal berakhir wajib diisi"
    if (validFrom && validTo && validTo < validFrom)
      next.validTo = "Tanggal berakhir tidak boleh sebelum tanggal mulai"
    const days = Number(term)
    if (!Number.isInteger(days) || days < 0 || days > 365)
      next.paymentTermDays = "Termin pembayaran 0 sampai 365 hari"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate({
      agreementNumber: number,
      agreementType: type,
      validFrom,
      validTo,
      fileId,
      consignmentMarginPercent: margin ? margin.replace(",", ".") : null,
      paymentTermDays: days,
      ...rules,
      notes,
    })
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
      <DialogHeader>
        <DialogTitle>
          {agreement
            ? `Ubah PKS ${agreement.agreementNumber}`
            : `Catat PKS ${partner.name}`}
        </DialogTitle>
        <DialogDescription>
          DO konsinyasi hanya bisa dibuat selama ada PKS konsinyasi yang
          berlaku.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.agreementNumber}>
              <FieldLabel htmlFor="agreement-number">Nomor PKS</FieldLabel>
              <Input
                id="agreement-number"
                value={number}
                aria-invalid={!!errors.agreementNumber}
                onChange={(e) => setNumber(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "agreementNumber")} />
            </Field>
            <ChoiceField
              id="agreement-type"
              label="Jenis PKS"
              value={type}
              onValueChange={setType}
              labels={AGREEMENT_TYPE_LABELS}
              error={errors.agreementType}
            />
            <Field data-invalid={!!errors.validFrom}>
              <FieldLabel htmlFor="agreement-from">Berlaku mulai</FieldLabel>
              <DateInput
                id="agreement-from"
                value={validFrom}
                onValueChange={setValidFrom}
                aria-invalid={!!errors.validFrom}
              />
              <FieldError errors={fieldError(errors, "validFrom")} />
            </Field>
            <Field data-invalid={!!errors.validTo}>
              <FieldLabel htmlFor="agreement-to">Berlaku sampai</FieldLabel>
              <DateInput
                id="agreement-to"
                value={validTo}
                onValueChange={setValidTo}
                aria-invalid={!!errors.validTo}
              />
              <FieldError errors={fieldError(errors, "validTo")} />
            </Field>
            <Field data-invalid={!!errors.consignmentMarginPercent}>
              <FieldLabel htmlFor="agreement-margin">
                Margin mitra (%)
              </FieldLabel>
              <Input
                id="agreement-margin"
                inputMode="decimal"
                placeholder="mis. 25"
                value={margin}
                aria-invalid={!!errors.consignmentMarginPercent}
                onChange={(e) => setMargin(e.target.value)}
              />
              <FieldDescription>
                Harga konsinyasi per SKU diatur di Daftar Harga mitra.
              </FieldDescription>
              <FieldError
                errors={fieldError(errors, "consignmentMarginPercent")}
              />
            </Field>
            <Field data-invalid={!!errors.paymentTermDays}>
              <FieldLabel htmlFor="agreement-term">
                Termin pembayaran (hari)
              </FieldLabel>
              <Input
                id="agreement-term"
                type="number"
                inputMode="numeric"
                min={0}
                max={365}
                value={term}
                aria-invalid={!!errors.paymentTermDays}
                onChange={(e) => setTerm(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "paymentTermDays")} />
            </Field>
          </div>
          <FieldSet>
            <FieldLegend variant="label">
              Ketentuan selisih rekonsiliasi
            </FieldLegend>
            <div className="grid gap-4 sm:grid-cols-2">
              {DISCREPANCY_RULES.map((r) => (
                <ChoiceField
                  key={r.field}
                  id={`agreement-${r.field}`}
                  label={`${r.label} ditanggung`}
                  value={rules[r.field]}
                  onValueChange={(v) =>
                    setRules((cur) => ({ ...cur, [r.field]: v }))
                  }
                  labels={BORNE_BY_LABELS}
                  error={errors[r.field]}
                />
              ))}
            </div>
          </FieldSet>
          <Field data-invalid={!!errors.fileId}>
            <FieldLabel>Dokumen PKS (PDF)</FieldLabel>
            {fileId && fileId === agreement?.fileId ? (
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="link"
                  className="h-auto px-0"
                  onClick={() =>
                    void openFile(fileId).catch((err) =>
                      showErrorDialog(parseProblem(err))
                    )
                  }
                >
                  <FileTextIcon /> {agreement.fileName ?? "Lihat PDF"}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setFileId(null)}
                >
                  Ganti PDF
                </Button>
              </div>
            ) : (
              <FileUpload
                label="Unggah PDF PKS"
                accept="application/pdf"
                maxSizeBytes={MAX_AGREEMENT_FILE_BYTES}
                requestUpload={requestAgreementUpload}
                onUploaded={(id) => {
                  setFileId(id)
                  setErrors((e) => {
                    const next = { ...e }
                    delete next.fileId
                    return next
                  })
                }}
                onRemoved={() => setFileId(null)}
                onError={(message) =>
                  setErrors((e) => ({ ...e, fileId: message }))
                }
              />
            )}
            <FieldDescription>Maksimal 10 MB.</FieldDescription>
            <FieldError errors={fieldError(errors, "fileId")} />
          </Field>
          <Field data-invalid={!!errors.notes}>
            <FieldLabel htmlFor="agreement-notes">Catatan</FieldLabel>
            <Textarea
              id="agreement-notes"
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "notes")} />
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
            loadingText="Menyimpan…"
          >
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
