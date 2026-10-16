import { useNavigate } from "@tanstack/react-router"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { ACTION_LABELS, errorMessageFor } from "@/lib/api/error-messages"
import { useErrorDialogStore } from "@/stores/error-dialog-store"

/**
 * Modal error standar untuk error bisnis, otentikasi, jaringan, dan server (Technical Guideline 10.6).
 * Dipasang sekali di root; dibuka lewat showErrorDialog().
 */
export function ErrorDialog() {
  const { problem, onRetry, close } = useErrorDialogStore()
  const navigate = useNavigate()

  if (!problem) {
    return null
  }
  const spec = errorMessageFor(problem.code)
  const action = spec.action === "retry" && !onRetry ? "close" : spec.action

  const handleAction = () => {
    close()
    if (action === "retry") {
      onRetry?.()
    } else if (action === "reload") {
      window.location.reload()
    } else if (action === "relogin") {
      void navigate({ to: "/login" })
    }
  }

  return (
    <AlertDialog open onOpenChange={(open) => !open && close()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{spec.title}</AlertDialogTitle>
          <AlertDialogDescription>{problem.detail || spec.message}</AlertDialogDescription>
          {problem.status >= 500 && problem.requestId ? (
            <p className="text-xs text-muted-foreground">
              Kode laporan: <span className="font-mono select-all">{problem.requestId}</span>
            </p>
          ) : null}
        </AlertDialogHeader>
        <AlertDialogFooter>
          {action !== "close" ? (
            <AlertDialogAction variant="outline" onClick={close}>
              {ACTION_LABELS.close}
            </AlertDialogAction>
          ) : null}
          <AlertDialogAction onClick={handleAction}>
            {ACTION_LABELS[action]}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
