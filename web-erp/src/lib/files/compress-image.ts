/** Target ukuran foto dari HP sebelum upload (Technical Guideline 5.16). */
export const MAX_IMAGE_BYTES = 1024 * 1024
const MAX_DIMENSION = 1920
const QUALITY_STEPS = [0.85, 0.75, 0.65, 0.55, 0.45]

function loadImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      resolve(img)
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error("Gambar tidak dapat dibaca"))
    }
    img.src = url
  })
}

function toBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, "image/jpeg", quality))
}

/** Memperkecil dan mengompres foto ke JPEG ±1 MB. File non-gambar dan gambar kecil dikembalikan apa adanya. */
export async function compressImage(file: File): Promise<File> {
  if (!file.type.startsWith("image/") || file.size <= MAX_IMAGE_BYTES) {
    return file
  }
  const img = await loadImage(file)
  const scale = Math.min(1, MAX_DIMENSION / Math.max(img.width, img.height))
  const canvas = document.createElement("canvas")
  canvas.width = Math.round(img.width * scale)
  canvas.height = Math.round(img.height * scale)
  const ctx = canvas.getContext("2d")
  if (!ctx) {
    return file
  }
  ctx.drawImage(img, 0, 0, canvas.width, canvas.height)

  let best: Blob | null = null
  for (const quality of QUALITY_STEPS) {
    best = await toBlob(canvas, quality)
    if (best && best.size <= MAX_IMAGE_BYTES) {
      break
    }
  }
  if (!best) {
    return file
  }
  const name = file.name.replace(/\.[^.]+$/, "") + ".jpg"
  return new File([best], name, { type: "image/jpeg", lastModified: file.lastModified })
}
