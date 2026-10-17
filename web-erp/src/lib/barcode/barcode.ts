/**
 * Pembuat pola barcode untuk label produk (Desain Dokumen Cetak 2.3): EAN-13 untuk barcode 13 digit dengan
 * digit periksa benar, selain itu Code 128 (subset B dan C). Hasilnya deretan modul (true = batang hitam)
 * yang dirender sebagai SVG di browser, tanpa dependensi tambahan.
 */

export type Symbology = "ean13" | "code128"

/** Lebar batang/spasi tiap simbol Code 128 (nilai 0–105) dan stop (106). */
export const CODE128_PATTERNS = [
  "212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312", "132212", "221213",
  "221312", "231212", "112232", "122132", "122231", "113222", "123122", "123221", "223211", "221132",
  "221231", "213212", "223112", "312131", "311222", "321122", "321221", "312212", "322112", "322211",
  "212123", "212321", "232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
  "231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121", "313121", "211331",
  "231131", "213113", "213311", "213131", "311123", "311321", "331121", "312113", "312311", "332111",
  "314111", "221411", "431111", "111224", "111422", "121124", "121421", "141122", "141221", "112214",
  "112412", "122114", "122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
  "111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112", "421211", "212141",
  "214121", "412121", "111143", "111341", "131141", "114113", "114311", "411113", "411311", "113141",
  "114131", "311141", "411131", "211412", "211214", "211232", "2331112",
] // prettier-ignore

const START_B = 104
const START_C = 105
const CODE_B = 100
const CODE_C = 99
const STOP = 106

/** Zona sepi di kiri/kanan barcode dalam modul. */
export const QUIET_ZONE = 10

function isDigit(c: string) {
  return c >= "0" && c <= "9"
}

/** Panjang deretan digit mulai posisi i. */
function digitRun(s: string, i: number) {
  let n = 0
  while (i + n < s.length && isDigit(s[i + n])) {
    n++
  }
  return n
}

/**
 * Nilai simbol Code 128 (tanpa stop) untuk teks ASCII tercetak. Deretan digit panjang memakai subset C
 * (dua digit per simbol) agar barcode lebih pendek: di awal jika ≥ 4 digit, di tengah jika ≥ 6 digit
 * atau ≥ 4 digit sampai akhir teks.
 */
export function code128Values(text: string): number[] {
  if (text.length === 0) {
    throw new Error("Barcode kosong")
  }
  for (const c of text) {
    const code = c.charCodeAt(0)
    if (code < 32 || code > 126) {
      throw new Error(`Karakter tidak didukung Code 128: ${c}`)
    }
  }
  const values: number[] = []
  let i = 0
  let set: "B" | "C"
  const firstRun = digitRun(text, 0)
  if (firstRun >= 4 || (firstRun === text.length && firstRun % 2 === 0)) {
    set = "C"
    values.push(START_C)
  } else {
    set = "B"
    values.push(START_B)
  }
  while (i < text.length) {
    const run = digitRun(text, i)
    if (set === "C") {
      if (run >= 2) {
        values.push(Number(text.slice(i, i + 2)))
        i += 2
        continue
      }
      values.push(CODE_B)
      set = "B"
    }
    const toEnd = i + run === text.length
    if (run >= 6 || (run >= 4 && toEnd)) {
      if (run % 2 === 1) {
        values.push(text.charCodeAt(i) - 32)
        i++
      }
      values.push(CODE_C)
      set = "C"
      continue
    }
    values.push(text.charCodeAt(i) - 32)
    i++
  }
  const checksum =
    values.reduce((sum, v, idx) => sum + v * (idx === 0 ? 1 : idx), 0) % 103
  values.push(checksum, STOP)
  return values
}

function widthsToModules(widths: string, out: boolean[]) {
  let bar = true
  for (const w of widths) {
    for (let k = 0; k < Number(w); k++) {
      out.push(bar)
    }
    bar = !bar
  }
}

/** Modul Code 128 tanpa zona sepi. */
export function code128Modules(text: string): boolean[] {
  const out: boolean[] = []
  for (const v of code128Values(text)) {
    widthsToModules(CODE128_PATTERNS[v], out)
  }
  return out
}

const EAN_L = [
  "0001101", "0011001", "0010011", "0111101", "0100011", "0110001", "0101111", "0111011", "0110111", "0001011",
] // prettier-ignore
const EAN_G = [
  "0100111", "0110011", "0011011", "0100001", "0011101", "0111001", "0000101", "0010001", "0001001", "0010111",
] // prettier-ignore
const EAN_R = [
  "1110010", "1100110", "1101100", "1000010", "1011100", "1001110", "1010000", "1000100", "1001000", "1110100",
] // prettier-ignore
const EAN_PARITY = [
  "LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG", "LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL",
] // prettier-ignore

export const EAN_TABLES = { L: EAN_L, G: EAN_G, R: EAN_R }

/** Digit periksa EAN-13 dari 12 digit pertama. */
export function ean13CheckDigit(first12: string): number {
  let sum = 0
  for (let i = 0; i < 12; i++) {
    sum += Number(first12[i]) * (i % 2 === 1 ? 3 : 1)
  }
  return (10 - (sum % 10)) % 10
}

/** Memeriksa 13 digit dengan digit periksa benar. */
export function isValidEan13(code: string): boolean {
  return /^\d{13}$/.test(code) && ean13CheckDigit(code) === Number(code[12])
}

/** Modul EAN-13 (95 modul) tanpa zona sepi. */
export function ean13Modules(code: string): boolean[] {
  if (!isValidEan13(code)) {
    throw new Error("EAN-13 tidak valid")
  }
  const parity = EAN_PARITY[Number(code[0])]
  let bits = "101"
  for (let i = 1; i <= 6; i++) {
    const d = Number(code[i])
    bits += parity[i - 1] === "L" ? EAN_L[d] : EAN_G[d]
  }
  bits += "01010"
  for (let i = 7; i <= 12; i++) {
    bits += EAN_R[Number(code[i])]
  }
  bits += "101"
  return [...bits].map((b) => b === "1")
}

/** Simbol yang dipakai label: EAN-13 jika valid, selain itu Code 128 (sama dengan backend). */
export function detectSymbology(code: string): Symbology {
  return isValidEan13(code) ? "ean13" : "code128"
}

/** Modul barcode sesuai simbol. */
export function barcodeModules(code: string, symbology: Symbology): boolean[] {
  return symbology === "ean13" ? ean13Modules(code) : code128Modules(code)
}

/** Deretan batang hitam sebagai pasangan [posisi, lebar] dalam modul, untuk dirender SVG. */
export function barRuns(modules: boolean[]): [number, number][] {
  const runs: [number, number][] = []
  let start = -1
  modules.forEach((on, i) => {
    if (on && start < 0) {
      start = i
    }
    if (!on && start >= 0) {
      runs.push([start, i - start])
      start = -1
    }
  })
  if (start >= 0) {
    runs.push([start, modules.length - start])
  }
  return runs
}
