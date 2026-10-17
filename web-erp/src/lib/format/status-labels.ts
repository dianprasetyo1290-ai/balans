// Kode status API (snake_case) → label UI sesuai Naming Convention bagian 7 (Technical Guideline 10.8).

export type StatusTone = "neutral" | "info" | "warning" | "success" | "danger"

type StatusSpec = { label: string; tone: StatusTone }

export const STATUS_LABELS = {
  draft: { label: "Draft", tone: "neutral" },
  pending_approval: { label: "Menunggu Persetujuan", tone: "warning" },
  approved: { label: "Disetujui", tone: "success" },
  rejected: { label: "Ditolak", tone: "danger" },
  in_progress: { label: "Diproses", tone: "info" },
  awaiting_payment: { label: "Menunggu Pembayaran", tone: "warning" },
  partially_paid: { label: "Dibayar Sebagian", tone: "warning" },
  paid: { label: "Lunas", tone: "success" },
  posted: { label: "Diposting", tone: "success" },
  cancelled: { label: "Dibatalkan", tone: "neutral" },
  completed: { label: "Selesai", tone: "success" },
  closed: { label: "Ditutup", tone: "neutral" },
  open: { label: "Terbuka", tone: "success" },
  expired: { label: "Kedaluwarsa", tone: "danger" },
  in_transit: { label: "Dalam Pengiriman", tone: "info" },
  partially_received: { label: "Diterima Sebagian", tone: "warning" },
  received: { label: "Diterima", tone: "success" },
  quarantine: { label: "Karantina", tone: "warning" },
  qc_rejected: { label: "Ditolak QC", tone: "danger" },
  submitted: { label: "Diajukan", tone: "info" },
} as const satisfies Record<string, StatusSpec>

export type StatusCode = keyof typeof STATUS_LABELS

/** Label UI untuk kode status. Kode yang belum terdaftar ditampilkan apa adanya agar mudah dilaporkan. */
export function statusLabel(code: string): StatusSpec {
  if (Object.hasOwn(STATUS_LABELS, code)) {
    return STATUS_LABELS[code as StatusCode]
  }
  return { label: code, tone: "neutral" }
}
