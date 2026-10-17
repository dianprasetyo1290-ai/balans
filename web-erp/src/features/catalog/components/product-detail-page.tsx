import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { ArrowLeftIcon, PrinterIcon } from "lucide-react"
import { useState, type ReactNode } from "react"
import { toast } from "sonner"

import { Barcode } from "@/components/common/barcode"
import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { PageTabs } from "@/components/common/page-tabs"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { EntityHistory } from "@/features/audit/components/entity-history"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteProduct,
  PACKAGING_TYPE_LABELS,
  PRODUCT_KINDS,
  PRODUCT_TYPE_LABELS,
  productDisplayName,
  productKindOf,
  productQuery,
  restoreProduct,
} from "../api/catalog-api"
import { ProductFormDialog } from "./product-form-dialog"

/** Label field audit trail produk (tab Riwayat). */
export const PRODUCT_FIELD_LABELS: Record<string, string> = {
  sku: "SKU",
  name: "Nama",
  variant: "Varian",
  productType: "Jenis",
  packagingType: "Tipe kemasan",
  uom: "Satuan",
  shelfLifeDays: "Umur simpan (hari)",
  hasBatch: "Lacak per batch",
  isBundle: "Bundel",
  category: "Kategori",
  minStockQty: "Stok minimum",
  unitsPerCarton: "Isi per karton",
  isActive: "Aktif",
  barcodes: "Barcode",
  bundleComponents: "Komponen",
  deletedAt: "Dihapus pada",
  code: "Kode",
}

type Tab = "summary" | "history"

type ProductDetailPageProps = {
  id: string
  tab: Tab
  onTabChange: (tab: Tab) => void
}

function Item({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  )
}

