import { Badge } from "@/components/ui/badge"

type RecordStatusBadgeProps = {
  isActive: boolean
  deletedAt?: string | null
}

/** Status data master: Aktif, Nonaktif, atau Dihapus (soft delete, S-106). */
export function RecordStatusBadge({
  isActive,
  deletedAt,
}: RecordStatusBadgeProps) {
  if (deletedAt) {
    return (
      <Badge
        variant="outline"
        className="border-destructive/40 text-destructive"
      >
        Dihapus
      </Badge>
    )
  }
  return isActive ? (
    <Badge className="bg-emerald-600/15 text-emerald-700 dark:text-emerald-300">
      Aktif
    </Badge>
  ) : (
    <Badge variant="outline">Nonaktif</Badge>
  )
}
