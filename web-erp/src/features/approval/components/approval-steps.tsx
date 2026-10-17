import {
  CheckCircle2Icon,
  CircleDashedIcon,
  CircleIcon,
  ClockIcon,
  XCircleIcon,
} from "lucide-react"

import { formatDateTime } from "@/lib/format/date"
import { cn } from "@/lib/utils"

import type { ApprovalStep } from "../api/approval-api"

const STEP_STATUS: Record<
  ApprovalStep["status"],
  { label: string; icon: typeof CircleIcon; className: string }
> = {
  approved: {
    label: "Disetujui",
    icon: CheckCircle2Icon,
    className: "text-emerald-600 dark:text-emerald-400",
  },
  rejected: {
    label: "Ditolak",
    icon: XCircleIcon,
    className: "text-destructive",
  },
  pending: {
    label: "Menunggu Persetujuan",
    icon: ClockIcon,
    className: "text-amber-600 dark:text-amber-400",
  },
  waiting: {
    label: "Belum aktif",
    icon: CircleIcon,
    className: "text-muted-foreground",
  },
  skipped: {
    label: "Tidak diproses",
    icon: CircleDashedIcon,
    className: "text-muted-foreground",
  },
}

/** Approver yang tertulis di langkah: orang yang ditunjuk atau jabatan. */
export function stepApprover(step: ApprovalStep): string {
  return step.approverUserName
    ? `${step.approverUserName} (${step.approverRoleName})`
    : step.approverRoleName
}

/** Panel Persetujuan: langkah per level berurutan (Desain UI/UX 3.3). */
export function ApprovalSteps({ steps }: { steps: ApprovalStep[] }) {
  return (
    <ol className="flex flex-col gap-3" aria-label="Langkah persetujuan">
      {steps.map((step) => {
        const spec = STEP_STATUS[step.status]
        const Icon = spec.icon
        return (
          <li key={step.level} className="flex gap-3">
            <Icon
              className={cn("mt-0.5 size-5 shrink-0", spec.className)}
              aria-hidden="true"
            />
            <div className="flex min-w-0 flex-col gap-0.5 text-sm">
              <span className="font-medium">
                Level {step.level} · {stepApprover(step)}
              </span>
              <span className={cn("text-xs", spec.className)}>
                {spec.label}
                {step.decidedByName ? ` oleh ${step.decidedByName}` : ""}
                {step.delegatedFromName
                  ? ` atas nama ${step.delegatedFromName}`
                  : ""}
                {step.decidedAt ? ` · ${formatDateTime(step.decidedAt)}` : ""}
              </span>
              {step.note ? (
                <span className="text-xs break-words text-muted-foreground">
                  “{step.note}”
                </span>
              ) : null}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
