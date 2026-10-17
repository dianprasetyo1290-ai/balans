import { render, screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { CashBankAccount } from "../api/finance-api"
import { bankOptions, FinanceStatusBadge, yearOptions } from "./finance-shared"

const account = (patch: Partial<CashBankAccount>): CashBankAccount => ({
  id: "1",
  code: "BCA-OPS",
  name: "BCA Operasional",
  kind: "bank",
  bankName: "BCA",
  accountNumberMasked: "••••7890",
  accountHolder: "PT Balans",
  accountId: "gl",
  accountCode: "1111",
  accountName: "Bank 1",
  isActive: true,
  description: "",
  updatedAt: "2026-10-19T00:00:00Z",
  version: 1,
  allowedActions: [],
  ...patch,
})

describe("pilihan rekening", () => {
  it("hanya rekening aktif dan bisa dibatasi rekening bank", () => {
    const list = [
      account({}),
      account({
        id: "2",
        kind: "cash",
        name: "Kas Besar",
        accountNumberMasked: "",
        accountCode: "1101",
      }),
      account({ id: "3", isActive: false }),
    ]
    expect(bankOptions(list).map((o) => o.value)).toEqual(["1", "2"])
    expect(bankOptions(list, true)).toEqual([
      { value: "1", label: "BCA Operasional ••••7890 · 1111" },
    ])
    expect(bankOptions(undefined)).toEqual([])
  })

  it("tahun pajak berjalan dan tiga tahun sebelumnya", () => {
    expect(yearOptions(2026).map((o) => o.value)).toEqual([
      "2026",
      "2025",
      "2024",
      "2023",
    ])
  })
})

describe("badge status keuangan", () => {
  it("label rekonsiliasi dan bukti potong", () => {
    render(
      <>
        <FinanceStatusBadge status="suggested" />
        <FinanceStatusBadge status="recorded" />
        <FinanceStatusBadge status="posted" />
      </>
    )
    expect(screen.getByText("Ada Usulan")).toBeInTheDocument()
    expect(screen.getByText("Belum Disetor")).toBeInTheDocument()
    expect(screen.getByText("Diposting")).toBeInTheDocument()
  })
})
