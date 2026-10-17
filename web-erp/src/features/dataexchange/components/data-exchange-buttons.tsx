import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"
import { DownloadIcon, UploadIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { usePermissions } from "@/features/auth/api/auth-api"
import { systemInfoQuery } from "@/features/system/api/system-queries"

import { exportPermission, importPermission } from "../api/dataexchange-api"

/**
 * Tombol Impor/Ekspor di halaman master yang membuka Integrasi › Import/Export Excel dengan jenis data
 * terpilih (S-125). Tampil jika modul dataexchange aktif dan user punya `<kind>.import`/`<kind>.export`.
 */
export function DataExchangeButtons({
  kind,
  resource = kind,
}: {
  kind: string
  /** Resource permission bila berbeda dari kind, mis. `dataexchange.transaction` untuk data transaksi. */
  resource?: string
}) {
  const { can } = usePermissions()
  const info = useQuery(systemInfoQuery)
  if (!info.data?.enabledModules.includes("dataexchange")) {
    return null
  }
  const canImport = can(importPermission(resource))
  const canExport = can(exportPermission(resource))
  return (
    <>
      {canImport ? (
        <Button
          variant="outline"
          className="min-h-11 sm:min-h-9"
          nativeButton={false}
          render={
            <Link
              to="/integrations/data-exchange"
              search={{ tab: "import", kind }}
            />
          }
        >
          <UploadIcon /> Impor
        </Button>
      ) : null}
      {canExport ? (
        <Button
          variant="outline"
          className="min-h-11 sm:min-h-9"
          nativeButton={false}
          render={
            <Link
              to="/integrations/data-exchange"
              search={{ tab: "export", kind }}
            />
          }
        >
          <DownloadIcon /> Ekspor
        </Button>
      ) : null}
    </>
  )
}
