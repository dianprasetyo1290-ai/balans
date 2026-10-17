import type { ReactNode } from "react"

import { Field, FieldError, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import type { Problem } from "@/lib/api/problem"
import { showErrorDialog } from "@/stores/error-dialog-store"

export type FieldErrors = Record<string, string>

/** Error per field untuk FieldError. */
export const fieldError = (errors: FieldErrors, key: string) =>
  errors[key] ? [{ message: errors[key] }] : []

/**
 * Menyalurkan VALIDATION_FAILED ke error per field (kunci API camelCase, mis. `addresses.0.city`);
 * error lain ke dialog error standar (Technical Guideline 10.6).
 */
export function handleFormProblem(
  problem: Problem,
  setErrors: (errors: FieldErrors) => void
) {
  if (problem.code === "VALIDATION_FAILED" && problem.fields) {
    setErrors(problem.fields)
    return
  }
  if (problem.code === "DUPLICATE_CODE") {
    setErrors({ code: problem.detail })
    return
  }
  showErrorDialog(problem)
}

type ChoiceFieldProps<T extends string> = {
  id: string
  label: string
  value: T
  onValueChange: (value: T) => void
  labels: Partial<Record<T, string>>
  error?: string
  disabled?: boolean
  children?: ReactNode
}

/** Field pilihan dari daftar enum dengan label Bahasa Indonesia. */
export function ChoiceField<T extends string>({
  id,
  label,
  value,
  onValueChange,
  labels,
  error,
  disabled,
  children,
}: ChoiceFieldProps<T>) {
  return (
    <Field data-invalid={!!error}>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Select
        value={value}
        disabled={disabled}
        onValueChange={(v) => v && onValueChange(v as T)}
      >
        <SelectTrigger id={id} className="w-full" aria-invalid={!!error}>
          <SelectValue>{(v: T) => labels[v] ?? "Pilih"}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          {(Object.keys(labels) as T[]).map((k) => (
            <SelectItem key={k} value={k}>
              {labels[k]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {children}
      <FieldError errors={error ? [{ message: error }] : []} />
    </Field>
  )
}
