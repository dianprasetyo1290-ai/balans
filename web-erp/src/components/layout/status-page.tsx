import { Link } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"

type StatusPageProps = {
  code: string
  title: string
  description: string
}

/** Halaman 403, 404, dan kesalahan tak terduga (Technical Guideline 10.6). */
export function StatusPage({ code, title, description }: StatusPageProps) {
  return (
    <main className="flex min-h-[60svh] flex-col items-center justify-center gap-4 p-6 text-center">
      <p className="font-mono text-sm text-muted-foreground">{code}</p>
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="max-w-md text-sm text-muted-foreground">{description}</p>
      <Button render={<Link to="/" />}>Kembali ke Ringkasan</Button>
    </main>
  )
}

export function NotFoundPage() {
  return <StatusPage code="404" title="Halaman tidak ditemukan" description="Alamat yang Anda buka tidak tersedia atau sudah dipindahkan." />
}

export function ForbiddenPage() {
  return (
    <StatusPage
      code="403"
      title="Akses ditolak"
      description="Anda tidak memiliki akses ke halaman ini. Hubungi Admin Sistem jika Anda membutuhkannya."
    />
  )
}
