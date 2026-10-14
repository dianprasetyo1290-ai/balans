import type { ComponentProps } from "react"

import { Button } from "@/components/ui/button"
import { useDelayedVisibility } from "@/hooks/use-delayed-visibility"

import { CircularProgress } from "./circular-progress"

type LoadingButtonProps = ComponentProps<typeof Button> & {
  loading: boolean
  /** Label saat proses berjalan, mis. "Menyimpan…". */
  loadingText?: string
}

/**
 * Tombol submit/aksi. Saat loading tombol langsung di-disable agar tidak terjadi double-submit,
 * sedangkan indikator mengikuti aturan tunda 300 ms (Technical Guideline 10.7).
 */
export function LoadingButton({ loading, loadingText, children, disabled, ...props }: LoadingButtonProps) {
  const showIndicator = useDelayedVisibility(loading)

  return (
    <Button {...props} disabled={disabled || loading} aria-busy={loading}>
      {showIndicator ? <CircularProgress size="sm" label={loadingText ?? "Memproses"} className="text-current" /> : null}
      {loading && loadingText ? loadingText : children}
    </Button>
  )
}
