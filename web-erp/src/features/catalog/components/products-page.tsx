import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import {
  MoreHorizontalIcon,
  PlusIcon,
  PrinterIcon,
  SearchIcon,
  TagsIcon,
} from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { RecordStatusBadge } from "@/components/common/record-status-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { bundleAvailabilityQuery } from "@/features/inventory/api/inventory-api"
import { systemInfoQuery } from "@/features/system/api/system-queries"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatQuantity } from "@/lib/format/number"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  deleteProduct,
  PACKAGING_TYPE_LABELS,
  PRODUCT_KINDS,
  PRODUCT_TYPE_LABELS,
  productDisplayName,
  productsQuery,
  restoreProduct,
  type Product,
  type ProductKind,
  type ProductListSearch,
  type ProductType,
} from "../api/catalog-api"
import { CategoriesDialog } from "./categories-dialog"
import { ProductFormDialog } from "./product-form-dialog"

type ProductsPageProps = {
  kind: ProductKind
  search: ProductListSearch
  onSearchChange: (patch: Partial<ProductListSearch>) => void
}

type Pending = { kind: "delete" | "restore"; product: Product }

const STATUS_ALL = "all"

/**
 * Data Master › Produk, Packaging & Material, dan Bundling (S-113, S-114, S-115). Satu halaman daftar dengan
 * filter jenis yang berbeda; produk terhapus bisa dilihat dan dipulihkan Admin Sistem (S-106).
 */
