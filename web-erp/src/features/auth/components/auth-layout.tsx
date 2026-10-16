import type { ReactNode } from "react"

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

type AuthLayoutProps = {
  title: string
  description?: ReactNode
  children: ReactNode
}

/** Kerangka halaman di luar sesi: masuk, atur password, aktivasi MFA. */
export function AuthLayout({ title, description, children }: AuthLayoutProps) {
  return (
    <main className="flex min-h-svh items-center justify-center bg-muted/40 p-4">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <div className="flex items-center justify-center gap-2">
          <span className="flex size-9 items-center justify-center rounded-md bg-primary text-base font-semibold text-primary-foreground">
            B
          </span>
          <span className="flex flex-col leading-tight">
            <span className="font-semibold">Balans ERP</span>
            <span className="text-xs text-muted-foreground">
              Balans Nutrition
            </span>
          </span>
        </div>
        <Card>
          <CardHeader>
            <CardTitle className="text-xl">{title}</CardTitle>
            {description ? (
              <CardDescription>{description}</CardDescription>
            ) : null}
          </CardHeader>
          <CardContent>{children}</CardContent>
        </Card>
      </div>
    </main>
  )
}
