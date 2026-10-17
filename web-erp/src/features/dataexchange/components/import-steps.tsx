import { CheckIcon } from "lucide-react"

import { cn } from "@/lib/utils"

const STEPS = ["Unduh template", "Unggah file", "Periksa", "Simpan"]

/** Penanda langkah wizard impor; `current` = indeks langkah aktif (0–3), 4 = semua selesai. */
export function ImportSteps({ current }: { current: number }) {
  return (
    <ol
      className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm"
      aria-label="Langkah impor"
    >
      {STEPS.map((label, i) => {
        const done = i < current
        const active = i === current
        return (
          <li
            key={label}
            className="flex items-center gap-2"
            aria-current={active ? "step" : undefined}
          >
            <span
              className={cn(
                "flex size-6 items-center justify-center rounded-full border text-xs font-medium",
                done && "border-primary bg-primary text-primary-foreground",
                active && "border-primary text-primary",
                !done && !active && "text-muted-foreground"
              )}
            >
              {done ? (
                <CheckIcon aria-hidden="true" className="size-3.5" />
              ) : (
                i + 1
              )}
            </span>
            <span
              className={cn(
                !active && !done && "text-muted-foreground",
                active && "font-medium"
              )}
            >
              {label}
            </span>
            {i < STEPS.length - 1 ? (
              <span aria-hidden="true" className="text-muted-foreground">
                →
              </span>
            ) : null}
          </li>
        )
      })}
    </ol>
  )
}
