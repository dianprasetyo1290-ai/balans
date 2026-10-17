import { Badge } from "@/components/ui/badge"

import { ASSET_STATUS_LABELS, type FixedAssetStatus } from "../api/close-api"

const TONE: Record<FixedAssetStatus, string> = {
  draft: "bg-muted text-muted-foreground",
  active: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  disposed: "bg-muted text-muted-foreground",
}

/** Status aset tetap: Draft, Aktif, Dilepas (status khusus register aset). */
export function AssetStatusBadge({ status }: { status: FixedAssetStatus }) {
  return (
    <Badge
      variant="outline"
      className={`border-transparent ${TONE[status]}`}
      data-status={status}
    >
      {ASSET_STATUS_LABELS[status]}
    </Badge>
  )
}
