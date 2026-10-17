import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"

import { PageTabs } from "@/components/common/page-tabs"

import {
  dataExchangeKindsQuery,
  type DataExchangeTab,
} from "../api/dataexchange-api"
import { ExportPanel } from "./export-panel"
import { ImportHistory } from "./import-history"
import { ImportPanel } from "./import-panel"

const route = getRouteApi("/_app/integrations/data-exchange")

/** Integrasi › Import/Export Excel: impor master data dengan preview, ekspor, dan riwayat (S-125, INT-02). */
export function DataExchangePage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const kinds = useQuery(dataExchangeKindsQuery)
  const items = kinds.data?.items ?? []
  const tab: DataExchangeTab = search.tab ?? "import"

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Import/Export Excel
        </h1>
        <p className="text-sm text-muted-foreground">
          Impor dan ekspor data master lewat template Excel dari sistem. Setiap
          baris diperiksa dulu sebelum disimpan; file yang sama tidak bisa
          diimpor dua kali.
        </p>
      </div>
      <PageTabs
        label="Bagian impor/ekspor"
        value={tab}
        onValueChange={(t) =>
          void navigate({ search: (s) => ({ kind: s.kind, tab: t }) })
        }
        tabs={[
          { id: "import", label: "Impor" },
          { id: "export", label: "Ekspor" },
          { id: "history", label: "Riwayat Impor" },
        ]}
      />
      {tab === "import" ? (
        <ImportPanel
          kinds={items.filter((k) => k.canImport)}
          loading={kinds.isPending}
          kind={search.kind}
          jobId={search.job}
          onKindChange={(kind) =>
            void navigate({ search: { tab: "import", kind } })
          }
          onJobChange={(job) =>
            void navigate({
              search: (s) => ({ tab: "import", kind: s.kind, job }),
            })
          }
        />
      ) : null}
      {tab === "export" ? (
        <ExportPanel
          kinds={items.filter((k) => k.canExport)}
          loading={kinds.isPending}
          highlight={search.kind}
        />
      ) : null}
      {tab === "history" ? (
        <ImportHistory
          page={search.page ?? 1}
          onPageChange={(page) =>
            void navigate({ search: { tab: "history", page } })
          }
          onOpen={(job) =>
            void navigate({
              search: { tab: "import", kind: job.kind, job: job.id },
            })
          }
        />
      ) : null}
    </div>
  )
}
