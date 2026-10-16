import { useMutation, type UseMutationOptions } from "@tanstack/react-query"
import { useCallback, useRef } from "react"

type IdempotentMutationFn<TData, TVariables> = (
  variables: TVariables,
  idempotencyKey: string
) => Promise<TData>

type Options<TData, TError, TVariables> = Omit<
  UseMutationOptions<TData, TError, TVariables>,
  "mutationFn"
> & {
  mutationFn: IdempotentMutationFn<TData, TVariables>
}

/**
 * Mutation dengan satu Idempotency-Key per percobaan submit (Technical Guideline 10.4).
 * Key tetap sama saat user menekan "Coba lagi" setelah gagal, dan baru diganti setelah sukses
 * atau saat `resetKey` dipanggil (misalnya ketika form di-reset).
 */
export function useIdempotentMutation<TData, TError = Error, TVariables = void>(
  options: Options<TData, TError, TVariables>
) {
  const keyRef = useRef<string | null>(null)
  const { mutationFn, onSuccess, ...rest } = options

  const resetKey = useCallback(() => {
    keyRef.current = null
  }, [])

  const mutation = useMutation<TData, TError, TVariables>({
    ...rest,
    retry: false,
    mutationFn: (variables) => {
      keyRef.current ??= crypto.randomUUID()
      return mutationFn(variables, keyRef.current)
    },
    onSuccess: (...args) => {
      keyRef.current = null
      return onSuccess?.(...args)
    },
  })

  return { ...mutation, resetKey }
}
