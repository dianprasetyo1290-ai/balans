import { useEffect, useRef, useState } from "react"

import { LoadingButton } from "@/components/common/loading-button"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { setReauthHandler } from "@/lib/api/auth-fetch"
import { parseProblem } from "@/lib/api/problem"

import { reauthenticate } from "../api/auth-api"

/**
 * Konfirmasi identitas untuk aksi sensitif (Technical Guideline 9.1). Dibuka otomatis oleh authFetch
 * saat API menjawab AUTH_REAUTH_REQUIRED; setelah berhasil, request yang sama diulang.
 */
export function ReauthDialog() {
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const resolver = useRef<((ok: boolean) => void) | null>(null)

  useEffect(() => {
    setReauthHandler(
      () =>
        new Promise<boolean>((resolve) => {
          resolver.current = resolve
          setPassword("")
          setError(null)
          setOpen(true)
        })
    )
    return () => setReauthHandler(null)
  }, [])

  const finish = (ok: boolean) => {
    setOpen(false)
    resolver.current?.(ok)
    resolver.current = null
  }

  const submit = async () => {
    if (!password) {
      setError("Masukkan password atau kode autentikasi")
      return
    }
    setSubmitting(true)
    try {
      const isCode = /^\d{6}$/.test(password)
      await reauthenticate(isCode ? { totpCode: password } : { password })
      finish(true)
    } catch (err) {
      const problem = parseProblem(err)
      setError(
        problem.code === "AUTH_MFA_INVALID"
          ? "Kode autentikasi salah."
          : problem.detail
      )
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && finish(false)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Konfirmasi identitas</DialogTitle>
          <DialogDescription>
            Tindakan ini sensitif. Masukkan password atau kode autentikator Anda
            untuk melanjutkan.
          </DialogDescription>
        </DialogHeader>
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <Field data-invalid={!!error}>
            <FieldLabel htmlFor="reauth-secret">
              Password atau kode 6 angka
            </FieldLabel>
            <Input
              id="reauth-secret"
              type="password"
              autoComplete="current-password"

              value={password}
              onChange={(e) => setPassword(e.target.value)}
              aria-invalid={!!error}
            />
            <FieldError>{error}</FieldError>
          </Field>
          <DialogFooter className="mt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => finish(false)}
            >
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={submitting}
              loadingText="Memeriksa…"
            >
              Konfirmasi
            </LoadingButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
