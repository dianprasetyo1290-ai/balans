import type { ReactNode } from "react"

import { useDelayedVisibility } from "@/hooks/use-delayed-visibility"

import { CircularProgress } from "./circular-progress"

type LoadingAreaProps = {
  loading: boolean
  label?: string
  children?: ReactNode
}

/**
 * Circular progress di tengah area konten yang dimuat, bukan menutup seluruh layar; header dan
 * sidebar tetap bisa dipakai (Technical Guideline 10.7).
 */
export function LoadingArea({ loading, label = "Memuat data", children }: LoadingAreaProps) {
  const visible = useDelayedVisibility(loading)
  if (loading) {
    return (
      <div className="flex min-h-40 items-center justify-center" aria-busy="true">
        {visible ? <CircularProgress label={label} /> : null}
      </div>
    )
  }
  return <>{children}</>
}
