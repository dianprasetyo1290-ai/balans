import * as Sentry from "@sentry/react"

/** Image web-erp yang sama dipakai staging dan production, jadi lingkungan dibaca dari host. */
function environmentFromHost(hostname: string): string {
  if (hostname.startsWith("erp-staging.")) {
    return "staging"
  }
  return hostname.startsWith("erp.") ? "production" : "local"
}

/**
 * Error tracking ERP (Technical Guideline 13). Aktif hanya jika VITE_SENTRY_DSN diisi saat build.
 * Data pribadi tidak dikirim: tanpa IP, cookie, header, query string, dan breadcrumb input.
 */
export function initSentry() {
  const dsn = import.meta.env.VITE_SENTRY_DSN
  if (!dsn) {
    return
  }
  Sentry.init({
    dsn,
    environment: environmentFromHost(window.location.hostname),
    release: `balans-web-erp@${import.meta.env.VITE_APP_VERSION ?? "dev"}`,
    beforeSend(event) {
      if (event.request) {
        delete event.request.cookies
        delete event.request.headers
        delete event.request.query_string
        delete event.request.data
      }
      event.user = event.user?.id ? { id: event.user.id } : undefined
      return event
    },
    beforeBreadcrumb(breadcrumb) {
      return breadcrumb.category === "ui.input" ? null : breadcrumb
    },
  })
}
