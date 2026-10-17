import { Link } from "@tanstack/react-router"
import { ExternalLinkIcon } from "lucide-react"

import { StatusBadge } from "@/components/common/status-badge"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"

import type { ApprovalRequest } from "../api/approval-api"
import { ApprovalSteps } from "./approval-steps"
import type { Decision } from "./decision-dialog"

type ApprovalDetailSheetProps = {
  request: ApprovalRequest | null
  delegatedFromName?: string | null
  onOpenChange: (open: boolean) => void
  onDecide?: (request: ApprovalRequest, decision: Decision) => void
}

/** Ringkasan dokumen dan langkah persetujuan; dibuka dari inbox dan riwayat. */
export function ApprovalDetailSheet({
  request,
  delegatedFromName,
  onOpenChange,
  onDecide,
}: ApprovalDetailSheetProps) {
  const actions = request?.allowedActions ?? []
  return (
    <Sheet open={!!request} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-md">
        {request ? (
          <>
            <SheetHeader>
              <SheetTitle className="flex flex-wrap items-center gap-2">
                {request.subjectLabel}
                <StatusBadge status={request.status} />
              </SheetTitle>
              <SheetDescription>{request.subjectTypeLabel}</SheetDescription>
            </SheetHeader>
            <div className="flex flex-col gap-6 px-4">
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                <dt className="text-muted-foreground">Diajukan oleh</dt>
                <dd>{request.requestedByName}</dd>
                <dt className="text-muted-foreground">Tanggal</dt>
                <dd>{formatDateTime(request.createdAt)}</dd>
                {Number(request.amount) > 0 ? (
                  <>
                    <dt className="text-muted-foreground">Nilai</dt>
                    <dd className="tabular-nums">
                      {formatRupiah(request.amount)}
                    </dd>
                  </>
                ) : null}
                {request.summary ? (
                  <>
                    <dt className="text-muted-foreground">Keterangan</dt>
                    <dd className="break-words">{request.summary}</dd>
                  </>
                ) : null}
                {delegatedFromName ? (
                  <>
                    <dt className="text-muted-foreground">Delegasi</dt>
                    <dd>Atas nama {delegatedFromName}</dd>
                  </>
                ) : null}
              </dl>
              <section aria-labelledby="approval-steps-title">
                <h3
                  id="approval-steps-title"
                  className="mb-3 text-sm font-semibold"
                >
                  Persetujuan
                </h3>
                <ApprovalSteps steps={request.steps} />
              </section>
              {request.subjectLink ? (
                <Button
                  variant="outline"
                  className="self-start"
                  render={<Link to={request.subjectLink} />}
                >
                  <ExternalLinkIcon aria-hidden="true" /> Buka dokumen
                </Button>
              ) : null}
            </div>
            {actions.length > 0 && onDecide ? (
              <SheetFooter className="flex-row justify-end">
                {actions.includes("reject") ? (
                  <Button
                    variant="outline"
                    onClick={() => onDecide(request, "reject")}
                  >
                    Tolak
                  </Button>
                ) : null}
                {actions.includes("approve") ? (
                  <Button onClick={() => onDecide(request, "approve")}>
                    Setujui
                  </Button>
                ) : null}
              </SheetFooter>
            ) : null}
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
