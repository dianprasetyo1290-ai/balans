import { createFileRoute, Link } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

export const Route = createFileRoute("/_app/")({
  component: OperationalSummaryPage,
})

function OperationalSummaryPage() {
  return (
    <div className="flex max-w-3xl flex-col gap-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          Ringkasan Operasional
        </h1>
        <p className="text-sm text-muted-foreground">
          Ringkasan stok, penjualan, dan persetujuan tampil di sini setelah
          modul dirilis.
        </p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Fondasi sistem siap</CardTitle>
          <CardDescription>
            Periksa versi aplikasi dan status layanan di halaman Tentang Sistem.
          </CardDescription>
        </CardHeader>
        <CardFooter>
          <Button variant="outline" render={<Link to="/system/about" />}>
            Buka Tentang Sistem
          </Button>
        </CardFooter>
      </Card>
    </div>
  )
}
