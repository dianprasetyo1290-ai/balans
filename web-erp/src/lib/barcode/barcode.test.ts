import { describe, expect, it } from "vitest"

import {
  barRuns,
  CODE128_PATTERNS,
  code128Modules,
  code128Values,
  detectSymbology,
  EAN_TABLES,
  ean13CheckDigit,
  ean13Modules,
  isValidEan13,
} from "./barcode"

const sum = (s: string) => [...s].reduce((n, c) => n + Number(c), 0)

describe("Code 128", () => {
  it("tabel pola lengkap, unik, dan tiap simbol 11 modul (stop 13)", () => {
    expect(CODE128_PATTERNS).toHaveLength(107)
    expect(new Set(CODE128_PATTERNS).size).toBe(107)
    CODE128_PATTERNS.slice(0, 106).forEach((p, i) => {
      expect(sum(p), `simbol ${i}`).toBe(11)
      expect(p).toHaveLength(6)
    })
    expect(sum(CODE128_PATTERNS[106])).toBe(13)
  })

  it("teks memakai subset B dengan checksum modulo 103", () => {
    // Start B (104), "A" = 33, "B" = 34 → (104 + 33·1 + 34·2) mod 103 = 102.
    expect(code128Values("AB")).toEqual([104, 33, 34, 102, 106])
  })

  it("digit genap memakai subset C", () => {
    // Start C (105), 12, 34 → (105 + 12 + 68) mod 103 = 82.
    expect(code128Values("1234")).toEqual([105, 12, 34, 82, 106])
    expect(code128Values("12")[0]).toBe(105)
  })

  it("pindah subset untuk deretan digit panjang", () => {
    const v = code128Values("MS123456")
    expect(v.slice(0, 4)).toEqual([104, 45, 51, 99])
    expect(v.slice(4, 7)).toEqual([12, 34, 56])
    // Digit ganjil di tengah: satu digit di subset B lalu pindah ke C.
    expect(code128Values("A1234567B").slice(0, 4)).toEqual([104, 33, 17, 99])
    // Diawali 4 digit lalu huruf: mulai C, pindah ke B.
    expect(code128Values("1234AB").slice(0, 4)).toEqual([105, 12, 34, 100])
    // Deretan digit pendek tetap B.
    expect(code128Values("MS-COK-30")[0]).toBe(104)
  })

  it("jumlah modul = 11 per simbol + 13 stop", () => {
    const values = code128Values("MS-COK-30")
    expect(code128Modules("MS-COK-30")).toHaveLength(
      11 * (values.length - 1) + 13
    )
  })

  it("menolak teks kosong dan karakter di luar ASCII tercetak", () => {
    expect(() => code128Values("")).toThrow()
    expect(() => code128Values("Kopi É")).toThrow()
  })
})

describe("EAN-13", () => {
  it("menghitung dan memeriksa digit periksa", () => {
    expect(ean13CheckDigit("400638133393")).toBe(1)
    expect(isValidEan13("4006381333931")).toBe(true)
    expect(isValidEan13("4006381333930")).toBe(false)
    expect(isValidEan13("400638133393")).toBe(false)
    expect(detectSymbology("8991234567891")).toBe("ean13")
    expect(detectSymbology("MS-COK-30")).toBe("code128")
  })

  it("tabel R adalah komplemen L dan G adalah kebalikan R", () => {
    EAN_TABLES.L.forEach((l, d) => {
      const complement = [...l].map((b) => (b === "1" ? "0" : "1")).join("")
      expect(EAN_TABLES.R[d]).toBe(complement)
      expect(EAN_TABLES.G[d]).toBe([...EAN_TABLES.R[d]].reverse().join(""))
    })
  })

  it("95 modul dengan penjaga kiri, tengah, kanan", () => {
    const m = ean13Modules("4006381333931").map((b) => (b ? "1" : "0"))
    expect(m).toHaveLength(95)
    expect(m.slice(0, 3).join("")).toBe("101")
    expect(m.slice(45, 50).join("")).toBe("01010")
    expect(m.slice(92).join("")).toBe("101")
    // Digit kedua (0) dengan paritas L untuk digit awal 4 (LGLLGG).
    expect(m.slice(3, 10).join("")).toBe("0001101")
    expect(() => ean13Modules("123")).toThrow()
  })
})

describe("barRuns", () => {
  it("menggabungkan modul hitam berurutan", () => {
    expect(barRuns([true, true, false, true, false, false, true])).toEqual([
      [0, 2],
      [3, 1],
      [6, 1],
    ])
  })
})
