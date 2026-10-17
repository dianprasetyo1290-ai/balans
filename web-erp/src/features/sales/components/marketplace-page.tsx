import { useQuery } from "@tanstack/react-query"
import { FileSpreadsheetIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { usePermissions } from "@/features/auth/api/auth-api"
import { dataExchangeKindsQuery } from "@/features/dataexchange/api/dataexchange-api"
import { ImportPanel } from "@/features/dataexchange/components/import-panel"

import type { OrderListSearch } from "../api/sales-documents-api"
import { OrderListPage } from "./order-list-page"

/** Jenis data impor penjualan marketplace (dataexchange). */
export const MARKETPLACE_IMPORT_KIND = "sales.marketplace_order"

type Props = {
  search: OrderListSearch & { job?: string }
  onSearchChange: (
    update: (
      s: OrderListSearch & { job?: string }
    ) => OrderListSearch & { job?: string }
  ) => void
}

/**
 * Penjualan › Penjualan Marketplace: daftar order marketplace dan impor Excel per marketplace (S-307). Satu baris
 * Excel = satu order; nomor order yang sudah tercatat ditolak di pemeriksaan impor.
 */
export function MarketplacePage({ search, onSearchChange }: Props) {
  const { can } = usePermissions()
  const canImport = can("sales.marketplace_order.import")
  const [importing, setImporting] = useState(!!search.job)
  const kinds = useQuery({ ...dataExchangeKindsQuery, enabled: canImport })
  const items = (kinds.data?.items ?? []).filter(
    (k) => k.kind === MARKETPLACE_IMPORT_KIND && k.canImport
  )
  return (
    <OrderListPage
      kind="marketplace"
      search={search}
      onSearchChange={onSearchChange}
    >
      {canImport ? (
        <section className="flex flex-col gap-3 rounded-lg border p-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 className="text-sm font-semibold">
                Impor Excel per Marketplace
              </h2>
              <p className="text-sm text-muted-foreground">
                Kolom Item: <code>SKU:qty:harga</code> dipisah titik koma; harga
                kosong memakai daftar harga. Baris diperiksa dulu sebelum
                disimpan.
              </p>
            </div>
            <Button variant="outline" onClick={() => setImporting((v) => !v)}>
              <FileSpreadsheetIcon />
              {importing ? "Tutup Impor" : "Impor Penjualan Marketplace"}
            </Button>
          </div>
          {importing ? (
            <ImportPanel
              kinds={items}
              loading={kinds.isPending}
              kind={MARKETPLACE_IMPORT_KIND}
              jobId={search.job}
              onKindChange={() => undefined}
              onJobChange={(job) => onSearchChange((s) => ({ ...s, job }))}
            />
          ) : null}
        </section>
      ) : null}
    </OrderListPage>
  )
}
