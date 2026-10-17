import Big from "big.js"

/** Baris form Jurnal Umum: nilai uang sebagai string desimal. */
export type JournalLineDraft = { debit: string; credit: string }

function toBig(value: string): Big {
  try {
    return value.trim() === "" ? new Big(0) : new Big(value)
  } catch {
    return new Big(0)
  }
}

/** Total debit, kredit, dan selisih (debit − kredit) sebagai string desimal 2 digit. */
export function journalTotals(lines: JournalLineDraft[]) {
  let debit = new Big(0)
  let credit = new Big(0)
  for (const l of lines) {
    debit = debit.plus(toBig(l.debit))
    credit = credit.plus(toBig(l.credit))
  }
  const difference = debit.minus(credit)
  return {
    debit: debit.toFixed(2),
    credit: credit.toFixed(2),
    difference: difference.toFixed(2),
    balanced: difference.eq(0) && debit.gt(0),
  }
}
