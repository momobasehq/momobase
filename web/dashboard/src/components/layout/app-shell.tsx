import { Activity, AppWindow, ChevronsUpDown, FileClock, LayoutDashboard, LogOut, Plug, Receipt, Route as RouteIcon, ShieldCheck, TerminalSquare, Users } from "lucide-react"
import { NavLink, Outlet, useLocation } from "react-router"

import { Logo } from "@/components/logo"
import { ThemeToggle } from "@/components/theme-toggle"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Separator } from "@/components/ui/separator"
import {
  Sidebar, SidebarContent, SidebarFooter, SidebarGroup, SidebarGroupContent, SidebarGroupLabel,
  SidebarHeader, SidebarInset, SidebarMenu, SidebarMenuButton, SidebarMenuItem, SidebarProvider, SidebarTrigger,
} from "@/components/ui/sidebar"
import { useAuth } from "@/hooks/use-auth"
import { titleCase } from "@/lib/format"

const navigation = [
  {
    label: "Overview",
    items: [
      { to: "/", label: "Dashboard", icon: LayoutDashboard, end: true },
      { to: "/transactions", label: "Transactions", icon: Receipt },
    ],
  },
  {
    label: "Configuration",
    items: [
      { to: "/apps", label: "Apps", icon: AppWindow },
      { to: "/providers", label: "Providers", icon: Plug },
      { to: "/routes", label: "Routes", icon: RouteIcon },
    ],
  },
  {
    label: "Administration",
    items: [
      { to: "/users", label: "Users", icon: Users },
      { to: "/roles", label: "Roles", icon: ShieldCheck },
      { to: "/operations", label: "Operations", icon: Activity },
      { to: "/audit", label: "Audit log", icon: FileClock },
      { to: "/api-tester", label: "API tester", icon: TerminalSquare },
    ],
  },
]

/** Titles keyed by the first path segment, for the header. */
const titles: Record<string, string> = {
  "": "Dashboard",
  transactions: "Transactions",
  apps: "Apps",
  providers: "Providers",
  routes: "Routes",
  users: "Users",
  roles: "Roles",
  operations: "Operations",
  audit: "Audit log",
  "api-tester": "API tester",
}

/** AppShell is the signed-in chrome: sidebar navigation plus a page header. */
export function AppShell() {
  const { me, signOut } = useAuth()
  const location = useLocation()
  const segment = location.pathname.split("/")[1] ?? ""
  const initials = (me?.name ?? me?.email ?? "?").slice(0, 2).toUpperCase()

  return (
    <SidebarProvider style={
        {
          "--sidebar-width": "calc(var(--spacing) * 72)",
          "--header-height": "calc(var(--spacing) * 12)",
        } as React.CSSProperties
      }>
      <Sidebar collapsible="icon">
        <SidebarHeader>
          <div className="flex items-center gap-2 px-2 py-1">
            <Logo className="size-5 shrink-0" />
            {/* The mark alone survives the icon-width collapse; the word does not. */}
            <span className="font-semibold tracking-tight group-data-[collapsible=icon]:hidden">Momobase</span>
          </div>
        </SidebarHeader>
        <SidebarContent>
          {navigation.map((group) => (
            <SidebarGroup key={group.label}>
              <SidebarGroupLabel>{group.label}</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  {group.items.map((item) => (
                    <SidebarMenuItem key={item.to}>
                      <NavLink to={item.to} end={item.end}>
                        {({ isActive }) => (
                          <SidebarMenuButton isActive={isActive} tooltip={item.label} render={<span />}>
                            <item.icon />
                            <span>{item.label}</span>
                          </SidebarMenuButton>
                        )}
                      </NavLink>
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          ))}
        </SidebarContent>
        <SidebarFooter>
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <SidebarMenuButton size="lg" tooltip={me?.name ?? "Account"}>
                      <Avatar className="size-7 shrink-0">
                        <AvatarFallback>{initials}</AvatarFallback>
                      </Avatar>
                      {/* Collapsed to icon width there is only room for the avatar,
                          and the trigger keeps the menu reachable at either width. */}
                      <div className="min-w-0 flex-1 text-start group-data-[collapsible=icon]:hidden">
                        <p className="truncate font-medium">{me?.name}</p>
                        <p className="text-muted-foreground truncate">{titleCase(me?.role ?? "")}</p>
                      </div>
                      <ChevronsUpDown className="ms-auto group-data-[collapsible=icon]:hidden" />
                    </SidebarMenuButton>
                  }
                />
                <DropdownMenuContent side="top" align="start" className="min-w-56">
                  <DropdownMenuLabel>
                    <p className="truncate font-medium">{me?.name}</p>
                    <p className="text-muted-foreground truncate font-normal">{me?.email}</p>
                  </DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={() => void signOut()}>
                    <LogOut />
                    Sign out
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
      </Sidebar>
      <SidebarInset>
        <header className="flex w-full h-12 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger />
          <Separator orientation="vertical" className="mx-2 data-[orientation=vertical]:h-6 my-auto" />
          <h1 className="font-medium">{titles[segment] ?? "Dashboard"}</h1>
          <div className="ms-auto">
            <ThemeToggle />
          </div>
        </header>
        <div className="flex-1 overflow-auto p-4">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
