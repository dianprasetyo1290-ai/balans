import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { LoadingButton } from "@/components/common/loading-button"
import { MoneyInput } from "@/components/common/money-input"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { rolesQuery } from "@/features/identity/api/identity-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  createPolicy,
  updatePolicy,
  type ApprovalPolicy,
  type ApprovalSubjectType,
  type ApprovalUserOption,
} from "../api/approval-api"
import { UserPicker } from "./user-picker"

type PolicyFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  subjectTypes: ApprovalSubjectType[]
  /** Kosong = level baru. */
  policy?: ApprovalPolicy
  /** Jenis dokumen awal untuk level baru. */
  defaultSubjectType?: string
}

type Field =
  | "subjectType"
  | "level"
  | "minAmount"
  | "maxAmount"
  | "condition"
  | "approverRole"
  | "approverUserId"
  | "substituteUserId"

const NO_CONDITION = "none"

function option(
  id: string | null | undefined,
  name: string | null | undefined
): ApprovalUserOption | null {
  return id ? { id, fullName: name ?? "", email: "" } : null
}

/**
 * Tambah atau ubah level Matriks Persetujuan. Perubahan membuat versi baru yang berlaku mulai sekarang;
 * dokumen yang sudah diajukan tetap memakai approver versi lama (Technical Guideline 5.14).
 */
export function PolicyFormDialog(props: PolicyFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <PolicyForm {...props} /> : null}
    </Dialog>
  )
}

