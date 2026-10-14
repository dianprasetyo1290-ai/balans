import { useQuery } from "@tanstack/react-query"
import { CheckCircle2Icon, XCircleIcon } from "lucide-react"

import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { formatDateTime } from "@/lib/format/date"

import { readinessQuery, systemInfoQuery } from "../api/system-queries"

const ENVIRONMENT_LABELS: Record<string, string> = {
  local: "Lokal",
  staging: "Staging",
  production: "Production",
}

const CHECK_LABELS: Record<string, string> = {
  database: "Database",
  migration: "Migrasi skema",
}

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-1 gap-1 py-2 sm:grid-cols-3">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="text-sm sm:col-span-2">{children}</dd>
    </div>
  )
}

/**
 * Halaman Tentang Sistem: walking skeleton ERP → Caddy → api → Postgres (F0-TEK-06).
 * Menampilkan versi aplikasi, lingkungan, modul aktif, dan status /readyz.
 */
export function AboutSystemPage() {
  const info = useQuery(systemInfoQuery)
  const readiness = useQuery(readinessQuery)

  return (
    <div className="flex max-w-3xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Tentang Sistem</h1>
        <p className="text-sm text-muted-foreground">Versi aplikasi dan status layanan.</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Aplikasi</CardTitle>
          <CardDescription>Informasi build yang sedang berjalan.</CardDescription>
        </CardHeader>
        <CardContent>
          <LoadingArea loading={info.isPending} label="Memuat informasi sistem">
            {info.data ? (
              <dl className="divide-y">
                <Row label="Versi">
                  <span className="font-mono">{info.data.version}</span>
                </Row>
                <Row label="Commit">
                  <span className="font-mono text-xs">{info.data.commit}</span>
                </Row>
                <Row label="Lingkungan">
                  {ENVIRONMENT_LABELS[info.data.environment] ?? info.data.environment}
                  {info.data.mockActive ? (
                    <Badge variant="outline" className="ml-2">
                      Mode simulasi pihak ketiga
                    </Badge>
                  ) : null}
                </Row>
                <Row label="Modul aktif">
                  {info.data.enabledModules.length > 0 ? (
                    <span className="flex flex-wrap gap-1">
                      {info.data.enabledModules.map((m) => (
                        <Badge key={m} variant="secondary">
                          {m}
                        </Badge>
                      ))}
                    </span>
                  ) : (
                    <span className="text-muted-foreground">Belum ada modul bisnis yang dirilis</span>
                  )}
                </Row>
                <Row label="Versi skema database">
                  <span className="font-mono">{info.data.database.schemaVersion}</span>
                </Row>
                <Row label="Waktu server">{formatDateTime(info.data.serverTime)} WIB</Row>
              </dl>
            ) : null}
          </LoadingArea>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            Status layanan
            {readiness.data ? (
              <Badge variant={readiness.data.status === "ready" ? "secondary" : "destructive"} data-testid="readiness">
                {readiness.data.status === "ready" ? "Siap" : "Belum siap"}
              </Badge>
            ) : null}
          </CardTitle>
          <CardDescription>Hasil pemeriksaan /readyz, diperbarui setiap 30 detik.</CardDescription>
        </CardHeader>
        <CardContent>
          <LoadingArea loading={readiness.isPending} label="Memeriksa layanan">
            {readiness.data ? (
              <ul className="flex flex-col gap-2">
                {readiness.data.checks.map((check) => (
                  <li key={check.name} className="flex items-center gap-2 text-sm">
                    {check.status === "ok" ? (
                      <CheckCircle2Icon className="size-4 text-emerald-600" aria-hidden="true" />
                    ) : (
                      <XCircleIcon className="size-4 text-destructive" aria-hidden="true" />
                    )}
                    <span>{CHECK_LABELS[check.name] ?? check.name}</span>
                    <span className="text-muted-foreground">{check.status === "ok" ? "Normal" : (check.detail ?? "Gagal")}</span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-destructive">Layanan tidak dapat dihubungi.</p>
            )}
          </LoadingArea>
        </CardContent>
      </Card>
    </div>
  )
}
