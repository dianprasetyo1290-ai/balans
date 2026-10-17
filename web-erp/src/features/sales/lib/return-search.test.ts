import { describe, expect, it } from "vitest"

import {
  creditNoteListSearchSchema,
  returnFieldSearchSchema,
  returnListSearchSchema,
} from "./return-search"

describe("filter Retur Penjualan & Nota Kredit", () => {
  it("menerima status retur MT dan tugas pickup", () => {
    expect(
      returnListSearchSchema.parse({
        page: 2,
        status: "verified",
        assignedToMe: true,
      })
    ).toEqual({ page: 2, status: "verified", assignedToMe: true })
  })

  it("mengabaikan status tidak dikenal", () => {
    expect(returnListSearchSchema.parse({ page: 0, status: "x" })).toEqual({
      page: 1,
      status: undefined,
    })
    expect(
      creditNoteListSearchSchema.parse({ page: 1, status: "awaiting_payment" })
        .status
    ).toBe("awaiting_payment")
  })

  it("mode form lapangan default terima barang", () => {
    expect(returnFieldSearchSchema.parse({}).mode).toBe("receive")
    expect(returnFieldSearchSchema.parse({ mode: "pickup" }).mode).toBe(
      "pickup"
    )
  })
})
