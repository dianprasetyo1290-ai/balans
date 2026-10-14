import { create } from "zustand"

import type { Problem } from "@/lib/api/problem"

type ErrorDialogState = {
  problem: Problem | null
  onRetry: (() => void) | null
  show: (problem: Problem, onRetry?: () => void) => void
  close: () => void
}

/** State UI global untuk ErrorDialog (Technical Guideline 10.6). Bukan tempat menyimpan data server. */
export const useErrorDialogStore = create<ErrorDialogState>((set) => ({
  problem: null,
  onRetry: null,
  show: (problem, onRetry) => set({ problem, onRetry: onRetry ?? null }),
  close: () => set({ problem: null, onRetry: null }),
}))

export function showErrorDialog(problem: Problem, onRetry?: () => void) {
  useErrorDialogStore.getState().show(problem, onRetry)
}
