import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon, SearchIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { DataTable, type DataTableColumn } from "@/components/common/data-table"
import { LoadingButton } from "@/components/common/loading-button"
import {
  SearchPicker,
  type PickerOption,
} from "@/components/common/search-picker"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { usePermissions } from "@/features/auth/api/auth-api"
import { DataExchangeButtons } from "@/features/dataexchange/components/data-exchange-buttons"
import { systemInfoQuery } from "@/features/system/api/system-queries"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createProductCode,
  deleteProductCode,
  partnerOptionsQuery,
  productCodesQuery,
  productOptionsQuery,
  updateProductCode,
  type PartnerProductCode,
} from "../api/party-api"
import { fieldError, handleFormProblem, type FieldErrors } from "./form-parts"

const route = getRouteApi("/_app/master/partner-product-codes")

/** Data Master › Kode Produk Mitra: kode artikel & barcode retailer per SKU (S-134, MD-07). */
export function ProductCodesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can } = usePermissions()
  const canWrite = can("party.partner_product_code.write")
  const info = useQuery(systemInfoQuery)
  const catalogEnabled = info.data?.enabledModules.includes("catalog") ?? false
  const codes = useQuery(productCodesQuery(search))
  const [partner, setPartner] = useState<PickerOption | null>(
    search.partnerId ? { id: search.partnerId, label: "Mitra terpilih" } : null
  )
  const [form, setForm] = useState<{ code?: PartnerProductCode } | null>(null)
  const [deleting, setDeleting] = useState<PartnerProductCode | null>(null)
  const [q, setQ] = useState(search.q ?? "")

  const remove = useIdempotentMutation({
    mutationFn: (c: PartnerProductCode, key) =>
      deleteProductCode(c.id, c.version, key),
    onSuccess: async () => {
      toast.success("Kode produk mitra dihapus")
      setDeleting(null)
      await queryClient.invalidateQueries({
        queryKey: ["party", "product-codes"],
      })
    },
    onError: (err) => {
      setDeleting(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const columns: DataTableColumn<PartnerProductCode>[] = [
    {
      id: "partner",
      header: "Mitra",
      cell: (c) => (
        <div className="flex flex-col">
          <span className="font-medium">{c.partnerName}</span>
          <span className="font-mono text-xs text-muted-foreground">
            {c.partnerCode}
          </span>
        </div>
      ),
    },
    {
      id: "product",
      header: "Produk",
      cell: (c) =>
        c.productSku ? (
          <div className="flex flex-col">
            <span>{c.productName}</span>
            <span className="font-mono text-xs text-muted-foreground">
              {c.productSku}
            </span>
          </div>
        ) : (
          <span className="font-mono text-xs">{c.productId.slice(0, 8)}</span>
        ),
    },
    {
      id: "article",
      header: "Kode artikel",
      cell: (c) => <span className="font-mono">{c.articleCode}</span>,
    },
    {
      id: "barcode",
      header: "Barcode retailer",
      cell: (c) => <span className="font-mono">{c.barcode || "-"}</span>,
    },
    { id: "desc", header: "Keterangan", cell: (c) => c.description || "-" },
    {
      id: "actions",
      header: "",
      align: "right",
      cell: (c) =>
        canWrite ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-11 sm:size-9"
                  aria-label={`Aksi untuk ${c.articleCode}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setForm({ code: c })}>
                Ubah
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setDeleting(c)}
              >
                Hapus
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null,
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Kode Produk Mitra
          </h1>
          <p className="text-sm text-muted-foreground">
            Kode artikel dan barcode retailer per SKU, dicetak di label dan DO
            mitra.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <DataExchangeButtons kind="party.partner_product_code" />
          {canWrite ? (
            <Button
              className="min-h-11 sm:min-h-9"
              disabled={!catalogEnabled}
              onClick={() => setForm({})}
            >
              <PlusIcon /> Tambah Kode Produk Mitra
            </Button>
          ) : null}
        </div>
      </div>
      {canWrite && !catalogEnabled ? (
        <p className="rounded-md bg-amber-500/10 px-3 py-2 text-sm text-amber-800 dark:text-amber-300">
          Modul Produk (catalog) belum aktif, sehingga produk belum bisa
          dipilih.
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <div className="w-full sm:w-72">
          <SearchPicker
            value={partner}
            onValueChange={(p) => {
              setPartner(p)
              void navigate({
                search: (s) => ({ ...s, partnerId: p?.id, page: 1 }),
              })
            }}
            query={partnerOptionsQuery}
            toOption={(p) => ({ id: p.id, label: p.name, description: p.code })}
            placeholder="Semua mitra"
            searchPlaceholder="Cari mitra"
            clearable
          />
        </div>
        <form
          className="flex max-w-md flex-1 gap-2"
          onSubmit={(e) => {
            e.preventDefault()
            void navigate({
              search: (s) => ({ ...s, q: q || undefined, page: 1 }),
            })
          }}
        >
          <Input
            placeholder="Cari kode artikel, barcode, atau keterangan"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            aria-label="Cari Kode Produk Mitra"
          />
          <Button type="submit" variant="outline" size="icon" aria-label="Cari">
            <SearchIcon />
          </Button>
        </form>
      </div>
      <DataTable
        columns={columns}
        rows={codes.data?.items}
        getRowId={(c) => c.id}
        loading={codes.isPending}
        fetching={codes.isFetching}
        page={search.page}
        totalPages={codes.data?.totalPages ?? 0}
        totalItems={codes.data?.totalItems ?? 0}
        onSortChange={() => undefined}
        onPageChange={(page) =>
          void navigate({ search: (s) => ({ ...s, page }) })
        }
        emptyMessage="Belum ada kode produk mitra."
      />
      <ProductCodeFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        code={form?.code}
        defaultPartner={partner}
      />
      {deleting ? (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setDeleting(null)}
          title="Hapus kode produk mitra?"
          description={`${deleting.partnerName} · ${deleting.articleCode}.`}
          confirmLabel="Hapus Kode"
          destructive
          loading={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
        />
      ) : null}
    </div>
  )
}

type ProductCodeFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  code?: PartnerProductCode
  defaultPartner: PickerOption | null
}

function ProductCodeFormDialog(props: ProductCodeFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <ProductCodeForm {...props} /> : null}
    </Dialog>
  )
}

function ProductCodeForm({
  onOpenChange,
  code,
  defaultPartner,
}: ProductCodeFormDialogProps) {
  const queryClient = useQueryClient()
  const [partner, setPartner] = useState<PickerOption | null>(
    code
      ? {
          id: code.partnerId,
          label: code.partnerName,
          description: code.partnerCode,
        }
      : defaultPartner
  )
  const [product, setProduct] = useState<PickerOption | null>(
    code
      ? {
          id: code.productId,
          label: code.productName ?? code.productId,
          description: code.productSku ?? undefined,
        }
      : null
  )
  const [articleCode, setArticleCode] = useState(code?.articleCode ?? "")
  const [barcode, setBarcode] = useState(code?.barcode ?? "")
  const [description, setDescription] = useState(code?.description ?? "")
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      code
        ? updateProductCode(
            code.id,
            { articleCode, barcode, description, version: code.version },
            key
          )
        : createProductCode(
            {
              partnerId: partner?.id ?? "",
              productId: product?.id ?? "",
              articleCode,
              barcode,
              description,
            },
            key
          ),
    onSuccess: async () => {
      toast.success(
        code ? "Kode produk mitra diperbarui" : "Kode produk mitra ditambahkan"
      )
      await queryClient.invalidateQueries({
        queryKey: ["party", "product-codes"],
      })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  const submit = () => {
    const next: FieldErrors = {}
    if (!partner) next.partnerId = "Mitra wajib dipilih"
    if (!product) next.productId = "Produk wajib dipilih"
    if (!articleCode.trim()) next.articleCode = "Kode artikel wajib diisi"
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>
          {code ? "Ubah Kode Produk Mitra" : "Tambah Kode Produk Mitra"}
        </DialogTitle>
        <DialogDescription>
          Satu produk hanya punya satu kode di setiap mitra. Impor dari Excel
          menyusul bersama menu Import/Export Excel.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.partnerId}>
            <FieldLabel htmlFor="code-partner">Mitra</FieldLabel>
            <SearchPicker
              id="code-partner"
              value={partner}
              onValueChange={setPartner}
              query={partnerOptionsQuery}
              toOption={(p) => ({
                id: p.id,
                label: p.name,
                description: p.code,
              })}
              placeholder="Pilih mitra"
              searchPlaceholder="Cari mitra"
              disabled={!!code}
              aria-invalid={!!errors.partnerId}
            />
            <FieldError errors={fieldError(errors, "partnerId")} />
          </Field>
          <Field data-invalid={!!errors.productId}>
            <FieldLabel htmlFor="code-product">Produk</FieldLabel>
            <SearchPicker
              id="code-product"
              value={product}
              onValueChange={setProduct}
              query={productOptionsQuery}
              toOption={(p) => ({
                id: p.id,
                label: p.name,
                description: p.sku,
              })}
              placeholder="Pilih produk"
              searchPlaceholder="Cari SKU atau nama produk"
              disabled={!!code}
              aria-invalid={!!errors.productId}
            />
            <FieldError errors={fieldError(errors, "productId")} />
          </Field>
          <Field data-invalid={!!errors.articleCode}>
            <FieldLabel htmlFor="code-article">Kode artikel mitra</FieldLabel>
            <Input
              id="code-article"
              value={articleCode}
              aria-invalid={!!errors.articleCode}
              onChange={(e) => setArticleCode(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "articleCode")} />
          </Field>
          <Field data-invalid={!!errors.barcode}>
            <FieldLabel htmlFor="code-barcode">Barcode retailer</FieldLabel>
            <Input
              id="code-barcode"
              inputMode="numeric"
              value={barcode}
              aria-invalid={!!errors.barcode}
              onChange={(e) => setBarcode(e.target.value)}
            />
            <FieldDescription>
              Opsional; dicetak di label produk mitra.
            </FieldDescription>
            <FieldError errors={fieldError(errors, "barcode")} />
          </Field>
          <Field data-invalid={!!errors.description}>
            <FieldLabel htmlFor="code-desc">Keterangan</FieldLabel>
            <Input
              id="code-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "description")} />
          </Field>
        </FieldGroup>
        <DialogFooter className="mt-6">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
