import type { Account } from "../api/accounting-api"

export type Row = { account: Account; depth: number }

/** Urutan pohon: induk diikuti sub-akunnya, masing-masing urut kode. */
export function accountTree(accounts: Account[]): Row[] {
  const children = new Map<string | null, Account[]>()
  const ids = new Set(accounts.map((a) => a.id))
  for (const a of accounts) {
    const parent = a.parentId && ids.has(a.parentId) ? a.parentId : null
    children.set(parent, [...(children.get(parent) ?? []), a])
  }
  const rows: Row[] = []
  const walk = (parent: string | null, depth: number) => {
    const list = [...(children.get(parent) ?? [])].sort((a, b) =>
      a.code.localeCompare(b.code)
    )
    for (const a of list) {
      rows.push({ account: a, depth })
      walk(a.id, depth + 1)
    }
  }
  walk(null, 0)
  return rows
}
