import { expect, test } from "@playwright/test"

// Smoke test walking skeleton: ERP → Caddy → api → Postgres (F0-TEK-06) dan alur masuk (S-101).
// Login penuh dijalankan jika E2E_EMAIL dan E2E_PASSWORD tersedia (akun khusus smoke test).
const email = process.env.E2E_EMAIL
const password = process.env.E2E_PASSWORD

test("halaman yang butuh sesi diarahkan ke halaman Masuk", async ({ page }) => {
  await page.goto("/system/about")

  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByLabel("Email")).toBeVisible()
})

test("password salah menampilkan modal Gagal masuk", async ({ page }) => {
  await page.goto("/login")
  await page.getByLabel("Email").fill("tidak.terdaftar@balans.local")
  await page.getByLabel("Password").fill("password-salah-sekali")
  await page.getByRole("button", { name: "Masuk" }).click()

  await expect(page.getByRole("alertdialog")).toContainText("Email atau password salah.")
})

test("masuk lalu membuka Tentang Sistem dan keluar", async ({ page }) => {
  test.skip(!email || !password, "E2E_EMAIL/E2E_PASSWORD tidak diset")
  await page.goto("/system/about")
  await page.getByLabel("Email").fill(email ?? "")
  await page.getByLabel("Password").fill(password ?? "")
  await page.getByRole("button", { name: "Masuk" }).click()

  await expect(page.getByRole("heading", { name: "Tentang Sistem" })).toBeVisible()
  await expect(page.getByTestId("readiness")).toHaveText("Siap")

  await page.reload()
  await expect(page.getByRole("heading", { name: "Tentang Sistem" })).toBeVisible()

  const userMenu = page.getByRole("button", { name: /@/ })
  if (!(await userMenu.isVisible())) {
    await page.getByRole("button", { name: "Buka atau tutup menu" }).click()
  }
  await userMenu.click()
  await page.getByRole("menuitem", { name: "Keluar" }).click()
  await expect(page).toHaveURL(/\/login/)
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
