import { z } from "zod"

/**
 * Pesan error Zod Bahasa Indonesia untuk seluruh form (Technical Guideline 10.5).
 * Locale bawaan Zod dipakai sebagai dasar; pesan yang paling sering muncul ditulis ulang
 * agar sesuai gaya bahasa ERP.
 */
export function configureZodIndonesian() {
  const base = z.locales.id().localeError

  z.config({
    localeError: (issue) => {
      switch (issue.code) {
        case "invalid_type":
          if (issue.input === undefined || issue.input === null || issue.input === "") {
            return "Wajib diisi"
          }
          return "Format tidak valid"
        case "too_small":
          if (issue.origin === "string" && issue.minimum === 1) {
            return "Wajib diisi"
          }
          if (issue.origin === "string") {
            return `Minimal ${issue.minimum} karakter`
          }
          if (issue.origin === "array") {
            return `Minimal ${issue.minimum} baris`
          }
          return issue.inclusive ? `Minimal ${issue.minimum}` : `Harus lebih dari ${issue.minimum}`
        case "too_big":
          if (issue.origin === "string") {
            return `Maksimal ${issue.maximum} karakter`
          }
          return issue.inclusive ? `Maksimal ${issue.maximum}` : `Harus kurang dari ${issue.maximum}`
        case "invalid_format":
          if (issue.format === "email") {
            return "Alamat email tidak valid"
          }
          return "Format tidak valid"
        default:
          return base?.(issue)
      }
    },
  })
}
