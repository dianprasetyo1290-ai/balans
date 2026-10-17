import { describe, expect, it } from "vitest"

import type { Account } from "../api/accounting-api"
import { accountTree } from "./account-tree"
import { journalTotals } from "./journal-totals"

const acc = (id: string, code: string, parentId?: string): Account => ({
  id,
  code,
  name: code,
  group: Number(code[0]),
  groupLabel: "",
  normalBalance: "debit",
  parentId: parentId ?? null,
  isPostable: !code.endsWith("00"),
  allowManualPosting: false,
  isActive: true,
  description: "",
  updatedAt: "2026-10-19T00:00:00Z",
  version: 1,
})

describe("accountTree", () => {
  it("menyusun induk lalu sub-akun urut kode dengan kedalaman", () => {
    const rows = accountTree([
      acc("c", "1111", "b"),
      acc("a", "1000"),
      acc("d", "2000"),
      acc("b", "1100", "a"),
      acc("e", "1101", "b"),
    ])
    expect(rows.map((r) => [r.account.code, r.depth])).toEqual([
      ["1000", 0],
      ["1100", 1],
      ["1101", 2],
      ["1111", 2],
      ["2000", 0],
    ])
  })

  it("akun dengan induk yang tidak tampil diletakkan di akar", () => {
    const rows = accountTree([acc("x", "1101", "hilang")])
    expect(rows[0]?.depth).toBe(0)
  })
})

describe("journalTotals", () => {
  it("menjumlahkan debit dan kredit tanpa galat floating point", () => {
    const t = journalTotals([
      { debit: "0.1", credit: "" },
      { debit: "0.2", credit: "" },
      { debit: "", credit: "0.3" },
    ])
    expect(t).toEqual({
      debit: "0.30",
      credit: "0.30",
      difference: "0.00",
      balanced: true,
    })
  })

  it("tidak balance dan nol tidak dianggap balance", () => {
    expect(journalTotals([{ debit: "100", credit: "" }]).balanced).toBe(false)
    expect(journalTotals([{ debit: "", credit: "" }]).balanced).toBe(false)
    expect(journalTotals([{ debit: "abc", credit: "5" }]).difference).toBe(
      "-5.00"
    )
  })
})
