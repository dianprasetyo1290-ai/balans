import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { useState } from "react"

import { LoadingButton } from "@/components/common/loading-button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { api, unwrap } from "@/lib/api/client"
import { errorMessageFor } from "@/lib/api/error-messages"
import { parseProblem } from "@/lib/api/problem"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { logout, meQuery } from "../api/auth-api"
import { AuthLayout } from "./auth-layout"

type Setup = { secret: string; qrCodeDataUrl: string }

/** Aktivasi MFA wajib untuk Admin Sistem, Head F&A, Direksi, dan HRGA (S-104). */
export function MfaSetupPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [setup, setSetup] = useState<Setup | null>(null)
  const [code, setCode] = useState("")
  const [codeError, setCodeError] = useState<string | null>(null)

  const start = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/api/v1/auth/mfa/setup", {
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        })
      ),
    onSuccess: (data) => setSetup(data),
    onError: (err) => showErrorDialog(parseProblem(err)),
  })
  const activate = useMutation({
    mutationFn: () =>
      unwrap(
        api.POST("/api/v1/auth/mfa/activate", {
          params: { header: { "Idempotency-Key": crypto.randomUUID() } },
          body: { code },
        })
      ),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: meQuery.queryKey })
      await navigate({ to: "/" })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.code === "AUTH_MFA_INVALID") {
        setCodeError(errorMessageFor(problem.code).message)
      } else {
        showErrorDialog(problem)
      }
    },
  })

  return (
    <AuthLayout
      title="Aktifkan autentikasi dua langkah"
      description="Role Anda mewajibkan kode dari aplikasi autentikator (Google Authenticator, Microsoft Authenticator, atau sejenisnya) setiap kali masuk."
    >
      {!setup ? (
        <div className="flex flex-col gap-3">
          <LoadingButton
            loading={start.isPending}
            loadingText="Menyiapkan…"
            onClick={() => start.mutate()}
          >
            Mulai
          </LoadingButton>
          <LoadingButton
            variant="outline"
            loading={false}
            onClick={async () => {
              await logout()
              await navigate({ to: "/login" })
            }}
          >
            Keluar
          </LoadingButton>
        </div>
      ) : (
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            if (!/^\d{6}$/.test(code)) {
              setCodeError("Kode terdiri dari 6 angka")
              return
            }
            setCodeError(null)
            activate.mutate()
          }}
        >
          <FieldGroup>
            <div className="flex flex-col items-center gap-2">
              <img
                src={setup.qrCodeDataUrl}
                alt="QR code untuk aplikasi autentikator"
                className="size-48 rounded-md border bg-white p-2"
              />
              <p className="text-center text-xs text-muted-foreground">
                Pindai dengan aplikasi autentikator, atau masukkan kode manual:
                <br />
                <span className="font-mono text-sm break-all text-foreground select-all">
                  {setup.secret}
                </span>
              </p>
            </div>
            <Field data-invalid={!!codeError}>
              <FieldLabel htmlFor="code">Kode 6 angka</FieldLabel>
              <Input
                id="code"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                className="font-mono tracking-[0.4em]"
                value={code}
                onChange={(e) => setCode(e.target.value.trim())}
                aria-invalid={!!codeError}
              />
              <FieldDescription>
                Kode berganti setiap 30 detik.
              </FieldDescription>
              <FieldError>{codeError}</FieldError>
            </Field>
            <LoadingButton
              type="submit"
              loading={activate.isPending}
              loadingText="Memverifikasi…"
              className="w-full"
            >
              Aktifkan
            </LoadingButton>
          </FieldGroup>
        </form>
      )}
    </AuthLayout>
  )
}
