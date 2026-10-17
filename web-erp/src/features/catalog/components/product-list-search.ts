import { z } from "zod"

/** Skema search URL daftar produk: filter, sort, dan halaman disimpan di URL (Technical Guideline 10.2). */
export const productListSearchSchema = z.object({
  page: z.number().int().min(1).catch(1),
  sort: z.string().optional(),
  q: z.string().optional(),
  productType: z
    .enum(["finished_good", "packaging", "reprocess_material"])
    .optional()
    .catch(undefined),
  categoryId: z.string().optional(),
  isActive: z.boolean().optional().catch(undefined),
  deleted: z.boolean().optional().catch(undefined),
})
