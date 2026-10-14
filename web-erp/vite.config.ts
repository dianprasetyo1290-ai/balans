/// <reference types="vitest/config" />
import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig, loadEnv } from "vite"

// API dilayani dari origin yang sama lewat proxy (Technical Guideline 9.4), sehingga CORS tidak dibuka.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "")
  const apiTarget = env.API_PROXY_TARGET ?? "http://localhost:8080"
  const mailTarget = env.MAILPIT_PROXY_TARGET ?? "http://localhost:8025"

  return {
    plugins: [
      tanstackRouter({ target: "react", autoCodeSplitting: true }),
      react(),
      tailwindcss(),
    ],
    resolve: {
      alias: {
        "@": resolve(import.meta.dirname, "./src"),
      },
    },
    server: {
      proxy: {
        "/api": apiTarget,
        "/healthz": apiTarget,
        "/readyz": apiTarget,
        "/_mock/mail": mailTarget,
        "/_mock": apiTarget,
      },
    },
    test: {
      environment: "jsdom",
      setupFiles: ["./src/test/setup.ts"],
      include: ["src/**/*.test.{ts,tsx}"],
      css: false,
    },
  }
})
