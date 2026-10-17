import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { BellIcon } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { useIdempotentMutation } from "@/lib/api/use-idempotent-mutation"
import { formatDateTime } from "@/lib/format/date"
import { cn } from "@/lib/utils"

import {
  markAllNotificationsRead,
  markNotificationRead,
  notificationsQuery,
  unreadCountQuery,
  type AppNotification,
} from "../api/notification-api"

/** Badge angka; lebih dari 99 ditampilkan 99+. */
export function countLabel(n: number) {
  return n > 99 ? "99+" : String(n)
}

/** Notifikasi in-app di header (NFR-11): approval menunggu, pengingat, dan hasil keputusan. */
export function NotificationBell() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const unread = useQuery(unreadCountQuery)
  const list = useQuery({ ...notificationsQuery, enabled: open })
  const count = unread.data?.unreadCount ?? 0

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["notification"] })

  const markRead = useIdempotentMutation({
    mutationFn: (n: AppNotification, key) => markNotificationRead(n.id, key),
    onSettled: refresh,
  })
  const markAll = useIdempotentMutation({
    mutationFn: (_: void, key) => markAllNotificationsRead(key),
    onSettled: refresh,
  })

  const openNotification = (n: AppNotification) => {
    if (!n.readAt) {
      markRead.mutate(n)
    }
    setOpen(false)
    if (n.link) {
      void navigate({ to: n.link })
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            className="relative"
            aria-label={
              count > 0 ? `Notifikasi, ${count} belum dibaca` : "Notifikasi"
            }
          />
        }
      >
        <BellIcon />
        {count > 0 ? (
          <span
            aria-hidden="true"
            className="absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] leading-none font-semibold text-white"
          >
            {countLabel(count)}
          </span>
        ) : null}
      </PopoverTrigger>
      <PopoverContent
        align="end"
        className="w-[min(24rem,calc(100vw-2rem))] p-0"
      >
        <div className="flex items-center justify-between border-b px-3 py-2">
          <span className="text-sm font-semibold">Notifikasi</span>
          <Button
            variant="ghost"
            size="sm"
            disabled={count === 0 || markAll.isPending}
            onClick={() => markAll.mutate()}
          >
            Tandai semua dibaca
          </Button>
        </div>
        <ul className="max-h-96 overflow-y-auto" aria-busy={list.isFetching}>
          {(list.data?.items ?? []).map((n) => (
            <li key={n.id} className="border-b last:border-b-0">
              <button
                type="button"
                className={cn(
                  "flex w-full gap-2 px-3 py-2.5 text-left text-sm hover:bg-accent focus-visible:bg-accent focus-visible:outline-none",
                  !n.readAt && "bg-primary/5"
                )}
                onClick={() => openNotification(n)}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    "mt-1.5 size-2 shrink-0 rounded-full",
                    n.readAt ? "bg-transparent" : "bg-primary"
                  )}
                />
                <span className="flex min-w-0 flex-col gap-0.5">
                  <span
                    className={cn("break-words", !n.readAt && "font-medium")}
                  >
                    {n.title}
                    {!n.readAt ? (
                      <span className="sr-only"> (belum dibaca)</span>
                    ) : null}
                  </span>
                  {n.body ? (
                    <span className="text-xs break-words text-muted-foreground">
                      {n.body}
                    </span>
                  ) : null}
                  <span className="text-xs text-muted-foreground">
                    {formatDateTime(n.createdAt)}
                  </span>
                </span>
              </button>
            </li>
          ))}
          {list.data && list.data.items.length === 0 ? (
            <li className="px-3 py-6 text-center text-sm text-muted-foreground">
              Belum ada notifikasi.
            </li>
          ) : null}
          {list.isPending && open ? (
            <li className="px-3 py-6 text-center text-sm text-muted-foreground">
              Memuat notifikasi…
            </li>
          ) : null}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
