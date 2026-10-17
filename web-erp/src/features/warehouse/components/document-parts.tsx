import { useQuery } from "@tanstack/react-query"
import type { ReactNode } from "react"

import { ApprovalSteps } from "@/features/approval/components/approval-steps"
import { api, unwrap } from "@/lib/api/client"

/** Satu pasangan label–nilai di kepala dokumen. */
export function Info({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{value || "—"}</dd>
    </div>
  )
}

/** Panel Persetujuan dokumen: langkah approval terakhir untuk subjek ini (Desain UI/UX 3.3). */
export function ApprovalPanel({
  subjectType,
  subjectId,
}: {
  subjectType: string
  subjectId: string
}) {
  const requests = useQuery({
    queryKey: ["approval", "subject", subjectType, subjectId],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/approval-requests", {
          params: { query: { subjectType, subjectId } },
        })
      ),
  })
  const latest = requests.data?.items[0]
  if (!latest) {
    return null
  }
  return (
    <section
      className="flex flex-col gap-3 rounded-lg border p-4"
      aria-label="Persetujuan"
    >
      <h2 className="font-medium">Persetujuan</h2>
      <ApprovalSteps steps={latest.steps} />
    </section>
  )
}
