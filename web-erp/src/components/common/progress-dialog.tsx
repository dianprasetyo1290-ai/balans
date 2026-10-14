import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"

import { CircularProgress } from "./circular-progress"

type ProgressDialogProps = {
  open: boolean
  title: string
  /** Progress 0–100 dari GET /jobs/{jobId}. */
  progress: number
  /** Pesan tahap, mis. "Memvalidasi baris 1.200 dari 5.000". */
  message?: string
  /** Menutup dialog; proses tetap berjalan dan notifikasi in-app muncul saat selesai. */
  onRunInBackground: () => void
}

/** Progress proses panjang via job: import, export, payroll, tutup buku, recost (Technical Guideline 10.7). */
export function ProgressDialog({ open, title, progress, message, onRunInBackground }: ProgressDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onRunInBackground()}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{message ?? "Sedang diproses…"}</DialogDescription>
        </DialogHeader>
        <div className="flex justify-center py-4">
          <CircularProgress value={progress} size="lg" showValue label={title} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onRunInBackground}>
            Jalankan di latar belakang
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
