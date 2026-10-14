import { PaperclipIcon, XIcon } from "lucide-react"
import { useId, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { compressImage } from "@/lib/files/compress-image"

import { CircularProgress } from "./circular-progress"

export type UploadTarget = {
  /** Presigned PUT URL ke object storage. */
  uploadUrl: string
  /** ID file yang dikirim di request bisnis setelah upload. */
  fileId: string
}

type FileUploadProps = {
  label: string
  accept?: string
  maxSizeBytes: number
  /** Meminta presigned URL dari backend untuk file yang akan diunggah. */
  requestUpload: (file: File) => Promise<UploadTarget>
  onUploaded: (fileId: string, file: File) => void
  onRemoved?: () => void
  onError: (message: string) => void
  disabled?: boolean
}

function putWithProgress(url: string, file: File, onProgress: (pct: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open("PUT", url)
    xhr.setRequestHeader("Content-Type", file.type || "application/octet-stream")
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        onProgress((e.loaded / e.total) * 100)
      }
    }
    xhr.onload = () => (xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(`upload ${xhr.status}`)))
    xhr.onerror = () => reject(new TypeError("network"))
    xhr.send(file)
  })
}

/**
 * Upload langsung dari browser ke object storage lewat presigned URL, dengan kompresi foto dan
 * progress persentase (Technical Guideline 5.16, 10.7).
 */
export function FileUpload({ label, accept, maxSizeBytes, requestUpload, onUploaded, onRemoved, onError, disabled }: FileUploadProps) {
  const inputId = useId()
  const inputRef = useRef<HTMLInputElement>(null)
  const [progress, setProgress] = useState<number | null>(null)
  const [fileName, setFileName] = useState<string | null>(null)

  const handleFile = async (picked: File) => {
    try {
      const file = await compressImage(picked)
      if (file.size > maxSizeBytes) {
        onError("Ukuran file tidak diizinkan.")
        return
      }
      setProgress(0)
      const target = await requestUpload(file)
      await putWithProgress(target.uploadUrl, file, setProgress)
      setFileName(file.name)
      onUploaded(target.fileId, file)
    } catch (error) {
      onError(error instanceof TypeError ? "Koneksi terputus. Periksa jaringan Anda lalu coba lagi." : "File gagal diunggah. Coba lagi.")
    } finally {
      setProgress(null)
      if (inputRef.current) {
        inputRef.current.value = ""
      }
    }
  }

  return (
    <div className="flex min-h-11 items-center gap-3">
      <input
        ref={inputRef}
        id={inputId}
        type="file"
        accept={accept}
        className="sr-only"
        disabled={disabled || progress !== null}
        onChange={(e) => {
          const file = e.target.files?.[0]
          if (file) {
            void handleFile(file)
          }
        }}
      />
      {progress !== null ? (
        <CircularProgress value={progress} showValue label="Mengunggah file" />
      ) : fileName ? (
        <span className="flex items-center gap-2 text-sm">
          <PaperclipIcon aria-hidden="true" className="size-4" />
          {fileName}
          <Button
            variant="ghost"
            size="icon"
            aria-label={`Hapus ${fileName}`}
            onClick={() => {
              setFileName(null)
              onRemoved?.()
            }}
          >
            <XIcon aria-hidden="true" />
          </Button>
        </span>
      ) : (
        <Button variant="outline" disabled={disabled} onClick={() => inputRef.current?.click()} className="min-h-11">
          <PaperclipIcon aria-hidden="true" />
          {label}
        </Button>
      )}
    </div>
  )
}