function PolicyForm({
  onOpenChange,
  subjectTypes,
  policy,
  defaultSubjectType,
}: PolicyFormDialogProps) {
  const queryClient = useQueryClient()
  const roles = useQuery(rolesQuery)
  const [subjectType, setSubjectType] = useState(
    policy?.subjectType ?? defaultSubjectType ?? ""
  )
  const [level, setLevel] = useState(String(policy?.level ?? 1))
  const [minAmount, setMinAmount] = useState(policy?.minAmount ?? "")
  const [maxAmount, setMaxAmount] = useState(policy?.maxAmount ?? "")
  const [condition, setCondition] = useState(policy?.condition || NO_CONDITION)
  const [role, setRole] = useState(policy?.approverRole ?? "")
  const [approver, setApprover] = useState(() =>
    option(policy?.approverUserId, policy?.approverUserName)
  )
  const [substitute, setSubstitute] = useState(() =>
    option(policy?.substituteUserId, policy?.substituteUserName)
  )
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({})

  const info = subjectTypes.find((t) => t.type === subjectType)

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) => {
      const fields = {
        minAmount: minAmount || null,
        maxAmount: maxAmount || null,
        condition: condition === NO_CONDITION ? "" : condition,
        approverRole: role,
        approverUserId: approver?.id ?? null,
        substituteUserId: substitute?.id ?? null,
      }
      return policy
        ? updatePolicy(policy.id, { ...fields, version: policy.version }, key)
        : createPolicy({ ...fields, subjectType, level: Number(level) }, key)
    },
    onSuccess: async () => {
      toast.success(
        policy
          ? "Kebijakan diperbarui dan berlaku mulai sekarang"
          : "Level persetujuan ditambahkan"
      )
      await queryClient.invalidateQueries({
        queryKey: ["approval", "policies"],
      })
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.code === "VALIDATION_FAILED" && problem.fields) {
        setErrors(problem.fields as Partial<Record<Field, string>>)
        return
      }
      showErrorDialog(problem)
    },
  })

  const submit = () => {
    const next: Partial<Record<Field, string>> = {}
    if (!subjectType) next.subjectType = "Jenis dokumen wajib diisi"
    const lv = Number(level)
    if (!Number.isInteger(lv) || lv < 1 || lv > 9)
      next.level = "Level harus 1 sampai 9"
    if (!role) next.approverRole = "Jabatan approver wajib diisi"
    if (minAmount && maxAmount && Number(maxAmount) <= Number(minAmount))
      next.maxAmount = "Batas atas harus lebih besar dari batas bawah"
    setErrors(next)
    if (Object.keys(next).length === 0) {
      save.mutate()
    }
  }

  const err = (k: Field) => (errors[k] ? [{ message: errors[k] }] : [])
  const roleName = (code: string) =>
    roles.data?.find((r) => r.code === code)?.name ?? code

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>
          {policy
            ? `Ubah ${policy.subjectTypeLabel} Level ${policy.level}`
            : "Tambah Level Persetujuan"}
        </DialogTitle>
        <DialogDescription>
          {policy
            ? "Versi baru berlaku mulai sekarang. Dokumen yang sudah diajukan tetap memakai approver lama."
            : "Level berlaku untuk dokumen yang diajukan mulai sekarang."}
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
      >
        <FieldGroup>
          <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
            <Field data-invalid={!!errors.subjectType}>
              <FieldLabel htmlFor="policy-subject">Jenis dokumen</FieldLabel>
              <Select
                value={subjectType}
                disabled={!!policy}
                onValueChange={(v) => {
                  setSubjectType(String(v ?? ""))
                  setCondition(NO_CONDITION)
                }}
              >
                <SelectTrigger
                  id="policy-subject"
                  className="w-full"
                  aria-invalid={!!errors.subjectType}
                >
                  <SelectValue>
                    {(v: string) =>
                      subjectTypes.find((t) => t.type === v)?.label ??
                      "Pilih jenis dokumen"
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {subjectTypes.map((t) => (
                    <SelectItem key={t.type} value={t.type}>
                      {t.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError errors={err("subjectType")} />
            </Field>
            <Field data-invalid={!!errors.level}>
              <FieldLabel htmlFor="policy-level">Level</FieldLabel>
              <Input
                id="policy-level"
                type="number"
                inputMode="numeric"
                min={1}
                max={9}
                disabled={!!policy}
                value={level}
                aria-invalid={!!errors.level}
                onChange={(e) => setLevel(e.target.value)}
              />
              <FieldError errors={err("level")} />
            </Field>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.minAmount}>
              <FieldLabel htmlFor="policy-min">
                Berlaku jika {info?.amountLabel.toLowerCase() ?? "nominal"}{" "}
                lebih dari
              </FieldLabel>
              <MoneyInput
                id="policy-min"
                value={minAmount}
                onValueChange={setMinAmount}
                placeholder="Semua nominal"
                aria-invalid={!!errors.minAmount}
              />
              <FieldError errors={err("minAmount")} />
            </Field>
            <Field data-invalid={!!errors.maxAmount}>
              <FieldLabel htmlFor="policy-max">
                Dan paling besar (opsional)
              </FieldLabel>
              <MoneyInput
                id="policy-max"
                value={maxAmount}
                onValueChange={setMaxAmount}
                placeholder="Tanpa batas"
                aria-invalid={!!errors.maxAmount}
              />
              <FieldError errors={err("maxAmount")} />
            </Field>
          </div>
          {info && info.conditions.length > 0 ? (
            <Field data-invalid={!!errors.condition}>
              <FieldLabel htmlFor="policy-condition">
                Syarat tambahan
              </FieldLabel>
              <Select
                value={condition}
                onValueChange={(v) => setCondition(String(v ?? NO_CONDITION))}
              >
                <SelectTrigger id="policy-condition" className="w-full">
                  <SelectValue>
                    {(v: string) =>
                      info.conditions.find((c) => c.code === v)?.label ??
                      "Tanpa syarat"
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_CONDITION}>Tanpa syarat</SelectItem>
                  {info.conditions.map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      {c.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FieldError errors={err("condition")} />
            </Field>
          ) : null}
          <Field data-invalid={!!errors.approverRole}>
            <FieldLabel htmlFor="policy-role">Jabatan approver</FieldLabel>
            <Select
              value={role}
              onValueChange={(v) => setRole(String(v ?? ""))}
            >
              <SelectTrigger
                id="policy-role"
                className="w-full"
                aria-invalid={!!errors.approverRole}
              >
                <SelectValue>
                  {(v: string) => (v ? roleName(v) : "Pilih jabatan")}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {(roles.data ?? []).map((r) => (
                  <SelectItem key={r.code} value={r.code}>
                    {r.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldError errors={err("approverRole")} />
          </Field>
          <Field data-invalid={!!errors.approverUserId}>
            <FieldLabel htmlFor="policy-user">
              Approver khusus (opsional)
            </FieldLabel>
            <UserPicker
              id="policy-user"
              value={approver}
              onValueChange={setApprover}
              placeholder="Semua pemegang jabatan"
              clearable
              aria-invalid={!!errors.approverUserId}
            />
            <FieldDescription>
              Kosongkan agar siapa pun yang memegang jabatan ini bisa
              menyetujui.
            </FieldDescription>
            <FieldError errors={err("approverUserId")} />
          </Field>
          <Field data-invalid={!!errors.substituteUserId}>
            <FieldLabel htmlFor="policy-substitute">Pengganti</FieldLabel>
            <UserPicker
              id="policy-substitute"
              value={substitute}
              onValueChange={setSubstitute}
              placeholder="Belum diatur"
              clearable
              excludeIds={approver ? [approver.id] : []}
              aria-invalid={!!errors.substituteUserId}
            />
            <FieldDescription>
              Menyetujui level ini jika pembuat dokumen sendiri memegang jabatan
              approver.
            </FieldDescription>
            <FieldError errors={err("substituteUserId")} />
          </Field>
        </FieldGroup>
        <DialogFooter className="mt-6">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            Batal
          </Button>
          <LoadingButton
            type="submit"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Simpan
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