export function ProductsPage({
  kind,
  search,
  onSearchChange,
}: ProductsPageProps) {
  const cfg = PRODUCT_KINDS[kind]
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const { can, canAny } = usePermissions()
  const products = useQuery(productsQuery(kind, search))
  const [q, setQ] = useState(search.q ?? "")
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Product | undefined>()
  const [categoriesOpen, setCategoriesOpen] = useState(false)
  const [pending, setPending] = useState<Pending | null>(null)
  const canWrite = can("catalog.product.write")
  const canPrint = canAny("logistics.label.post", "logistics.label.read")
  const info = useQuery(systemInfoQuery)
  const bundleIds =
    kind === "bundle" ? (products.data?.items ?? []).map((p) => p.id) : []
  // Stok tersedia bundel = min(floor(stok komponen ÷ qty)) dari modul inventory (S-115).
  const showAvailability =
    kind === "bundle" &&
    can("inventory.stock.read") &&
    (info.data?.enabledModules.includes("inventory") ?? false)
  const availability = useQuery({
    ...bundleAvailabilityQuery(bundleIds),
    enabled: showAvailability && bundleIds.length > 0,
  })
  const availableQty = (id: string) =>
    availability.data?.items.find((a) => a.bundleId === id)?.availableQty

  const action = useIdempotentMutation({
    mutationFn: (p: Pending, key) =>
      p.kind === "delete"
        ? deleteProduct(p.product.id, key)
        : restoreProduct(p.product.id, key),
    onSuccess: async (_, p) => {
      toast.success(
        p.kind === "delete" ? `${cfg.noun} dihapus` : `${cfg.noun} dipulihkan`
      )
      setPending(null)
      await queryClient.invalidateQueries({ queryKey: ["catalog"] })
    },
    onError: (err) => {
      setPending(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<Product>[] = [
    {
      id: "sku",
      header: "SKU",
      sortKey: "sku",
      cell: (p) => (
        <Link
          to="/master-data/products/$id"
          params={{ id: p.id }}
          className="font-medium whitespace-nowrap text-primary underline-offset-4 hover:underline"
        >
          {p.sku}
        </Link>
      ),
    },
    {
      id: "name",
      header: "Nama",
      sortKey: "name",
      cell: (p) => (
        <div className="flex min-w-48 flex-col">
          <span>{productDisplayName(p)}</span>
          {p.categoryName ? (
            <span className="text-xs text-muted-foreground">
              {p.categoryName}
            </span>
          ) : null}
        </div>
      ),
    },
    ...(kind === "material"
      ? [
          {
            id: "type",
            header: "Jenis",
            cell: (p: Product) => (
              <Badge variant="secondary">
                {PRODUCT_TYPE_LABELS[p.productType]}
              </Badge>
            ),
          },
        ]
      : []),
    ...(kind === "bundle"
      ? [
          {
            id: "components",
            header: "Komponen",
            cell: (p: Product) => (
              <ul className="text-xs">
                {p.bundleComponents.map((c) => (
                  <li key={c.componentProductId} className="whitespace-nowrap">
                    {formatQuantity(c.qty)} × {c.componentSku}
                  </li>
                ))}
              </ul>
            ),
          },
          ...(showAvailability
            ? [
                {
                  id: "available",
                  header: "Stok tersedia",
                  align: "right" as const,
                  cell: (p: Product) => (
                    <span className="whitespace-nowrap tabular-nums">
                      {availability.isPending
                        ? "…"
                        : `${formatQuantity(availableQty(p.id) ?? "0")} ${p.uom}`}
                    </span>
                  ),
                },
              ]
            : []),
        ]
      : [
          {
            id: "packaging",
            header: "Kemasan",
            cell: (p: Product) =>
              p.packagingType ? PACKAGING_TYPE_LABELS[p.packagingType] : "—",
          },
          {
            id: "shelfLife",
            header: "Umur simpan",
            align: "right" as const,
            cell: (p: Product) =>
              p.shelfLifeDays ? `${p.shelfLifeDays} hari` : "—",
          },
        ]),
    {
      id: "barcode",
      header: "Barcode utama",
      cell: (p) => (
        <span className="font-mono text-xs">{p.primaryBarcode ?? "—"}</span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: (p) => (
        <RecordStatusBadge isActive={p.isActive} deletedAt={p.deletedAt} />
      ),
    },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (p) => (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon"
                aria-label={`Aksi untuk ${p.sku}`}
              />
            }
          >
            <MoreHorizontalIcon />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              onClick={() =>
                void navigate({
                  to: "/master-data/products/$id",
                  params: { id: p.id },
                })
              }
            >
              Lihat Detail
            </DropdownMenuItem>
            {p.allowedActions.includes("update") ? (
              <DropdownMenuItem
                onClick={() => {
                  setEditing(p)
                  setFormOpen(true)
                }}
              >
                Ubah
              </DropdownMenuItem>
            ) : null}
            {canPrint && !p.deletedAt && p.primaryBarcode ? (
              <DropdownMenuItem
                onClick={() =>
                  void navigate({
                    to: "/logistics/product-labels",
                    search: { productId: p.id },
                  })
                }
              >
                Cetak Label Produk
              </DropdownMenuItem>
            ) : null}
            {p.allowedActions.includes("restore") ? (
              <DropdownMenuItem
                onClick={() => setPending({ kind: "restore", product: p })}
              >
                Pulihkan
              </DropdownMenuItem>
            ) : null}
            {p.allowedActions.includes("delete") ? (
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setPending({ kind: "delete", product: p })}
              >
                Hapus
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ]

  const statusValue = search.deleted
    ? "deleted"
    : search.isActive === undefined
      ? STATUS_ALL
      : search.isActive
        ? "active"
        : "inactive"
  const statusLabels: Record<string, string> = {
    [STATUS_ALL]: "Semua status",
    active: "Aktif",
    inactive: "Nonaktif",
    deleted: "Dihapus",
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">{cfg.title}</h1>
          <p className="text-sm text-muted-foreground">{cfg.description}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons kind="catalog.product" />
          {kind === "finished" && canPrint ? (
            <Button
              variant="outline"
              nativeButton={false}
              render={<Link to="/logistics/product-labels" search={{}} />}
            >
              <PrinterIcon /> Cetak Label Produk
            </Button>
          ) : null}
          {kind !== "bundle" ? (
            <Button variant="outline" onClick={() => setCategoriesOpen(true)}>
              <TagsIcon /> Kategori
            </Button>
          ) : null}
          {canWrite ? (
            <Button
              onClick={() => {
                setEditing(undefined)
                setFormOpen(true)
              }}
            >
              <PlusIcon />{" "}
              {kind === "bundle"
                ? "Tambah Bundel"
                : kind === "material"
                  ? "Tambah Packaging/Material"
                  : "Tambah Produk"}
            </Button>
          ) : null}
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        <form
          className="flex w-full max-w-md gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            onSearchChange({ q: q || undefined, page: 1 })
          }}
        >
          <Input
            placeholder="Cari SKU, nama, atau barcode"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Cari Produk"
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
        {kind === "material" ? (
          <Select
            value={search.productType ?? STATUS_ALL}
            onValueChange={(v) =>
              onSearchChange({
                productType:
                  v === STATUS_ALL || !v ? undefined : (v as ProductType),
                page: 1,
              })
            }
          >
            <SelectTrigger className="w-44" aria-label="Filter jenis">
              <SelectValue>
                {(v: string) =>
                  v === STATUS_ALL
                    ? "Semua jenis"
                    : PRODUCT_TYPE_LABELS[v as ProductType]
                }
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={STATUS_ALL}>Semua jenis</SelectItem>
              <SelectItem value="packaging">Packaging</SelectItem>
              <SelectItem value="reprocess_material">Material</SelectItem>
            </SelectContent>
          </Select>
        ) : null}
        <Select
          value={statusValue}
          onValueChange={(v) =>
            onSearchChange({
              page: 1,
              deleted: v === "deleted" ? true : undefined,
              isActive:
                v === "active" ? true : v === "inactive" ? false : undefined,
            })
          }
        >
          <SelectTrigger className="w-40" aria-label="Filter status">
            <SelectValue>{(v: string) => statusLabels[v]}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            {Object.entries(statusLabels).map(([v, l]) => (
              <SelectItem key={v} value={v}>
                {l}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DataTable
        columns={columns}
        rows={products.data?.items}
        getRowId={(p) => p.id}
        loading={products.isPending}
        fetching={products.isFetching}
        page={search.page}
        totalPages={products.data?.totalPages ?? 0}
        totalItems={products.data?.totalItems ?? 0}
        sort={search.sort}
        onSortChange={(sort) => onSearchChange({ sort, page: 1 })}
        onPageChange={(page) => onSearchChange({ page })}
        emptyMessage={
          search.deleted
            ? "Tidak ada data yang dihapus."
            : `Belum ada ${cfg.noun.toLowerCase()} yang cocok.`
        }
      />
      <ProductFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        kind={kind}
        product={editing}
      />
      <CategoriesDialog
        open={categoriesOpen}
        onOpenChange={setCategoriesOpen}
      />
      {pending ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setPending(null)}
          title={
            pending.kind === "delete"
              ? `Hapus ${pending.product.sku}?`
              : `Pulihkan ${pending.product.sku}?`
          }
          description={
            pending.kind === "delete"
              ? "Data tidak tampil lagi di daftar dan pilihan transaksi baru, tetapi tetap tampil di dokumen lama. SKU tidak bisa dipakai ulang. Admin Sistem bisa memulihkannya."
              : "Data kembali tampil di daftar dan bisa dipilih di transaksi baru."
          }
          confirmLabel={
            pending.kind === "delete"
              ? `Hapus ${cfg.noun}`
              : `Pulihkan ${cfg.noun}`
          }
          destructive={pending.kind === "delete"}
          loading={action.isPending}
          onConfirm={() => action.mutate(pending)}
        />
      ) : null}
    </div>
  )
}
