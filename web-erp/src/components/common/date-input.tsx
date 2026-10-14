import { id } from "date-fns/locale"
import { CalendarIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { formatDate, parseBusinessDate, toBusinessDate } from "@/lib/format/date"
import { cn } from "@/lib/utils"

type DateInputProps = {
  id?: string
  /** Tanggal bisnis `YYYY-MM-DD`. Kosong = belum dipilih. */
  value: string
  onValueChange: (value: string) => void
  placeholder?: string
  disabled?: boolean
  "aria-invalid"?: boolean
}

/** Pemilih tanggal bisnis dengan tampilan `14 Okt 2026` dan nilai `YYYY-MM-DD` (Technical Guideline 4.5). */
export function DateInput({ id: inputId, value, onValueChange, placeholder = "Pilih tanggal", disabled, ...aria }: DateInputProps) {
  const [open, setOpen] = useState(false)
  const selected = value ? (parseBusinessDate(value) ?? undefined) : undefined

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            id={inputId}
            variant="outline"
            disabled={disabled}
            aria-invalid={aria["aria-invalid"]}
            className={cn("w-full justify-start font-normal", !value && "text-muted-foreground")}
          />
        }
      >
        <CalendarIcon aria-hidden="true" />
        {value ? formatDate(value) : placeholder}
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="single"
          locale={id}
          selected={selected}
          defaultMonth={selected}
          onSelect={(date) => {
            if (date) {
              onValueChange(toBusinessDate(date))
              setOpen(false)
            }
          }}
        />
      </PopoverContent>
    </Popover>
  )
}
