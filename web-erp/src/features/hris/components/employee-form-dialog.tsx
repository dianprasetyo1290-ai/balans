import { useQueryClient } from "@tanstack/react-query"
import { useState } from "react"
import { toast } from "sonner"

import { DateInput } from "@/components/common/date-input"
import { LoadingButton } from "@/components/common/loading-button"
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
import { Textarea } from "@/components/ui/textarea"
import {
  fieldError,
  handleFormProblem,
  type FieldErrors,
} from "@/features/party/components/form-parts"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { todayBusinessDate } from "@/lib/format/date"

import {
  createEmployee,
  resignEmployee,
  updateEmployee,
  type Employee,
  type EmployeeInput,
} from "../api/hris-api"

type EmployeeFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Kosong = karyawan baru. */
  employee?: Employee
}

/** Tambah/ubah data dasar karyawan (HR-01). NIK disimpan terenkripsi dan ditampilkan tersamar. */
export function EmployeeFormDialog(props: EmployeeFormDialogProps) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      {props.open ? <EmployeeForm {...props} /> : null}
    </Dialog>
  )
}

function EmployeeForm({ onOpenChange, employee }: EmployeeFormDialogProps) {
  const queryClient = useQueryClient()
  const [number, setNumber] = useState(employee?.employeeNumber ?? "")
  const [fullName, setFullName] = useState(employee?.fullName ?? "")
  const [position, setPosition] = useState(employee?.positionTitle ?? "")
  const [joinDate, setJoinDate] = useState(employee?.joinDate ?? "")
  const [workEmail, setWorkEmail] = useState(employee?.workEmail ?? "")
  const [phone, setPhone] = useState(employee?.phone ?? "")
  const [nik, setNik] = useState("")
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (body: EmployeeInput, key) =>
      employee
        ? updateEmployee(
            employee.id,
            { ...body, version: employee.version },
            key
          )
        : createEmployee(body, key),
    onSuccess: async () => {
      toast.success(
        employee ? "Data karyawan diperbarui" : "Karyawan ditambahkan"
      )
      await queryClient.invalidateQueries({ queryKey: ["hris", "employees"] })
      onOpenChange(false)
    },
    onError: (err) => {
      const problem = parseProblem(err)
      if (problem.code === "DUPLICATE_CODE") {
        setErrors({ employeeNumber: "Nomor karyawan sudah dipakai" })
        return
      }
      handleFormProblem(problem, setErrors)
    },
  })

  const submit = () => {
    const next: FieldErrors = {}
    if (!number.trim()) next.employeeNumber = "Nomor karyawan wajib diisi"
    if (!fullName.trim()) next.fullName = "Nama lengkap wajib diisi"
    if (!position.trim()) next.positionTitle = "Jabatan wajib diisi"
    if (!joinDate) next.joinDate = "Tanggal bergabung wajib diisi"
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate({
      employeeNumber: number,
      fullName,
      positionTitle: position,
      joinDate,
      workEmail,
      phone,
      // Kosong saat ubah = NIK lama dipertahankan.
      nik: employee && !nik ? undefined : nik,
      divisionId: employee?.divisionId ?? null,
    })
  }

  return (
    <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-xl">
      <DialogHeader>
        <DialogTitle>
          {employee
            ? `Ubah Karyawan ${employee.employeeNumber}`
            : "Tambah Karyawan"}
        </DialogTitle>
        <DialogDescription>
          Nomor karyawan permanen dan tidak bisa dipakai ulang.
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
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={!!errors.employeeNumber}>
              <FieldLabel htmlFor="employee-number">Nomor karyawan</FieldLabel>
              <Input
                id="employee-number"
                value={number}
                disabled={!!employee}
                className="uppercase"
                aria-invalid={!!errors.employeeNumber}
                onChange={(e) => setNumber(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "employeeNumber")} />
            </Field>
            <Field data-invalid={!!errors.fullName}>
              <FieldLabel htmlFor="employee-name">Nama lengkap</FieldLabel>
              <Input
                id="employee-name"
                value={fullName}
                aria-invalid={!!errors.fullName}
                onChange={(e) => setFullName(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "fullName")} />
            </Field>
            <Field data-invalid={!!errors.positionTitle}>
              <FieldLabel htmlFor="employee-position">Jabatan</FieldLabel>
              <Input
                id="employee-position"
                value={position}
                aria-invalid={!!errors.positionTitle}
                onChange={(e) => setPosition(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "positionTitle")} />
            </Field>
            <Field data-invalid={!!errors.joinDate}>
              <FieldLabel htmlFor="employee-join">Tanggal bergabung</FieldLabel>
              <DateInput
                id="employee-join"
                value={joinDate}
                onValueChange={setJoinDate}
                aria-invalid={!!errors.joinDate}
              />
              <FieldError errors={fieldError(errors, "joinDate")} />
            </Field>
            <Field data-invalid={!!errors.workEmail}>
              <FieldLabel htmlFor="employee-email">Email kantor</FieldLabel>
              <Input
                id="employee-email"
                type="email"
                value={workEmail}
                aria-invalid={!!errors.workEmail}
                onChange={(e) => setWorkEmail(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "workEmail")} />
            </Field>
            <Field data-invalid={!!errors.phone}>
              <FieldLabel htmlFor="employee-phone">Nomor HP</FieldLabel>
              <Input
                id="employee-phone"
                type="tel"
                inputMode="tel"
                value={phone}
                aria-invalid={!!errors.phone}
                onChange={(e) => setPhone(e.target.value)}
              />
              <FieldError errors={fieldError(errors, "phone")} />
            </Field>
          </div>
          <Field data-invalid={!!errors.nik}>
            <FieldLabel htmlFor="employee-nik">NIK (KTP)</FieldLabel>
            <Input
              id="employee-nik"
              inputMode="numeric"
              autoComplete="off"
              placeholder={
                employee?.nikMasked
                  ? `${employee.nikMasked} (kosongkan agar tetap)`
                  : "16 digit"
              }
              value={nik}
              aria-invalid={!!errors.nik}
              onChange={(e) => setNik(e.target.value)}
            />
            <FieldDescription>Disimpan terenkripsi.</FieldDescription>
            <FieldError errors={fieldError(errors, "nik")} />
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

type ResignDialogProps = {
  employee: Employee | null
  onOpenChange: (open: boolean) => void
}

/** Catat resign: mulai tanggal resign karyawan tidak aktif dan harga karyawan berhenti (HR-03). */
export function ResignDialog(props: ResignDialogProps) {
  return (
    <Dialog open={!!props.employee} onOpenChange={props.onOpenChange}>
      {props.employee ? (
        <ResignForm
          employee={props.employee}
          onOpenChange={props.onOpenChange}
        />
      ) : null}
    </Dialog>
  )
}

function ResignForm({
  employee,
  onOpenChange,
}: {
  employee: Employee
  onOpenChange: (open: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [date, setDate] = useState(todayBusinessDate())
  const [reason, setReason] = useState("")
  const [errors, setErrors] = useState<FieldErrors>({})

  const save = useIdempotentMutation({
    mutationFn: (_: void, key) =>
      resignEmployee(
        employee.id,
        { resignDate: date, reason, version: employee.version },
        key
      ),
    onSuccess: async () => {
      toast.success("Resign karyawan dicatat")
      await queryClient.invalidateQueries({ queryKey: ["hris", "employees"] })
      onOpenChange(false)
    },
    onError: (err) => handleFormProblem(parseProblem(err), setErrors),
  })

  return (
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>Catat Resign {employee.fullName}</DialogTitle>
        <DialogDescription>
          Mulai tanggal resign, karyawan tidak aktif dan pelanggan yang tertaut
          tidak lagi mendapat harga karyawan.
        </DialogDescription>
      </DialogHeader>
      <form
        noValidate
        onSubmit={(e) => {
          e.preventDefault()
          const next: FieldErrors = {}
          if (!date) next.resignDate = "Tanggal resign wajib diisi"
          if (!reason.trim()) next.reason = "Alasan resign wajib diisi"
          setErrors(next)
          if (Object.keys(next).length === 0) save.mutate()
        }}
      >
        <FieldGroup>
          <Field data-invalid={!!errors.resignDate}>
            <FieldLabel htmlFor="resign-date">Tanggal resign</FieldLabel>
            <DateInput
              id="resign-date"
              value={date}
              onValueChange={setDate}
              aria-invalid={!!errors.resignDate}
            />
            <FieldError errors={fieldError(errors, "resignDate")} />
          </Field>
          <Field data-invalid={!!errors.reason}>
            <FieldLabel htmlFor="resign-reason">Alasan</FieldLabel>
            <Textarea
              id="resign-reason"
              value={reason}
              aria-invalid={!!errors.reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <FieldError errors={fieldError(errors, "reason")} />
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
            variant="destructive"
            loading={save.isPending}
            loadingText="Menyimpan…"
          >
            Catat Resign
          </LoadingButton>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
