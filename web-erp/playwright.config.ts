import { defineConfig, devices } from "@playwright/test"

// Smoke test dijalankan terhadap staging setelah deploy (Technical Guideline 14.3).
// E2E_BASE_URL wajib diisi; kredensial basic auth staging lewat E2E_BASIC_AUTH ("user:password").
const baseURL = process.env.E2E_BASE_URL ?? "http://localhost:5173"
const basicAuth = process.env.E2E_BASIC_AUTH

export default defineConfig({
  testDir: "./e2e",
  timeout: 30_000,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
    httpCredentials: basicAuth
      ? { username: basicAuth.split(":")[0] ?? "", password: basicAuth.split(":").slice(1).join(":") }
      : undefined,
  },
  projects: [
    { name: "desktop", use: { ...devices["Desktop Chrome"] } },
    { name: "hp", use: { ...devices["Pixel 7"] } },
  ],
})
