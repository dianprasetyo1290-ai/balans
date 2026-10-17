import { useQueryClient } from "@tanstack/react-query"
import { FileSpreadsheetIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { FileUpload } from "@/components/common/file-upload"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createImportJob,
  downloadTemplate,
  requestImportUpload,
  XLSX_TYPE,
  type DataExchangeKind,
} from "../api/dataexchange-api"
import { ImportJobView } from "./import-job-view"
import { ImportSteps } from "./import-steps"

type ImportPanelProps = {
  kinds: DataExchangeKind[]
  loading: boolean
  kind?: string
  jobId?: string
  onKindChange: (kind: string) => void
  onJobChange: (job: string | undefined) => void
}

/** Wizard impor: 1 Unduh template → 2 Unggah file → 3 Periksa → 4 Simpan (Desain UI/UX 3.6). */
export function ImportPanel({
  kinds,
  loading,
  kind,
  jobId,
  onKindChange,
  onJobChange,
}: ImportPanelProps) {
  const queryClient = useQueryClient()
  const selected = kinds.find((k) => k.kind === kind)
  const [downloading, setDownloading] = useState(false)

  const create = useIdempotentMutation({
    mutationFn: (fileId: string, key) =>
      createImportJob(selected?.kind ?? "", fileId, key),
    onSuccess: async (job) => {
      await queryClient.invalidateQueries({
        queryKey: ["dataexchange", "import-jobs", "list"],
      })
      onJobChange(job.id)
    },
    onError: (err) => showErrorDialog(parseProblem(err)),
  })

  if (jobId) {
    return (
      <ImportJobView
        id={jobId}
        kind={selected}
        onReset={() => onJobChange(undefined)}
      />
    )
  }

  return (
    <LoadingArea loading={loading}>
      {kinds.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          Anda belum memiliki akses impor data.
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          <ImportSteps current={selected ? 1 : 0} />
          <Select
            value={selected?.kind ?? ""}
            onValueChange={(v) => onKindChange(v as string)}
          >
            <SelectTrigger
              className="w-full sm:w-72"
              aria-label="Jenis data yang diimpor"
            >
              <SelectValue>
                {(v: string) =>
                  kinds.find((k) => k.kind === v)?.label ?? "Pilih jenis data"
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {kinds.map((k) => (
                <SelectItem key={k.kind} value={k.kind}>
                  {k.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {selected ? (
            <div className="grid gap-4 md:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>1. Unduh template</CardTitle>
                  <CardDescription>
                    Isi data mulai baris 2. Kolom bertanda * wajib diisi;
                    petunjuk dan contoh ada di sheet Petunjuk.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <LoadingButton
                    variant="outline"
                    className="min-h-11 sm:min-h-9"
                    loading={downloading}
                    loadingText="Mengunduh…"
                    onClick={() => {
                      setDownloading(true)
                      downloadTemplate(selected)
                        .catch(() =>
                          toast.error("Template gagal diunduh. Coba lagi.")
                        )
                        .finally(() => setDownloading(false))
                    }}
                  >
                    <FileSpreadsheetIcon /> Unduh Template {selected.label}
                    .xlsx
                  </LoadingButton>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>2. Unggah file</CardTitle>
                  <CardDescription>
                    File .xlsx maksimal{" "}
                    {Math.round((selected.maxFileBytes ?? 0) / (1 << 20))} MB.
                    Pemeriksaan berjalan di latar belakang.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <FileUpload
                    key={selected.kind}
                    label="Pilih File Excel"
                    accept={`.xlsx,${XLSX_TYPE}`}
                    maxSizeBytes={selected.maxFileBytes ?? 10 << 20}
                    disabled={create.isPending}
                    requestUpload={(file) =>
                      requestImportUpload(selected, file)
                    }
                    onUploaded={(fileId) => create.mutate(fileId)}
                    onError={(message) => toast.error(message)}
                  />
                </CardContent>
              </Card>
            </div>
          ) : null}
        </div>
      )}
    </LoadingArea>
  )
}
