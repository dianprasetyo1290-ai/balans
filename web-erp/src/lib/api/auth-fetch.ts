import { useSessionStore } from "@/stores/session-store"

type TokenBody = { accessToken: string; expiresAt: string }

let refreshing: Promise<boolean> | null = null

/**
 * Memperbarui access token dari cookie refresh token. Hanya satu permintaan refresh berjalan sekaligus,
 * sehingga beberapa request yang gagal bersamaan tidak merotasi token berkali-kali.
 */
export function refreshSession(): Promise<boolean> {
  refreshing ??= (async () => {
    try {
      const res = await window.fetch("/api/v1/auth/refresh", {
        method: "POST",
        credentials: "same-origin",
      })
      if (!res.ok) {
        useSessionStore.getState().clear()
        return false
      }
      const body = (await res.json()) as TokenBody
      useSessionStore.getState().setToken(body.accessToken, body.expiresAt)
      return true
    } catch {
      return false
    } finally {
      refreshing = null
    }
  })()
  return refreshing
}

/** Memastikan ada access token: dipakai saat aplikasi dimuat ulang. */
export async function ensureSession(): Promise<boolean> {
  if (useSessionStore.getState().accessToken) {
    return true
  }
  return refreshSession()
}

type ReauthHandler = () => Promise<boolean>

let reauthHandler: ReauthHandler | null = null

/** Didaftarkan ReauthDialog: membuka dialog password/kode dan resolve true jika user berhasil konfirmasi. */
export function setReauthHandler(handler: ReauthHandler | null) {
  reauthHandler = handler
}

async function errorCode(res: Response): Promise<string | undefined> {
  if (res.status !== 401) {
    return undefined
  }
  try {
    const body = (await res.clone().json()) as { code?: string }
    return body.code
  } catch {
    return undefined
  }
}

function withToken(request: Request): Request {
  const token = useSessionStore.getState().accessToken
  const req = request.clone()
  if (token) {
    req.headers.set("Authorization", `Bearer ${token}`)
  }
  return req
}

/**
 * fetch untuk client API: menyisipkan Bearer token, refresh otomatis saat token kedaluwarsa,
 * dan membuka dialog konfirmasi identitas untuk aksi sensitif lalu mengulang request yang sama
 * (Idempotency-Key tetap sama, sehingga aman diulang).
 */
export async function authFetch(input: Request): Promise<Response> {
  const original = input.clone()
  let res = await window.fetch(withToken(input))
  let code = await errorCode(res)

  if (code === "AUTH_TOKEN_EXPIRED" && (await refreshSession())) {
    res = await window.fetch(withToken(original.clone()))
    code = await errorCode(res)
  }
  if (
    code === "AUTH_REAUTH_REQUIRED" &&
    reauthHandler &&
    (await reauthHandler())
  ) {
    res = await window.fetch(withToken(original.clone()))
  }
  return res
}
