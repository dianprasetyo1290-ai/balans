import { useEffect, useRef, useState } from "react"

/** Indikator muncul setelah 300 ms dan tampil minimal 500 ms (Technical Guideline 10.7). */
export const LOADING_SHOW_DELAY_MS = 300
export const LOADING_MIN_VISIBLE_MS = 500

/**
 * Mengubah status `loading` menjadi status tampil indikator tanpa kedipan: request cepat tidak
 * menampilkan indikator, dan indikator yang sudah tampil tidak hilang terlalu cepat.
 */
export function useDelayedVisibility(loading: boolean): boolean {
  const [visible, setVisible] = useState(false)
  const shownAt = useRef<number | null>(null)

  useEffect(() => {
    if (loading) {
      const timer = setTimeout(() => {
        shownAt.current = Date.now()
        setVisible(true)
      }, LOADING_SHOW_DELAY_MS)
      return () => clearTimeout(timer)
    }
    if (shownAt.current === null) {
      return undefined
    }
    const remaining = Math.max(0, LOADING_MIN_VISIBLE_MS - (Date.now() - shownAt.current))
    const timer = setTimeout(() => {
      shownAt.current = null
      setVisible(false)
    }, remaining)
    return () => clearTimeout(timer)
  }, [loading])

  return visible
}
