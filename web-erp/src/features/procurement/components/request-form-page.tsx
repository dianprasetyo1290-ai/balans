import { useQuery, useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import { PlusIcon, Trash2Icon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingArea } from "@/components/common/loading-area"
import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { QuantityInput } from "@/components/common/quantity-input"
import { Button } from "@/components/ui/button"
import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import {
  ProductPicker,
  type ProductOption,
} from "@/features/catalog/components/product-picker"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createRequest,
  divisionOptionsQuery,
  requestQuery,
  updateRequest,
  type PurchaseRequest,
  type PurchaseRequestFields,
} from "../api/procurement-api"
import { documentTotal, lineAmount } from "../lib/procurement-lib"
import { fieldErrors, LineError, OptionSelect } from "./shared"

type LineDraft = {
  key: string
  product: ProductOption | null
  description: string
  qty: string
  estimatedPrice: string
  note: string
}

const emptyLine = (): LineDraft => ({
  key: crypto.randomUUID(),
  product: null,
  description: "",
  qty: "",
  estimatedPrice: "",
  note: "",
})

const fromRequest = (r: PurchaseRequest): LineDraft[] =>
  r.lines.map((l) => ({
    key: crypto.randomUUID(),
    product: l.product ? { ...l.product, variant: "" } : null,
    description: l.description,
    qty: l.qty,
    estimatedPrice: l.estimatedPrice,
    note: l.note,
  }))

/** Pengadaan › Purchase Request › Buat Purchase Request (S-201). */
export function RequestCreatePage() {
  return <RequestForm />
}

/** Ubah draft Purchase Request. */
export function RequestEditPage({ id }: { id: string }) {
  const request = useQuery(requestQuery(id))
  return (
    <LoadingArea loading={request.isPending}>
      {request.data ? (
        request.data.allowedActions.includes("edit") ? (
          <RequestForm request={request.data} />
        ) : (
          <p className="text-sm text-muted-foreground">
            Hanya Purchase Request berstatus Draft yang bisa diubah.
          </p>
        )
      ) : null}
    </LoadingArea>
  )
}

