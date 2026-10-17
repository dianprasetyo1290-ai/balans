import { Link } from "@tanstack/react-router"

const linkClass = "text-primary underline-offset-4 hover:underline"

/**
 * Tautan drill-down ke dokumen sumber jurnal (S-410). Jenis dokumen yang belum punya halaman detail ditampilkan
 * sebagai teks.
 */
export function SourceDocumentLink({
  sourceType,
  sourceId,
  label,
}: {
  sourceType: string
  sourceId: string
  label: string
}) {
  const text = label || sourceType
  switch (sourceType) {
    case "fixed_asset":
    case "fixed_asset_disposal":
      return (
        <Link
          to="/finance/fixed-assets/$id"
          params={{ id: sourceId }}
          search={{ tab: "detail" }}
          className={linkClass}
        >
          {text}
        </Link>
      )
    case "fixed_asset_depreciation":
      return (
        <Link
          to="/finance/depreciation"
          search={{ page: 1 }}
          className={linkClass}
        >
          {text}
        </Link>
      )
    case "opening_balance":
      return (
        <Link
          to="/finance/opening-balances"
          search={{ tab: "balances", page: 1 }}
          className={linkClass}
        >
          {text}
        </Link>
      )
    case "journal_reversal":
    case "manual_journal":
      return (
        <Link
          to="/finance/journals/$id"
          params={{ id: sourceId }}
          className={linkClass}
        >
          {text}
        </Link>
      )
    case "purchase_advance":
      return (
        <Link
          to="/procurement/purchase-advances/$id"
          params={{ id: sourceId }}
          className={linkClass}
        >
          {text}
        </Link>
      )
    case "stock_count":
      return (
        <Link
          to="/warehouse/stock-counts/$id"
          params={{ id: sourceId }}
          className={linkClass}
        >
          {text}
        </Link>
      )
    case "stock_transfer":
      return (
        <Link
          to="/warehouse/stock-transfers/$id"
          params={{ id: sourceId }}
          className={linkClass}
        >
          {text}
        </Link>
      )
  }
  return <span>{text}</span>
}
