import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import type { ReactNode } from "react"
import { describe, expect, it } from "vitest"

import {
  formatPercent,
  parsePercentInput,
  priceListVersionQuery,
  type PriceListDetail,
  type PriceListVersion,
} from "../api/pricing-api"
import { PriceListFormDialog } from "./price-list-form-dialog"
import { scopeLabel } from "./price-lists-page"
import { formatPeriod } from "./pricing-parts"
import { PromotionFormDialog } from "./promotion-form-dialog"
import { DiscountLimitFormDialog } from "./rule-form-dialogs"
import { VersionDialog } from "./version-dialog"

const noop = () => undefined

function wrap(node: ReactNode, client = new QueryClient()) {
  return render(
    <QueryClientProvider client={client}>{node}</QueryClientProvider>
  )
}

const summary = {
  id: "v1",
  versionNo: 1,
  status: "active" as const,
  validFrom: "2026-10-23",
  validTo: null,
  percent: null,
  itemCount: 1,
  notes: "Harga awal",
  updatedAt: "2026-10-23T02:00:00Z",
  version: 2,
}

const base: PriceListDetail = {
  id: "pl-base",
  code: "HARGA-DASAR",
  name: "Harga Dasar & HET",
  priceListType: "base",
  resellerLevel: "",
  basis: "fixed",
  description: "",
  isActive: true,
  currentVersion: summary,
  versions: [summary],
  allowedActions: ["update", "create_version"],
  updatedAt: "2026-10-23T02:00:00Z",
  version: 1,
}

const activeVersion: PriceListVersion = {
  ...summary,
  priceListId: base.id,
  priceListCode: base.code,
  priceListName: base.name,
  priceListType: "base",
  basis: "fixed",
  items: [
    {
      productId: "p1",
      productSku: "BLN-WPC-500",
      productName: "Whey 500 g",
      priceAmount: "450000.00",
      maxRetailPriceAmount: "499000.00",
    },
  ],
  allowedActions: [],
}

describe("pricing helpers", () => {
  it("memformat dan membaca persen", () => {
    expect(formatPercent("12.5000")).toBe("12,5%")
    expect(formatPercent(null)).toBe("—")
    expect(parsePercentInput(" 12,5 ")).toBe("12.5")
    expect(parsePercentInput("")).toBe("")
  })

  it("menampilkan cakupan dan periode", () => {
    const name = (id?: string | null) => (id ? `CH-${id}` : null)
    expect(scopeLabel(base, name)).toBe("Semua channel")
    expect(
      scopeLabel(
        {
          ...base,
          priceListType: "channel",
          channelId: "a",
          subChannelId: "b",
        },
        name
      )
    ).toBe("CH-a › CH-b")
    expect(
      scopeLabel(
        { ...base, priceListType: "partner", partnerName: "Alfa" },
        name
      )
    ).toBe("Alfa")
    expect(scopeLabel({ ...base, priceListType: "employee" }, name)).toMatch(
      /karyawan/
    )
    expect(formatPeriod("2026-11-01")).toMatch(/^Mulai /)
    expect(formatPeriod("2026-11-01", "2026-11-30")).toContain("–")
  })
})

describe("form harga", () => {
  it("daftar harga baru wajib kode, nama, dan channel", async () => {
    wrap(<PriceListFormDialog open onOpenChange={noop} />)
    await userEvent.click(screen.getByRole("button", { name: "Simpan" }))
    expect(
      screen.getByText("Kode daftar harga wajib diisi")
    ).toBeInTheDocument()
    expect(
      screen.getByText("Nama daftar harga wajib diisi")
    ).toBeInTheDocument()
    // Placeholder pemilih dan pesan error sama-sama "Pilih channel".
    expect(screen.getAllByText("Pilih channel")).toHaveLength(2)
  })

  it("ubah Harga Dasar & HET mengunci kode dan jenis", () => {
    wrap(<PriceListFormDialog open onOpenChange={noop} priceList={base} />)
    expect(screen.getByLabelText("Kode")).toBeDisabled()
    expect(screen.getAllByDisplayValue("Harga Dasar & HET")).toHaveLength(2)
    expect(screen.queryByText("Basis harga")).not.toBeInTheDocument()
  })

  it("versi yang berlaku tampil baca saja dengan HET", () => {
    const client = new QueryClient()
    client.setQueryData(priceListVersionQuery("v1").queryKey, activeVersion)
    wrap(
      <VersionDialog
        open
        onOpenChange={noop}
        priceList={base}
        versionId="v1"
      />,
      client
    )
    expect(screen.getByText("HARGA-DASAR v1")).toBeInTheDocument()
    expect(screen.getByText("BLN-WPC-500")).toBeInTheDocument()
    expect(screen.getByText("HET")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Simpan Draft" })
    ).not.toBeInTheDocument()
  })

  it("versi baru menyalin harga versi berlaku", () => {
    const client = new QueryClient()
    client.setQueryData(priceListVersionQuery("v1").queryKey, activeVersion)
    wrap(<VersionDialog open onOpenChange={noop} priceList={base} />, client)
    expect(screen.getByText("Versi baru HARGA-DASAR")).toBeInTheDocument()
    expect(screen.getByText(/disalin dari v1/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Simpan Draft" })).toBeEnabled()
  })

  it("promo baru wajib kode, nama, periode, dan produk", async () => {
    wrap(<PromotionFormDialog open onOpenChange={noop} />)
    await userEvent.click(screen.getByRole("button", { name: "Simpan" }))
    expect(screen.getByText("Kode promo wajib diisi")).toBeInTheDocument()
    expect(screen.getByText("Tanggal selesai wajib diisi")).toBeInTheDocument()
    expect(screen.getAllByText("Pilih produk")).toHaveLength(2)
  })

  it("batas diskon wajib role dan persen", async () => {
    wrap(<DiscountLimitFormDialog open onOpenChange={noop} />)
    await userEvent.click(screen.getByRole("button", { name: "Simpan" }))
    expect(screen.getAllByText("Pilih role")).toHaveLength(2)
    expect(screen.getByText("Batas per baris wajib diisi")).toBeInTheDocument()
  })
})
