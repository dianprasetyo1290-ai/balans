import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import "./index.css"
import { AppProviders } from "@/app/providers"
import { initSentry } from "@/app/sentry"
import { configureZodIndonesian } from "@/lib/zod/error-map"

initSentry()
configureZodIndonesian()

const root = document.getElementById("root")
if (!root) {
  throw new Error("Elemen #root tidak ditemukan")
}

createRoot(root).render(
  <StrictMode>
    <AppProviders />
  </StrictMode>,
)
