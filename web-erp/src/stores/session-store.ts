import { create } from "zustand"

type SessionState = {
  accessToken: string | null
  expiresAt: number | null
  setToken: (token: string, expiresAt: string) => void
  clear: () => void
}

/**
 * Access token disimpan di memori saja (Technical Guideline 9.1). DILARANG localStorage/sessionStorage.
 * Saat halaman dimuat ulang, token diambil lagi lewat refresh token di cookie HttpOnly.
 */
export const useSessionStore = create<SessionState>((set) => ({
  accessToken: null,
  expiresAt: null,
  setToken: (accessToken, expiresAt) =>
    set({ accessToken, expiresAt: Date.parse(expiresAt) }),
  clear: () => set({ accessToken: null, expiresAt: null }),
}))
