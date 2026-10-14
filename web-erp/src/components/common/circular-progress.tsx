import { cn } from "@/lib/utils"

type CircularProgressProps = {
  /** Persentase 0–100. Kosong berarti indeterminate. */
  value?: number
  size?: "sm" | "md" | "lg"
  label?: string
  showValue?: boolean
  className?: string
}

const SIZES = { sm: 16, md: 32, lg: 56 } as const
const STROKE = { sm: 2.5, md: 3, lg: 4 } as const

/**
 * Satu-satunya indikator loading di aplikasi (Technical Guideline 10.7). DILARANG membuat spinner sendiri.
 */
export function CircularProgress({ value, size = "md", label = "Memuat", showValue = false, className }: CircularProgressProps) {
  const px = SIZES[size]
  const stroke = STROKE[size]
  const radius = (px - stroke) / 2
  const circumference = 2 * Math.PI * radius
  const determinate = value !== undefined
  const clamped = determinate ? Math.min(100, Math.max(0, Math.round(value))) : 0

  return (
    <span
      role="progressbar"
      aria-label={label}
      aria-valuemin={determinate ? 0 : undefined}
      aria-valuemax={determinate ? 100 : undefined}
      aria-valuenow={determinate ? clamped : undefined}
      className={cn("relative inline-flex items-center justify-center text-primary", className)}
      style={{ width: px, height: px }}
    >
      <svg
        width={px}
        height={px}
        viewBox={`0 0 ${px} ${px}`}
        className={cn(!determinate && "animate-spin")}
        aria-hidden="true"
      >
        <circle cx={px / 2} cy={px / 2} r={radius} fill="none" stroke="currentColor" strokeOpacity={0.15} strokeWidth={stroke} />
        <circle
          cx={px / 2}
          cy={px / 2}
          r={radius}
          fill="none"
          stroke="currentColor"
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={determinate ? circumference * (1 - clamped / 100) : circumference * 0.7}
          transform={`rotate(-90 ${px / 2} ${px / 2})`}
          className="transition-[stroke-dashoffset] duration-300"
        />
      </svg>
      {determinate && showValue && size !== "sm" ? (
        <span className="absolute text-[0.65rem] font-medium tabular-nums text-foreground">{clamped}%</span>
      ) : null}
    </span>
  )
}