/** Detail produk dengan tab Ringkasan dan Riwayat (S-113, S-105). Produk terhapus tetap bisa dibuka. */
export function ProductDetailPage({
  id,
  tab,
  onTabChange,
}: ProductDetailPageProps) {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { canAny } = usePermissions()
  const product = useQuery(productQuery(id))
  const [formOpen, setFormOpen] = useState(false)
  const [pending, setPending] = useState<"delete" | "restore" | null>(null)
  const p = product.data
  const canPrint = canAny("logistics.label.post", "logistics.label.read")

  const action = useIdempotentMutation({
    mutationFn: (kind: "delete" | "restore", key) =>
      kind === "delete" ? deleteProduct(id, key) : restoreProduct(id, key),
    onSuccess: async (_, kind) => {
      toast.success(kind === "delete" ? "Data dihapus" : "Data dipulihkan")
      setPending(null)
      await queryClient.invalidateQueries({ queryKey: ["catalog"] })
    },
    onError: (err) => {
      setPending(null)
      showErrorDialog(parseProblem(err))
    },
  })

  if (product.isError) {
    return (
      <p className="text-sm text-destructive">
        Produk tidak ditemukan atau tidak bisa dimuat.
      </p>
    )
  }

  const kind = p ? productKindOf(p) : "finished"
  const cfg = PRODUCT_KINDS[kind]

  return (
    <LoadingArea loading={product.isPending}>
      {p ? (
        <div className="flex max-w-4xl flex-col gap-4">
          <div>
            <Button
              variant="ghost"
              size="sm"
              className="-ml-2"
              nativeButton={false}
              render={<Link to={cfg.to} search={{ page: 1 }} />}
            >
              <ArrowLeftIcon /> {cfg.title}
            </Button>
          </div>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-1">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="text-2xl font-semibold tracking-tight">
                  {p.sku}
                </h1>
                <RecordStatusBadge
                  isActive={p.isActive}
                  deletedAt={p.deletedAt}
                />
                <Badge variant="secondary">
                  {p.isBundle ? "Bundel" : PRODUCT_TYPE_LABELS[p.productType]}
                </Badge>
              </div>
              <p className="text-sm text-muted-foreground">
                {productDisplayName(p)}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {p.allowedActions.includes("update") ? (
                <Button onClick={() => setFormOpen(true)}>
                  Ubah {cfg.noun}
                </Button>
              ) : null}
              {canPrint && !p.deletedAt && p.primaryBarcode ? (
                <Button
                  variant="outline"
                  onClick={() =>
                    void navigate({
                      to: "/logistics/product-labels",
                      search: { productId: p.id },
                    })
                  }
                >
                  <PrinterIcon /> Cetak Label Produk
                </Button>
              ) : null}
              {p.allowedActions.includes("restore") ? (
                <Button variant="outline" onClick={() => setPending("restore")}>
                  Pulihkan
                </Button>
              ) : null}
              {p.allowedActions.includes("delete") ? (
                <Button
                  variant="destructive"
                  onClick={() => setPending("delete")}
                >
                  Hapus
                </Button>
              ) : null}
            </div>
          </div>
          {p.deletedAt ? (
            <p className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm">
              Dihapus {formatDateTime(p.deletedAt)}. Tidak tampil di daftar dan
              pilihan transaksi baru, tetapi tetap tampil di dokumen lama.
            </p>
          ) : null}
          <PageTabs
            label="Bagian produk"
            value={tab}
            onValueChange={onTabChange}
            tabs={[
              { id: "summary", label: "Ringkasan" },
              { id: "history", label: "Riwayat" },
            ]}
          />
          {tab === "summary" ? (
            <div className="flex flex-col gap-6">
              <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3">
                <Item label="Nama">{p.name}</Item>
                <Item label="Varian/rasa">{p.variant || "—"}</Item>
                <Item label="Kategori">{p.categoryName ?? "—"}</Item>
                <Item label="Satuan">{p.uom}</Item>
                {p.isBundle ? null : (
                  <>
                    <Item label="Tipe kemasan">
                      {p.packagingType
                        ? PACKAGING_TYPE_LABELS[p.packagingType]
                        : "—"}
                    </Item>
                    <Item label="Umur simpan">
                      {p.shelfLifeDays ? `${p.shelfLifeDays} hari` : "—"}
                    </Item>
                    <Item label="Lacak per batch">
                      {p.hasBatch ? "Ya" : "Tidak"}
                    </Item>
                    <Item label="Stok minimum">
                      {p.minStockQty
                        ? `${formatQuantity(p.minStockQty)} ${p.uom}`
                        : "—"}
                    </Item>
                  </>
                )}
                <Item label="Isi per karton">
                  {p.unitsPerCarton ? `${p.unitsPerCarton} ${p.uom}` : "—"}
                </Item>
                <Item label="Diperbarui">{formatDateTime(p.updatedAt)}</Item>
              </dl>
              {p.isBundle ? (
                <section className="flex flex-col gap-2">
                  <h2 className="text-sm font-semibold">Komponen per bundel</h2>
                  <ul className="flex flex-col divide-y rounded-lg border">
                    {p.bundleComponents.map((c) => (
                      <li
                        key={c.componentProductId}
                        className="flex items-center justify-between gap-2 p-3 text-sm"
                      >
                        <Link
                          to="/master-data/products/$id"
                          params={{ id: c.componentProductId }}
                          className="text-primary underline-offset-4 hover:underline"
                        >
                          {c.componentSku} ·{" "}
                          {productDisplayName({
                            name: c.componentName,
                            variant: c.componentVariant,
                          })}
                        </Link>
                        <span className="tabular-nums">
                          {formatQuantity(c.qty)} {c.componentUom}
                          {c.componentDeleted ? " (dihapus)" : ""}
                        </span>
                      </li>
                    ))}
                  </ul>
                  <p className="text-xs text-muted-foreground">
                    Stok tersedia bundel = nilai terkecil dari stok komponen ÷
                    qty per bundel (dibulatkan ke bawah).
                  </p>
                </section>
              ) : null}
              <section className="flex flex-col gap-2">
                <h2 className="text-sm font-semibold">Barcode</h2>
                {p.barcodes.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    Belum ada barcode.
                  </p>
                ) : (
                  <ul className="grid gap-3 sm:grid-cols-2">
                    {p.barcodes.map((b) => (
                      <li
                        key={b.barcode}
                        className="flex flex-col gap-2 rounded-lg border p-3"
                      >
                        <div className="flex items-center justify-between gap-2 text-sm">
                          <span className="font-mono">{b.barcode}</span>
                          <span className="flex gap-1">
                            {b.isPrimary ? <Badge>Utama</Badge> : null}
                            <Badge variant="outline">
                              {b.symbology === "ean13" ? "EAN-13" : "Code 128"}
                            </Badge>
                          </span>
                        </div>
                        <div className="h-16 rounded bg-white p-1">
                          <Barcode
                            value={b.barcode}
                            symbology={b.symbology}
                            showText={false}
                            className="h-full"
                          />
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            </div>
          ) : (
            <EntityHistory
              entityType="product"
              entityId={p.id}
              fieldLabels={PRODUCT_FIELD_LABELS}
              valueLabels={{
                productType: PRODUCT_TYPE_LABELS,
                packagingType: PACKAGING_TYPE_LABELS,
              }}
            />
          )}
          <ProductFormDialog
            open={formOpen}
            onOpenChange={setFormOpen}
            kind={kind}
            product={p}
          />
          {pending ? (
            <ConfirmDialog
              open
              onOpenChange={(open) => !open && setPending(null)}
              title={
                pending === "delete" ? `Hapus ${p.sku}?` : `Pulihkan ${p.sku}?`
              }
              description={
                pending === "delete"
                  ? "Data tidak tampil lagi di daftar dan pilihan transaksi baru, tetapi tetap tampil di dokumen lama. SKU tidak bisa dipakai ulang."
                  : "Data kembali tampil di daftar dan bisa dipilih di transaksi baru."
              }
              confirmLabel={
                pending === "delete"
                  ? `Hapus ${cfg.noun}`
                  : `Pulihkan ${cfg.noun}`
              }
              destructive={pending === "delete"}
              loading={action.isPending}
              onConfirm={() => action.mutate(pending)}
            />
          ) : null}
        </div>
      ) : null}
    </LoadingArea>
  )
}
