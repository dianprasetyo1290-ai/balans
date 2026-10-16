import { useQueryClient } from "@tanstack/react-query"
import { Link, useNavigate, useRouterState } from "@tanstack/react-router"
import { ChevronsUpDownIcon, LogOutIcon } from "lucide-react"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar"
import { logout } from "@/features/auth/api/auth-api"
import { visibleMenu } from "@/lib/navigation/menu"

type AppSidebarProps = {
  enabledModules: readonly string[]
  permissions: readonly string[]
  version: string
  user: { fullName: string; email: string }
}

function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join("")
}

/** Sidebar dibangkitkan dari modul aktif dan permission user (Technical Guideline 10.2, 14.5). */
export function AppSidebar({
  enabledModules,
  permissions,
  version,
  user,
}: AppSidebarProps) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const groups = visibleMenu(enabledModules, [], permissions)

  const handleLogout = async () => {
    await logout()
    queryClient.clear()
    await navigate({ to: "/login" })
  }

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" render={<Link to="/" />}>
              <span className="flex size-8 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground">
                B
              </span>
              <span className="flex flex-col leading-tight">
                <span className="font-semibold">Balans ERP</span>
                <span className="text-xs text-sidebar-foreground/70">
                  Balans Nutrition
                </span>
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        {groups.map((group) => (
          <SidebarGroup key={group.label}>
            <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {group.items.map((item) =>
                  item.to ? (
                    <SidebarMenuItem key={item.label}>
                      <SidebarMenuButton
                        tooltip={item.label}
                        isActive={
                          item.to === "/"
                            ? pathname === "/"
                            : pathname.startsWith(item.to)
                        }
                        render={<Link to={item.to} />}
                      >
                        <group.icon aria-hidden="true" />
                        <span>{item.label}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ) : null
                )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <DropdownMenu>
              <DropdownMenuTrigger render={<SidebarMenuButton size="lg" />}>
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-sidebar-accent text-xs font-semibold">
                  {initials(user.fullName)}
                </span>
                <span className="flex min-w-0 flex-col leading-tight">
                  <span className="truncate text-sm font-medium">
                    {user.fullName}
                  </span>
                  <span className="truncate text-xs text-sidebar-foreground/70">
                    {user.email}
                  </span>
                </span>
                <ChevronsUpDownIcon
                  className="ml-auto size-4"
                  aria-hidden="true"
                />
              </DropdownMenuTrigger>
              <DropdownMenuContent
                side="top"
                align="start"
                className="min-w-56"
              >
                <DropdownMenuGroup>
                  <DropdownMenuLabel>
                    <span className="block truncate">{user.fullName}</span>
                    <span className="block truncate text-xs font-normal text-muted-foreground">
                      Versi {version}
                    </span>
                  </DropdownMenuLabel>
                </DropdownMenuGroup>
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={() => void handleLogout()}>
                  <LogOutIcon aria-hidden="true" />
                  Keluar
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
