import Big from "big.js"

export type DiscountKind = "percent" | "amount"

function toBig(value: string | undefined | null): Big {
  try {
    return !value || value.trim() === "" ? new Big(0) : new Big(value)
  } catch {
    return new Big(0)
  }
}

/** Nominal diskon dari dasar (bruto baris atau subtotal), dibulatkan half-up 2 desimal seperti backend. */
export function discountAmount(
  base: string,
  kind: DiscountKind,
  value: string
): string {
  const b = toBig(base)
  const v = toBig(value)
  if (v.lte(0)) return "0.00"
  if (kind === "percent") {
    return b.times(v).div(100).round(2, Big.roundHalfUp).toFixed(2)
  }
  return (v.gt(b) ? b : v).round(2, Big.roundHalfUp).toFixed(2)
}

export type PreviewLine = {
  qty: string
  unitPrice: string
  discountKind: DiscountKind
  discountValue: string
}

export type OrderPreview = {
  gross: string
  lineDiscount: string
  subtotal: string
  documentDiscount: string
  shipping: string
  total: string
}

/** Preview total pesanan (NON-PKP tanpa PPN). Total final dihitung backend. */
export function orderPreview(
  lines: PreviewLine[],
  doc: { kind: DiscountKind; value: string },
  shipping: string
): OrderPreview {
  let gross = new Big(0)
  let lineDiscount = new Big(0)
  for (const l of lines) {
    const g = toBig(l.qty).times(toBig(l.unitPrice)).round(2, Big.roundHalfUp)
    gross = gross.plus(g)
    lineDiscount = lineDiscount.plus(
      discountAmount(g.toFixed(2), l.discountKind, l.discountValue)
    )
  }
  const subtotal = gross.minus(lineDiscount)
  const documentDiscount = new Big(
    discountAmount(subtotal.toFixed(2), doc.kind, doc.value)
  )
  const ship = toBig(shipping)
  return {
    gross: gross.toFixed(2),
    lineDiscount: lineDiscount.toFixed(2),
    subtotal: subtotal.toFixed(2),
    documentDiscount: documentDiscount.toFixed(2),
    shipping: ship.toFixed(2),
    total: subtotal.minus(documentDiscount).plus(ship).toFixed(2),
  }
}

/** Jumlah alokasi pembayaran; dipakai untuk membandingkan dengan nominal pembayaran. */
export function sumAmounts(values: string[]): string {
  return values.reduce((s, v) => s.plus(toBig(v)), new Big(0)).toFixed(2)
}

/** Isi kolom alokasi berurutan (jatuh tempo terdekat dulu) sampai nominal pembayaran habis. */
export function autoAllocate(
  amount: string,
  outstanding: { id: string; outstandingAmount: string }[]
): Record<string, string> {
  let rest = toBig(amount)
  const out: Record<string, string> = {}
  for (const inv of outstanding) {
    if (rest.lte(0)) break
    const open = toBig(inv.outstandingAmount)
    const take = rest.gt(open) ? open : rest
    out[inv.id] = take.toFixed(2)
    rest = rest.minus(take)
  }
  return out
}

/** Hari lewat jatuh tempo → label umur piutang. */
export function overdueLabel(days: number): string {
  if (days <= 0) return "Belum jatuh tempo"
  return `Lewat ${days} hari`
}
