import Big from "big.js"

const rupiah = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  maximumFractionDigits: 0,
  minimumFractionDigits: 0,
})

const grouping = new Intl.NumberFormat("id-ID", { maximumFractionDigits: 0 })

/** Format nilai uang (string desimal dari API) menjadi `Rp1.250.000` (Technical Guideline 10.9). */
export function formatRupiah(value: string | null | undefined): string {
  if (value === null || value === undefined || value.trim() === "") {
    return "-"
  }
  try {
    // Pembulatan half-up ke rupiah penuh, sama dengan backend (Technical Guideline 5.8.6).
    const rounded = new Big(value).round(0, Big.roundHalfUp)
    return rupiah.format(Number(rounded.toFixed(0))).replace(/\u00a0/g, "")
  } catch {
    return "-"
  }
}

const unitCostFormat = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 4,
})

/** Format HPP per unit (4 desimal, Technical Guideline 5.8.6): `12500.3333` → `Rp12.500,3333`. */
export function formatUnitCost(value: string | null | undefined): string {
  if (value === null || value === undefined || value.trim() === "") {
    return "-"
  }
  try {
    return unitCostFormat.format(Number(new Big(value).toFixed(4))).replace(/\u00a0/g, "")
  } catch {
    return "-"
  }
}

/** Format angka bulat rupiah dengan pemisah ribuan untuk input: `1250000.00` → `1.250.000`. */
export function formatMoneyInput(value: string): string {
  if (value === "") {
    return ""
  }
  try {
    return grouping.format(Number(new Big(value).round(0, Big.roundHalfUp).toFixed(0)))
  } catch {
    return ""
  }
}

/**
 * Membaca input pengguna berformat Indonesia (`1.250.000`, `Rp 1.250.000`) menjadi string desimal
 * kontrak API (`"1250000.00"`). Nilai uang DILARANG dikirim sebagai number JavaScript.
 */
export function parseMoneyInput(input: string): string | null {
  const digits = input.replace(/[^\d,-]/g, "")
  if (digits === "" || digits === "-") {
    return null
  }
  const [whole, fraction = ""] = digits.split(",")
  try {
    return new Big(`${whole}.${fraction.slice(0, 2) || "0"}`).toFixed(2)
  } catch {
    return null
  }
}
