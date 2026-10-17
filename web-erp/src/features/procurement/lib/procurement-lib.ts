import Big from "big.js"

function toBig(value: string): Big {
  try {
    return value.trim() === "" ? new Big(0) : new Big(value)
  } catch {
    return new Big(0)
  }
}

/** Jumlah baris = qty × harga, dibulatkan half-up ke 2 desimal (sama dengan backend). */
export function lineAmount(qty: string, price: string): string {
  return toBig(qty).times(toBig(price)).round(2, Big.roundHalfUp).toFixed(2)
}

/** Total preview dokumen dari baris (total final dihitung backend; NON-PKP tanpa PPN). */
export function documentTotal(lines: { qty: string; price: string }[]): string {
  return lines
    .reduce((sum, l) => sum.plus(lineAmount(l.qty, l.price)), new Big(0))
    .toFixed(2)
}

const ONES = [
  "",
  "satu",
  "dua",
  "tiga",
  "empat",
  "lima",
  "enam",
  "tujuh",
  "delapan",
  "sembilan",
  "sepuluh",
  "sebelas",
]

function below1000(n: number): string {
  if (n < 12) return ONES[n]
  if (n < 20) return `${ONES[n - 10]} belas`
  if (n < 100) {
    const rest = n % 10
    return `${ONES[Math.floor(n / 10)]} puluh${rest ? ` ${ONES[rest]}` : ""}`
  }
  const hundreds = Math.floor(n / 100)
  const rest = n % 100
  const head = hundreds === 1 ? "seratus" : `${ONES[hundreds]} ratus`
  return rest ? `${head} ${below1000(rest)}` : head
}

const SCALES = ["", "ribu", "juta", "miliar", "triliun"]

/** Terbilang Bahasa Indonesia untuk nominal rupiah penuh, mis. 1.250.000 → "satu juta dua ratus lima puluh ribu rupiah". */
export function terbilang(value: string): string {
  let n: Big
  try {
    n = new Big(value).round(0, Big.roundHalfUp).abs()
  } catch {
    return ""
  }
  if (n.eq(0)) return "nol rupiah"
  const groups: number[] = []
  let digits = n.toFixed(0)
  while (digits.length > 0) {
    groups.unshift(Number(digits.slice(-3)))
    digits = digits.slice(0, -3)
  }
  const words: string[] = []
  groups.forEach((g, i) => {
    if (g === 0) return
    const scale = SCALES[groups.length - 1 - i] ?? ""
    if (g === 1 && scale === "ribu") {
      words.push("seribu")
    } else {
      words.push([below1000(g), scale].filter(Boolean).join(" "))
    }
  })
  return `${words.join(" ")} rupiah`
}
