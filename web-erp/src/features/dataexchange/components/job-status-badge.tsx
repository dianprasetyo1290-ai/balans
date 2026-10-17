import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

import {
  EXPORT_STATUS_LABELS,
  IMPORT_STATUS_LABELS,
  type ExportStatus,
  type ImportStatus,
} from "../api/dataexchange-api"

const TONES: Record<string, string> = {
  neutral: "bg-muted text-muted-foreground",
  info: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  warning: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  success: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  danger: "bg-destructive/10 text-destructive",
}

const TONE_BY_STATUS: Record<ImportStatus | ExportStatus, keyof typeof TONES> =
  {
    uploaded: "info",
    validating: "info",
    committing: "info",
    queued: "info",
    running: "info",
    validated: "warning",
    completed: "success",
    failed: "danger",
    cancelled: "neutral",
  }

/** Badge status job impor/ekspor. */
export function JobStatusBadge({
  status,
  kind,
}: {
  status: ImportStatus | ExportStatus
  kind: "import" | "export"
}) {
  const label =
    kind === "import"
      ? IMPORT_STATUS_LABELS[status as ImportStatus]
      : EXPORT_STATUS_LABELS[status as ExportStatus]
  return (
    <Badge
      variant="outline"
      className={cn("border-transparent", TONES[TONE_BY_STATUS[status]])}
      data-status={status}
    >
      {label ?? status}
    </Badge>
  )
}
