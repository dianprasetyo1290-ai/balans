import {
  QueryClient,
  QueryClientProvider,
  queryOptions,
} from "@tanstack/react-query"
import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { useState } from "react"
import { describe, expect, it, vi } from "vitest"

import { SearchPicker, type PickerOption } from "./search-picker"

type Employee = { id: string; name: string; number: string }

const data: Employee[] = [
  { id: "1", name: "Rudi Hartono", number: "BN-001" },
  { id: "2", name: "Siti Aminah", number: "BN-002" },
]

function Harness({ onChange }: { onChange: (v: PickerOption | null) => void }) {
  const [value, setValue] = useState<PickerOption | null>(null)
  const query = (q: string) =>
    queryOptions({
      queryKey: ["employees", q],
      queryFn: async () =>
        data.filter((e) => e.name.toLowerCase().includes(q.toLowerCase())),
    })
  return (
    <SearchPicker
      value={value}
      onValueChange={(v) => {
        setValue(v)
        onChange(v)
      }}
      query={query}
      toOption={(e) => ({ id: e.id, label: e.name, description: e.number })}
      placeholder="Pilih karyawan"
      searchPlaceholder="Cari karyawan"
      clearable
    />
  )
}

describe("SearchPicker", () => {
  it("mencari, memilih, dan mengosongkan pilihan", async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(
      <QueryClientProvider client={new QueryClient()}>
        <Harness onChange={onChange} />
      </QueryClientProvider>
    )

    await user.click(screen.getByRole("combobox"))
    await user.type(screen.getByLabelText("Cari karyawan"), "siti")
    await waitFor(() =>
      expect(screen.queryByText("Rudi Hartono")).not.toBeInTheDocument()
    )
    await user.click(await screen.findByText("Siti Aminah"))

    expect(onChange).toHaveBeenLastCalledWith({
      id: "2",
      label: "Siti Aminah",
      description: "BN-002",
    })
    expect(screen.getByRole("combobox")).toHaveTextContent(
      "Siti Aminah · BN-002"
    )

    await user.click(screen.getByRole("button", { name: "Kosongkan pilihan" }))
    expect(onChange).toHaveBeenLastCalledWith(null)
    expect(screen.getByRole("combobox")).toHaveTextContent("Pilih karyawan")
  })
})
