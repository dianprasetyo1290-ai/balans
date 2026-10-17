import { cn } from "@/lib/utils"

type PageTabsProps<T extends string> = {
  tabs: { id: T; label: string }[]
  value: T
  onValueChange: (value: T) => void
  label: string
}

/** Tab halaman (mis. Ringkasan / Riwayat). Tab aktif disimpan di URL oleh route pemakai. */
export function PageTabs<T extends string>({
  tabs,
  value,
  onValueChange,
  label,
}: PageTabsProps<T>) {
  return (
    <div
      role="tablist"
      aria-label={label}
      className="flex gap-1 overflow-x-auto border-b"
    >
      {tabs.map((tab) => {
        const active = value === tab.id
        return (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={active}
            className={cn(
              "-mb-px min-h-11 shrink-0 border-b-2 px-3 text-sm font-medium transition-colors",
              active
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
            onClick={() => onValueChange(tab.id)}
          >
            {tab.label}
          </button>
        )
      })}
    </div>
  )
}
