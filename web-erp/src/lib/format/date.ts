// Tanggal ditampilkan dalam WIB (Technical Guideline 6.5, 10.9). Server dan DB tetap UTC.
export const TIME_ZONE = "Asia/Jakarta"

const dateFormat = new Intl.DateTimeFormat("id-ID", {
  day: "numeric",
  month: "short",
  year: "numeric",
  timeZone: TIME_ZONE,
})

const timeFormat = new Intl.DateTimeFormat("id-ID", {
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: TIME_ZONE,
})

const BUSINESS_DATE = /^(\d{4})-(\d{2})-(\d{2})$/

/** Membaca tanggal bisnis `YYYY-MM-DD` sebagai tengah hari WIB, agar tidak bergeser hari. */
export function parseBusinessDate(value: string): Date | null {
  const match = BUSINESS_DATE.exec(value)
  if (!match) {
    return null
  }
  const [, year, month, day] = match
  const date = new Date(`${year}-${month}-${day}T12:00:00+07:00`)
  return Number.isNaN(date.getTime()) ? null : date
}

/** `2026-10-14` → `14 Okt 2026`. */
export function formatDate(value: string | null | undefined): string {
  if (!value) {
    return "-"
  }
  const date = parseBusinessDate(value) ?? new Date(value)
  return Number.isNaN(date.getTime()) ? "-" : dateFormat.format(date)
}

/** Timestamp RFC 3339 UTC → `14 Okt 2026 09.30` (WIB). */
export function formatDateTime(value: string | null | undefined): string {
  if (!value) {
    return "-"
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return "-"
  }
  return `${dateFormat.format(date)} ${timeFormat.format(date).replace(":", ".")}`
}

/** Tanggal bisnis hari ini menurut WIB, format API `YYYY-MM-DD`. */
export function todayBusinessDate(now: Date = new Date()): string {
  return toBusinessDate(now)
}

/** Date → `YYYY-MM-DD` menurut WIB. */
export function toBusinessDate(date: Date): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    timeZone: TIME_ZONE,
  }).format(date)
  return parts
}
