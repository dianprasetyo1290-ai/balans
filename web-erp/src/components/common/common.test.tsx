import { act, render, renderHook, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { useDelayedVisibility } from "@/hooks/use-delayed-visibility"

import { CircularProgress } from "./circular-progress"
import { LoadingButton } from "./loading-button"
import { MoneyInput } from "./money-input"
import { StatusBadge } from "./status-badge"

describe("CircularProgress", () => {
  it("mode persentase memakai aria-valuenow", () => {
    render(<CircularProgress value={42.4} label="Mengimpor" showValue />)
    const bar = screen.getByRole("progressbar", { name: "Mengimpor" })
    expect(bar).toHaveAttribute("aria-valuenow", "42")
    expect(screen.getByText("42%")).toBeInTheDocument()
  })

  it("mode indeterminate tanpa nilai", () => {
    render(<CircularProgress />)
    expect(screen.getByRole("progressbar", { name: "Memuat" })).not.toHaveAttribute("aria-valuenow")
  })
})

describe("useDelayedVisibility", () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it("tidak tampil untuk proses di bawah 300 ms", () => {
    const { result, rerender } = renderHook(({ loading }) => useDelayedVisibility(loading), { initialProps: { loading: true } })
    act(() => vi.advanceTimersByTime(299))
    rerender({ loading: false })
    act(() => vi.advanceTimersByTime(1000))
    expect(result.current).toBe(false)
  })

  it("tampil setelah 300 ms dan bertahan minimal 500 ms", () => {
    const { result, rerender } = renderHook(({ loading }) => useDelayedVisibility(loading), { initialProps: { loading: true } })
    act(() => vi.advanceTimersByTime(300))
    expect(result.current).toBe(true)
    rerender({ loading: false })
    act(() => vi.advanceTimersByTime(499))
    expect(result.current).toBe(true)
    act(() => vi.advanceTimersByTime(1))
    expect(result.current).toBe(false)
  })
})

describe("LoadingButton", () => {
  it("langsung di-disable dan mengganti label saat loading", () => {
    render(
      <LoadingButton loading loadingText="Menyimpan…">
        Simpan
      </LoadingButton>,
    )
    const button = screen.getByRole("button", { name: /Menyimpan/ })
    expect(button).toBeDisabled()
    expect(button).toHaveAttribute("aria-busy", "true")
  })
})

describe("MoneyInput", () => {
  function Harness({ onChange }: { onChange: (v: string) => void }) {
    const [value, setValue] = useState("")
    return (
      <MoneyInput
        aria-label="Nominal"
        value={value}
        onValueChange={(v) => {
          setValue(v)
          onChange(v)
        }}
      />
    )
  }

  it("mengirim string desimal dan menampilkan pemisah ribuan", async () => {
    const onChange = vi.fn()
    render(<Harness onChange={onChange} />)
    const input = screen.getByLabelText("Nominal")

    await userEvent.type(input, "1250000")

    expect(onChange).toHaveBeenLastCalledWith("1250000.00")
    expect(input).toHaveValue("1.250.000")
  })
})

describe("StatusBadge", () => {
  it("menampilkan label Bahasa Indonesia", () => {
    render(<StatusBadge status="partially_received" />)
    expect(screen.getByText("Diterima Sebagian")).toHaveAttribute("data-status", "partially_received")
  })
})
