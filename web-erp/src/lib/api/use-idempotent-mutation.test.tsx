import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"

import { useIdempotentMutation } from "./use-idempotent-mutation"

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

describe("useIdempotentMutation", () => {
  it("memakai key yang sama saat Coba lagi dan key baru setelah sukses", async () => {
    const keys: string[] = []
    let fail = true
    const mutationFn = vi.fn(async (_v: void, key: string) => {
      keys.push(key)
      if (fail) {
        throw new TypeError("network")
      }
      return "ok"
    })

    const { result } = renderHook(() => useIdempotentMutation({ mutationFn, onError: () => undefined }), { wrapper })

    await act(async () => {
      await result.current.mutateAsync().catch(() => undefined)
    })
    fail = false
    await act(async () => {
      await result.current.mutateAsync()
    })
    await act(async () => {
      await result.current.mutateAsync()
    })

    await waitFor(() => expect(keys).toHaveLength(3))
    expect(keys[0]).toBe(keys[1])
    expect(keys[2]).not.toBe(keys[1])
  })

  it("resetKey membuat key baru untuk submit berikutnya", async () => {
    const keys: string[] = []
    const mutationFn = vi.fn(async (_v: void, key: string) => {
      keys.push(key)
      throw new TypeError("network")
    })
    const { result } = renderHook(() => useIdempotentMutation({ mutationFn, onError: () => undefined }), { wrapper })

    await act(async () => {
      await result.current.mutateAsync().catch(() => undefined)
    })
    act(() => result.current.resetKey())
    await act(async () => {
      await result.current.mutateAsync().catch(() => undefined)
    })

    expect(keys[0]).not.toBe(keys[1])
  })
})
