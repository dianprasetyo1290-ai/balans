import { useQuery, useQueryClient } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { MoreHorizontalIcon, PlusIcon } from "lucide-react"
import { useState } from "react"
import { toast } from "sonner"

import { ConfirmDialog } from "@/components/common/confirm-dialog"
import { LoadingArea } from "@/components/common/loading-area"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { usePermissions } from "@/features/auth/api/auth-api"
import { parseProblem } from "@/lib/api/problem"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { formatRupiah } from "@/lib/format/money"
import { showErrorDialog } from "@/stores/error-dialog-store"

import {
  approvalPoliciesQuery,
  endPolicy,
  subjectTypesQuery,
  type ApprovalPolicy,
  type ApprovalSubjectType,
} from "../api/approval-api"
import { DelegationsSection } from "./approval-inbox-page"
import { PolicyFormDialog } from "./policy-form-dialog"

const route = getRouteApi("/_app/settings/approval-policies")

/** `> Rp10.000.000`, `> Rp5.000.000 s.d. Rp25.000.000`, atau `Semua nominal`. */
export function amountRule(p: Pick<ApprovalPolicy, "minAmount" | "maxAmount">) {
  if (p.minAmount && p.maxAmount) {
    return `> ${formatRupiah(p.minAmount)} s.d. ${formatRupiah(p.maxAmount)}`
  }
  if (p.minAmount) {
    return `> ${formatRupiah(p.minAmount)}`
  }
  if (p.maxAmount) {
    return `≤ ${formatRupiah(p.maxAmount)}`
  }
  return "Semua nominal"
}

