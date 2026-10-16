import { zodResolver } from "@hookform/resolvers/zod"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { useState } from "react"
import { useForm } from "react-hook-form"
import { toast } from "sonner"
import { z } from "zod"

import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { Button } from "@/components/ui/button"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { api, unwrap } from "@/lib/api/client"
import { parseProblem } from "@/lib/api/problem"
import { showErrorDialog } from "@/stores/error-dialog-store"

import { AuthLayout } from "./auth-layout"

const passwordSchema = z
  .object({
    password: z
      .string()
      .min(10, "Password minimal 10 karakter")
      .max(128, "Password maksimal 128 karakter"),
    confirm: z.string().min(1),
  })
  .refine((v) => v.password === v.confirm, {
    path: ["confirm"],
    message: "Konfirmasi password tidak sama",
  })

type PasswordValues = z.infer<typeof passwordSchema>

/** Atur password dari tautan undangan atau atur ulang (S-102). */
export function SetPasswordPage({ token }: { token: string }) {
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)
  const info = useQuery({
    queryKey: ["identity", "password-token", token],
    queryFn: () =>
      unwrap(
        api.GET("/api/v1/auth/password/token", { params: { query: { token } } })
      ),
    retry: false,
    enabled: token !== "",
  })
  const form = useForm<PasswordValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { password: "", confirm: "" },
  })
  const { errors } = form.formState

  const onSubmit = form.handleSubmit(async (values) => {
    setSubmitting(true)
    try {
      await unwrap(
        api.POST("/api/v1/auth/password/setup", {
          body: { token, password: values.password },
        })
      )
      toast.success("Password tersimpan. Silakan masuk.")
      await navigate({ to: "/login" })
    } catch (err) {
      const problem = parseProblem(err)
      if (problem.code === "VALIDATION_FAILED" && problem.fields?.password) {
        form.setError("password", { message: problem.fields.password })
      } else {
        showErrorDialog(problem)
      }
    } finally {
      setSubmitting(false)
    }
  })

  if (token === "" || info.isError) {
    return (
      <AuthLayout
        title="Tautan tidak berlaku"
        description="Tautan sudah dipakai atau kedaluwarsa."
      >
        <div className="flex flex-col gap-3">
          <Button render={<Link to="/forgot-password" />}>
            Minta tautan baru
          </Button>
          <Button variant="outline" render={<Link to="/login" />}>
            Kembali ke halaman Masuk
          </Button>
        </div>
      </AuthLayout>
    )
  }

  const invite = info.data?.purpose === "invite"
  return (
    <AuthLayout
      title={invite ? "Selamat datang" : "Atur ulang password"}
      description={
        info.data ? `${info.data.fullName} · ${info.data.email}` : undefined
      }
    >
      <LoadingArea loading={info.isPending} label="Memeriksa tautan">
        <form onSubmit={onSubmit} noValidate>
          <FieldGroup>
            <Field data-invalid={!!errors.password}>
              <FieldLabel htmlFor="password">Password baru</FieldLabel>
              <Input
                id="password"
                type="password"
                autoComplete="new-password"
                aria-invalid={!!errors.password}
                {...form.register("password")}
              />
              <FieldDescription>
                Minimal 10 karakter, bukan password umum, dan tidak memuat email
                Anda.
              </FieldDescription>
              <FieldError errors={[errors.password]} />
            </Field>
            <Field data-invalid={!!errors.confirm}>
              <FieldLabel htmlFor="confirm">Ulangi password</FieldLabel>
              <Input
                id="confirm"
                type="password"
                autoComplete="new-password"
                aria-invalid={!!errors.confirm}
                {...form.register("confirm")}
              />
              <FieldError errors={[errors.confirm]} />
            </Field>
            <LoadingButton
              type="submit"
              loading={submitting}
              loadingText="Menyimpan…"
              className="w-full"
            >
              Simpan password
            </LoadingButton>
          </FieldGroup>
        </form>
      </LoadingArea>
    </AuthLayout>
  )
}

const forgotSchema = z.object({ email: z.email("Alamat email tidak valid") })

/** Lupa password: selalu menampilkan pesan yang sama agar email tidak bisa ditebak. */
export function ForgotPasswordPage() {
  const [sent, setSent] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const form = useForm<z.infer<typeof forgotSchema>>({
    resolver: zodResolver(forgotSchema),
    defaultValues: { email: "" },
  })
  const { errors } = form.formState

  const onSubmit = form.handleSubmit(async (values) => {
    setSubmitting(true)
    try {
      await unwrap(api.POST("/api/v1/auth/password/forgot", { body: values }))
      setSent(true)
    } catch (err) {
      showErrorDialog(parseProblem(err))
    } finally {
      setSubmitting(false)
    }
  })

  if (sent) {
    return (
      <AuthLayout
        title="Periksa email Anda"
        description="Jika email terdaftar, tautan atur ulang password sudah dikirim dan berlaku 1 jam."
      >
        <Button
          variant="outline"
          className="w-full"
          render={<Link to="/login" />}
        >
          Kembali ke halaman Masuk
        </Button>
      </AuthLayout>
    )
  }
  return (
    <AuthLayout
      title="Lupa password"
      description="Masukkan email akun Anda. Kami kirim tautan untuk mengatur ulang password."
    >
      <form onSubmit={onSubmit} noValidate>
        <FieldGroup>
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
          <LoadingButton
            type="submit"
            loading={submitting}
            loadingText="Mengirim…"
            className="w-full"
          >
            Kirim tautan
          </LoadingButton>
          <Link
            to="/login"
            className="text-center text-sm text-muted-foreground underline-offset-4 hover:underline"
          >
            Kembali ke halaman Masuk
          </Link>
        </FieldGroup>
      </form>
    </AuthLayout>
  )
}
