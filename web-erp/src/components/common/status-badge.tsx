import { Badge } from "@/components/ui/badge"
import { statusLabel, type StatusTone } from "@/lib/format/status-labels"
import { cn } from "@/lib/utils"

const TONE_CLASSES: Record<StatusTone, string> = {
  neutral: "bg-muted text-muted-foreground",
  info: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
  warning: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  success: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  danger: "bg-destructive/10 text-destructive",
}

/** Badge status dokumen dengan label dari Naming Convention bagian 7 dan warna konsisten. */
export function StatusBadge({ status, className }: { status: string; className?: string }) {
  const { label, tone } = statusLabel(status)
  return (
    <Badge variant="outline" className={cn("border-transparent", TONE_CLASSES[tone], className)} data-status={status}>
      {label}
    </Badge>
  )
}
