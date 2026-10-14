import { Link, useRouterState } from "@tanstack/react-router"

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
import { visibleMenu } from "@/lib/navigation/menu"

type AppSidebarProps = {
  enabledModules: readonly string[]
  version: string
}

/** Sidebar dibangkitkan dari modul aktif (Technical Guideline 10.2, 14.5). */
export function AppSidebar({ enabledModules, version }: AppSidebarProps) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const groups = visibleMenu(enabledModules)

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
                <span className="text-xs text-sidebar-foreground/70">Balans Nutrition</span>
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
                      <SidebarMenuButton tooltip={item.label} isActive={pathname === item.to} render={<Link to={item.to} />}>
                        <group.icon aria-hidden="true" />
                        <span>{item.label}</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ) : null,
                )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>
      <SidebarFooter>
        <p className="px-2 text-xs text-sidebar-foreground/60 group-data-[collapsible=icon]:hidden">Versi {version}</p>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