/** Pengaturan › Matriks Persetujuan (BRD 5.17, Desain Role & Approval bagian 4). */
export function ApprovalPoliciesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const { can, user } = usePermissions()
  const canWrite = can("approval.policy.write")
  const subjectTypes = useQuery(subjectTypesQuery)
  const policies = useQuery(approvalPoliciesQuery(search.includeEnded))
  const [form, setForm] = useState<{
    policy?: ApprovalPolicy
    subjectType?: string
  } | null>(null)
  const [ending, setEnding] = useState<ApprovalPolicy | null>(null)

  const end = useIdempotentMutation({
    mutationFn: (p: ApprovalPolicy, key) => endPolicy(p.id, p.version, key),
    onSuccess: async () => {
      toast.success("Level persetujuan dihentikan")
      setEnding(null)
      await queryClient.invalidateQueries({
        queryKey: ["approval", "policies"],
      })
    },
    onError: (err) => {
      setEnding(null)
      showErrorDialog(parseProblem(err))
    },
  })

  const types = subjectTypes.data ?? []
  const byType = (t: ApprovalSubjectType) =>
    (policies.data ?? []).filter((p) => p.subjectType === t.type)
  const conditionLabel = (t: ApprovalSubjectType, code: string) =>
    t.conditions.find((c) => c.code === code)?.label ?? code

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Matriks Persetujuan
          </h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Level, batas nominal, dan approver per jenis dokumen. Perubahan
            berlaku untuk dokumen yang diajukan setelahnya; pembuat dokumen
            tidak pernah menyetujui dokumennya sendiri.
          </p>
        </div>
        {canWrite ? (
          <Button onClick={() => setForm({})}>
            <PlusIcon /> Tambah Level
          </Button>
        ) : null}
      </div>
      <label className="flex min-h-11 w-fit items-center gap-2 text-sm">
        <input
          type="checkbox"
          className="size-4 accent-(--color-primary)"
          checked={search.includeEnded}
          onChange={(e) =>
            void navigate({
              search: (s) => ({ ...s, includeEnded: e.target.checked }),
            })
          }
        />
        Tampilkan versi yang sudah tidak berlaku
      </label>
      <LoadingArea loading={policies.isPending || subjectTypes.isPending}>
        <div className="flex flex-col gap-4">
          {types.map((t) => {
            const rows = byType(t)
            return (
              <Card key={t.type}>
                <CardHeader className="flex flex-row items-center justify-between gap-2">
                  <CardTitle className="text-base">{t.label}</CardTitle>
                  {canWrite ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setForm({ subjectType: t.type })}
                    >
                      <PlusIcon /> Level
                    </Button>
                  ) : null}
                </CardHeader>
                <CardContent className="overflow-x-auto">
                  {rows.length === 0 ? (
                    <p className="text-sm text-amber-700 dark:text-amber-300">
                      Belum ada kebijakan. Dokumen jenis ini tidak bisa
                      diajukan.
                    </p>
                  ) : (
                    <Table>
                      <TableHeader>
                        <TableRow>
                          <TableHead className="w-16">Level</TableHead>
                          <TableHead>
                            {t.amountLabel === "—" ? "Kondisi" : t.amountLabel}
                          </TableHead>
                          <TableHead>Approver</TableHead>
                          <TableHead>Pengganti</TableHead>
                          <TableHead>Berlaku</TableHead>
                          <TableHead className="w-12" />
                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {rows.map((p) => (
                          <TableRow
                            key={p.id}
                            className={
                              p.isActive ? "" : "text-muted-foreground"
                            }
                          >
                            <TableCell className="font-medium">
                              {p.level}
                            </TableCell>
                            <TableCell>
                              <div className="flex flex-col">
                                <span>{amountRule(p)}</span>
                                {p.condition ? (
                                  <span className="text-xs text-muted-foreground">
                                    {conditionLabel(t, p.condition)}
                                  </span>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell>
                              <div className="flex flex-col">
                                <span>{p.approverRoleName}</span>
                                {p.approverUserName ? (
                                  <span className="text-xs text-muted-foreground">
                                    {p.approverUserName}
                                  </span>
                                ) : null}
                              </div>
                            </TableCell>
                            <TableCell>
                              {p.substituteUserName ?? (
                                <span className="text-xs text-muted-foreground">
                                  Belum diatur
                                </span>
                              )}
                            </TableCell>
                            <TableCell className="text-xs">
                              {p.isActive ? (
                                `Sejak ${formatDateTime(p.validFrom)}`
                              ) : p.validTo ? (
                                <Badge variant="outline">
                                  Berakhir {formatDateTime(p.validTo)}
                                </Badge>
                              ) : (
                                `Mulai ${formatDateTime(p.validFrom)}`
                              )}
                            </TableCell>
                            <TableCell className="text-right">
                              {canWrite && p.isActive ? (
                                <DropdownMenu>
                                  <DropdownMenuTrigger
                                    render={
                                      <Button
                                        variant="ghost"
                                        size="icon"
                                        aria-label={`Aksi untuk ${t.label} level ${p.level}`}
                                      />
                                    }
                                  >
                                    <MoreHorizontalIcon />
                                  </DropdownMenuTrigger>
                                  <DropdownMenuContent align="end">
                                    <DropdownMenuItem
                                      onClick={() => setForm({ policy: p })}
                                    >
                                      Ubah
                                    </DropdownMenuItem>
                                    <DropdownMenuItem
                                      variant="destructive"
                                      onClick={() => setEnding(p)}
                                    >
                                      Hentikan
                                    </DropdownMenuItem>
                                  </DropdownMenuContent>
                                </DropdownMenu>
                              ) : null}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  )}
                </CardContent>
              </Card>
            )
          })}
        </div>
      </LoadingArea>
      <section
        className="flex flex-col gap-3"
        aria-labelledby="all-delegations"
      >
        <div>
          <h2 id="all-delegations" className="text-lg font-semibold">
            Delegasi Semua Pengguna
          </h2>
          <p className="text-sm text-muted-foreground">
            Approver yang cuti mendelegasikan persetujuannya ke pengguna lain.
          </p>
        </div>
        {user ? (
          <DelegationsSection
            currentUserId={user.id}
            all
            canManageOthers={canWrite}
          />
        ) : null}
      </section>
      <PolicyFormDialog
        open={!!form}
        onOpenChange={(open) => !open && setForm(null)}
        subjectTypes={types}
        policy={form?.policy}
        defaultSubjectType={form?.subjectType}
      />
      {ending ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setEnding(null)}
          title={`Hentikan ${ending.subjectTypeLabel} level ${ending.level}?`}
          description="Dokumen yang diajukan setelah ini tidak lagi melewati level ini. Dokumen yang sudah diajukan tidak terpengaruh."
          confirmLabel="Hentikan Level"
          destructive
          loading={end.isPending}
          onConfirm={() => end.mutate(ending)}
        />
      ) : null}
    </div>
  )
}
