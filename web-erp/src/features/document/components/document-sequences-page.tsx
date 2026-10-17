import { useQuery } from "@tanstack/react-query"
import { getRouteApi } from "@tanstack/react-router"
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"

import { LoadingArea } from "@/components/common/loading-area"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { formatDateTime } from "@/lib/format/date"

import { documentSequencesQuery } from "../api/document-api"

const route = getRouteApi("/_app/settings/document-sequences")

/** Pengaturan › Nomor Dokumen: nomor terakhir per kode dokumen per tahun (U-15, S-133). */
export function DocumentSequencesPage() {
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const sequences = useQuery(documentSequencesQuery(search.year))
  const year = sequences.data?.year ?? search.year
  const setYear = (y: number) => void navigate({ search: { year: y } })

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Nomor Dokumen</h1>
        <p className="max-w-2xl text-sm text-muted-foreground">
          Format KODE/TAHUN/BULAN/NOMOR dengan nomor urut 5 digit yang mulai
          dari 1 setiap tahun. Nomor diberikan saat dokumen diajukan atau
          diposting, sehingga draft yang batal tidak membuat nomor loncat.
        </p>
      </div>
      {year ? (
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="icon"
            aria-label="Tahun sebelumnya"
            onClick={() => setYear(year - 1)}
          >
            <ChevronLeftIcon />
          </Button>
          <span className="min-w-16 text-center font-medium tabular-nums">
            {year}
          </span>
          <Button
            variant="outline"
            size="icon"
            aria-label="Tahun berikutnya"
            onClick={() => setYear(year + 1)}
          >
            <ChevronRightIcon />
          </Button>
        </div>
      ) : null}
      <LoadingArea loading={sequences.isPending}>
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-20">Kode</TableHead>
                <TableHead>Dokumen</TableHead>
                <TableHead>Nomor Diberikan Saat</TableHead>
                <TableHead className="text-right">Nomor Terakhir</TableHead>
                <TableHead>Nomor Berikutnya</TableHead>
                <TableHead>Terakhir Dipakai</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(sequences.data?.items ?? []).map((s) => (
                <TableRow key={s.documentType}>
                  <TableCell className="font-medium">
                    {s.documentType}
                  </TableCell>
                  <TableCell>{s.label}</TableCell>
                  <TableCell>{s.assignedAt}</TableCell>
                  <TableCell className="text-right tabular-nums">
                    {s.lastNumber > 0 ? s.lastNumber : "—"}
                  </TableCell>
                  <TableCell className="font-mono text-xs whitespace-nowrap">
                    {s.example}
                  </TableCell>
                  <TableCell className="text-sm whitespace-nowrap text-muted-foreground">
                    {s.lastIssuedAt ? formatDateTime(s.lastIssuedAt) : "—"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </LoadingArea>
    </div>
  )
}
