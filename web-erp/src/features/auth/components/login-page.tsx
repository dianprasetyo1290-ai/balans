import { zodResolver } from "@hookform/resolvers/zod"
import { useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "react-hook-form"
import { z } from "zod"

import { LoadingButton } from "@/components/common/loading-button"
import {
  Field,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { errorMessageFor } from "@/lib/api/error-messages"
import { parseProblem } from "@/lib/api/problem"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { login } from "../api/auth-api"
import { AuthLayout } from "./auth-layout"

const schema = z.object({
  email: z.email("Alamat email tidak valid"),
  password: z.string().min(1),
  totpCode: z
    .string()
    .regex(/^\d{6}$/, "Kode terdiri dari 6 angka")
    .or(z.literal("")),
})

type Values = z.infer<typeof schema>

type LoginPageProps = {
  redirect?: string
}

/** Halaman Masuk (S-101). Password salah dan akun terkunci tampil sebagai modal (Technical Guideline 10.6). */
export function LoginPage({ redirect }: LoginPageProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [needsCode, setNeedsCode] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "", totpCode: "" },
  })

  const onSubmit = form.handleSubmit(async (values) => {
    if (needsCode && !values.totpCode) {
      form.setError("totpCode", {
        message: "Masukkan kode dari aplikasi autentikator",
      })
      return
    }
    setSubmitting(true)
    try {
      await login(
        values.email,
        values.password,
        needsCode ? values.totpCode : undefined
      )
      queryClient.clear()
      await navigate({
        to: redirect && redirect.startsWith("/") ? redirect : "/",
      })
    } catch (err) {
      const problem = parseProblem(err)
      if (problem.code === "AUTH_MFA_REQUIRED") {
        setNeedsCode(true)
      } else if (problem.code === "AUTH_MFA_INVALID") {
        form.setError("totpCode", {
          message: errorMessageFor(problem.code).message,
        })
      } else if (problem.code === "VALIDATION_FAILED" && problem.fields) {
        for (const [field, message] of Object.entries(problem.fields)) {
          form.setError(field as keyof Values, { message })
        }
      } else {
        showErrorDialog(problem)
      }
    } finally {
      setSubmitting(false)
    }
  })

  const { errors } = form.formState

  return (
    <AuthLayout
      title={needsCode ? "Kode autentikasi" : "Masuk"}
      description={
        needsCode
          ? "Masukkan 6 angka dari aplikasi autentikator di HP Anda."
          : "Masuk dengan email dan password akun Balans ERP."
      }
    >
      <form onSubmit={onSubmit} noValidate>
        <FieldGroup>
          {!needsCode ? (
            <>
              <Field data-invalid={!!errors.email}>
                <FieldLabel htmlFor="email">Email</FieldLabel>
                <Input
                  id="email"
                  type="email"
                  autoComplete="username"
                  aria-invalid={!!errors.email}
                  {...form.register("email")}
                />
                <FieldError errors={[errors.email]} />
              </Field>
              <Field data-invalid={!!errors.password}>
                <div className="flex items-center justify-between">
                  <FieldLabel htmlFor="password">Password</FieldLabel>
                  <Link
                    to="/forgot-password"
                    className="text-sm text-muted-foreground underline-offset-4 hover:underline"
                  >
                    Lupa password?
                  </Link>
                </div>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  aria-invalid={!!errors.password}
                  {...form.register("password")}
                />
                <FieldError errors={[errors.password]} />
              </Field>
            </>
          ) : (
            <Field data-invalid={!!errors.totpCode}>
              <FieldLabel htmlFor="totpCode">Kode 6 angka</FieldLabel>
              <Input
                id="totpCode"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}

                className="font-mono tracking-[0.4em]"
                aria-invalid={!!errors.totpCode}
                {...form.register("totpCode")}
              />
              <FieldError errors={[errors.totpCode]} />
            </Field>
          )}
          <LoadingButton
            type="submit"
            loading={submitting}
            loadingText="Memproses…"
            className="w-full"
          >
            {needsCode ? "Verifikasi" : "Masuk"}
          </LoadingButton>
          {needsCode ? (
            <button
              type="button"
              className="text-sm text-muted-foreground underline-offset-4 hover:underline"
              onClick={() => {
                setNeedsCode(false)
                form.setValue("totpCode", "")
              }}
            >
              Kembali
            </button>
          ) : null}
        </FieldGroup>
      </form>
    </AuthLayout>
  )
}
