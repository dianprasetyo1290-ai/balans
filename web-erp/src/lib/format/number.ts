import Big from "big.js"

const QUANTITY_DECIMALS = 3

const quantityFormat = new Intl.NumberFormat("id-ID", { maximumFractionDigits: QUANTITY_DECIMALS })

/** Format kuantitas (string desimal API, 3 desimal) dengan format Indonesia: `1250.5` → `1.250,5`. */
export function formatQuantity(value: string | null | undefined): string {
  if (value === null || value === undefined || value.trim() === "") {
    return "-"
  }
  try {
    return quantityFormat.format(Number(new Big(value).toFixed(QUANTITY_DECIMALS)))
  } catch {
    return "-"
  }
}

/** Membaca input kuantitas Indonesia (`1.250,5`) menjadi string desimal API (`"1250.500"`). */
export function parseQuantityInput(input: string): string | null {
  const normalized = input.replace(/\./g, "").replace(",", ".").replace(/[^\d.-]/g, "")
  if (normalized === "" || normalized === "-" || normalized === ".") {
    return null
  }
  try {
    return new Big(normalized).toFixed(QUANTITY_DECIMALS)
  } catch {
    return null
  }
}

/** Format persentase: `11.5` → `11,5%`. */
export function formatPercent(value: string | null | undefined): string {
  if (value === null || value === undefined || value.trim() === "") {
    return "-"
  }
  try {
    return `${new Intl.NumberFormat("id-ID", { maximumFractionDigits: 4 }).format(Number(new Big(value)))}%`
  } catch {
    return "-"
  }
}