function RequestForm({ request }: { request?: PurchaseRequest }) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const divisions = useQuery(divisionOptionsQuery)
  const [divisionId, setDivisionId] = useState(request?.divisionId ?? "")
  const [businessDate, setBusinessDate] = useState(
    request?.businessDate ?? todayBusinessDate()
  )
  const [neededDate, setNeededDate] = useState(request?.neededDate ?? "")
  const [note, setNote] = useState(request?.note ?? "")
  const [lines, setLines] = useState<LineDraft[]>(() =>
    request ? fromRequest(request) : [emptyLine()]
  )
  const [errors, setErrors] = useState<Record<string, string>>({})

  const update = (key: string, patch: Partial<LineDraft>) =>
    setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)))
  const total = documentTotal(
    lines.map((l) => ({ qty: l.qty, price: l.estimatedPrice }))
  )

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const body: PurchaseRequestFields = {
        divisionId,
        businessDate,
        neededDate: neededDate || null,
        note,
        lines: lines.map((l) => ({
          productId: l.product?.id ?? null,
          description: l.description,
          qty: l.qty,
          estimatedPrice: l.estimatedPrice || "0",
          note: l.note,
        })),
      }
      return request
        ? updateRequest(request.id, { ...body, version: request.version }, key)
        : createRequest(body, key)
    },
    onSuccess: async (saved) => {
      toast.success("Draft Purchase Request disimpan")
      await queryClient.invalidateQueries({ queryKey: ["procurement"] })
      void navigate({
        to: "/procurement/purchase-requests/$id",
        params: { id: saved.id },
        search: {},
      })
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.fields) {
        setErrors(problem.fields)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    const next: Record<string, string> = {}
    if (!divisionId) next.divisionId = "Divisi peminta wajib dipilih"
    if (!businessDate) next.businessDate = "Tanggal wajib diisi"
    if (lines.length === 0) next.lines = "Minimal satu baris barang"
    lines.forEach((l, i) => {
      if (!l.product && !l.description.trim())
        next[`lines.${i}.productId`] = "Pilih produk atau isi uraian barang"
      if (!l.qty || Number(l.qty) <= 0)
        next[`lines.${i}.qty`] = "Qty harus lebih dari nol"
    })
    setErrors(next)
    if (Object.keys(next).length === 0) save.mutate()
  }

  const lineError = (i: number, field: string) => errors[`lines.${i}.${field}`]

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">
          {request
            ? `Ubah Purchase Request ${request.number ?? ""}`.trim()
            : "Buat Purchase Request"}
        </h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Draft bisa disimpan bertahap. Setelah diajukan, Purchase Request
          menunggu persetujuan Head divisi peminta.
        </p>
      </div>
      <form
        noValidate
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <div className="grid gap-4 sm:grid-cols-3">
          <Field data-invalid={!!errors.divisionId}>
            <FieldLabel htmlFor="pr-division">Divisi Peminta</FieldLabel>
            <OptionSelect
              id="pr-division"
              value={divisionId}
              onValueChange={setDivisionId}
              options={divisions.data ?? []}
              placeholder="Pilih divisi"
              aria-invalid={!!errors.divisionId}
            />
            <FieldError errors={fieldErrors(errors.divisionId)} />
          </Field>
          <Field data-invalid={!!errors.businessDate}>
            <FieldLabel htmlFor="pr-date">Tanggal</FieldLabel>
            <DateInput
              id="pr-date"
              value={businessDate}
              onValueChange={setBusinessDate}
              aria-invalid={!!errors.businessDate}
            />
            <FieldError errors={fieldErrors(errors.businessDate)} />
          </Field>
          <Field data-invalid={!!errors.neededDate}>
            <FieldLabel htmlFor="pr-needed">Tanggal Dibutuhkan</FieldLabel>
            <DateInput
              id="pr-needed"
              value={neededDate}
              onValueChange={setNeededDate}
              placeholder="Opsional"
              aria-invalid={!!errors.neededDate}
            />
            <FieldError errors={fieldErrors(errors.neededDate)} />
          </Field>
        </div>
        <Field data-invalid={!!errors.note}>
          <FieldLabel htmlFor="pr-note">Keperluan / Catatan</FieldLabel>
          <Textarea
            id="pr-note"
            value={note}
            rows={2}
            maxLength={1000}
            onChange={(e) => setNote(e.target.value)}
          />
          <FieldError errors={fieldErrors(errors.note)} />
        </Field>
        <div className="overflow-x-auto rounded-lg border">
          <Table className="min-w-[60rem]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-72">Produk</TableHead>
                <TableHead>Uraian (bila bukan produk katalog)</TableHead>
                <TableHead className="w-36 text-right">Qty</TableHead>
                <TableHead className="w-40 text-right">
                  Estimasi Harga
                </TableHead>
                <TableHead className="w-36 text-right">Jumlah</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {lines.map((l, i) => (
                <TableRow key={l.key} className="align-top">
                  <TableCell>
                    <div className="flex gap-1">
                      <ProductPicker
                        value={l.product}
                        onValueChange={(p) => update(l.key, { product: p })}
                        aria-invalid={!!lineError(i, "productId")}
                      />
                      {l.product ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          onClick={() => update(l.key, { product: null })}
                        >
                          Hapus
                        </Button>
                      ) : null}
                    </div>
                    <LineError message={lineError(i, "productId")} />
                  </TableCell>
                  <TableCell>
                    <Input
                      aria-label={`Uraian baris ${i + 1}`}
                      value={l.description}
                      maxLength={500}
                      placeholder={
                        l.product ? "Opsional" : "Mis. lakban coklat"
                      }
                      onChange={(e) =>
                        update(l.key, { description: e.target.value })
                      }
                    />
                    <LineError message={lineError(i, "description")} />
                  </TableCell>
                  <TableCell>
                    <QuantityInput
                      aria-label={`Qty baris ${i + 1}`}
                      value={l.qty}
                      unit={l.product?.uom}
                      aria-invalid={!!lineError(i, "qty")}
                      onValueChange={(v) => update(l.key, { qty: v })}
                    />
                    <LineError message={lineError(i, "qty")} />
                  </TableCell>
                  <TableCell>
                    <MoneyInput
                      aria-label={`Estimasi harga baris ${i + 1}`}
                      value={l.estimatedPrice}
                      aria-invalid={!!lineError(i, "estimatedPrice")}
                      onValueChange={(v) =>
                        update(l.key, { estimatedPrice: v })
                      }
                    />
                    <LineError message={lineError(i, "estimatedPrice")} />
                  </TableCell>
                  <TableCell className="pt-4 text-right tabular-nums">
                    {formatRupiah(lineAmount(l.qty, l.estimatedPrice))}
                  </TableCell>
                  <TableCell>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`Hapus baris ${i + 1}`}
                      disabled={lines.length <= 1}
                      onClick={() =>
                        setLines((ls) => ls.filter((x) => x.key !== l.key))
                      }
                    >
                      <Trash2Icon />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell colSpan={4} className="font-medium">
                  Estimasi Total (preview)
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {formatRupiah(total)}
                </TableCell>
                <TableCell />
              </TableRow>
            </TableFooter>
          </Table>
        </div>
        {errors.lines ? (
          <p className="text-sm text-destructive">{errors.lines}</p>
        ) : null}
        <div className="flex flex-wrap justify-between gap-3">
          <Button
            type="button"
            variant="outline"
            onClick={() => setLines((ls) => [...ls, emptyLine()])}
          >
            <PlusIcon /> Tambah Baris
          </Button>
          <div className="flex gap-2">
            <Button
              variant="outline"
              render={
                request ? (
                  <Link
                    to="/procurement/purchase-requests/$id"
                    params={{ id: request.id }}
                    search={{}}
                  />
                ) : (
                  <Link
                    to="/procurement/purchase-requests"
                    search={{ page: 1 }}
                  />
                )
              }
            >
              Batal
            </Button>
            <LoadingButton
              type="submit"
              loading={save.isPending}
              loadingText="Menyimpan…"
            >
              Simpan Draft
            </LoadingButton>
          </div>
        </div>
      </form>
    </div>
  )
}
