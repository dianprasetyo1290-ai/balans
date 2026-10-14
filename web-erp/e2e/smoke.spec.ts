import { expect, test } from "@playwright/test"

// Smoke test walking skeleton: ERP → Caddy → api → Postgres (F0-TEK-06).
test("halaman Tentang Sistem menampilkan versi dan status siap", async ({ page }) => {
  await page.goto("/system/about")

  await expect(page.getByRole("heading", { name: "Tentang Sistem" })).toBeVisible()
  await expect(page.getByTestId("readiness")).toHaveText("Siap")
  await expect(page.getByText("Versi", { exact: true })).toBeVisible()
})

test("alamat tidak dikenal menampilkan halaman 404", async ({ page }) => {
  await page.goto("/alamat-yang-tidak-ada")

  await expect(page.getByRole("heading", { name: "Halaman tidak ditemukan" })).toBeVisible()
})

test("API health check dapat diakses lewat host ERP", async ({ request }) => {
  const res = await request.get("/readyz")
  expect(res.status()).toBe(200)
  expect(await res.json()).toMatchObject({ status: "ready" })
})
