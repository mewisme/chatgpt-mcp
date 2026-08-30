import { useEffect, useMemo, useState } from "react"
import { LoaderCircle, LogOut } from "lucide-react"
import { AppSidebar } from "@/components/app-sidebar"
import { navItems } from "@/lib/admin-navigation"
import { Button } from "@/components/ui/button"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { adminApi, adminToken, ApiError } from "@/lib/api"
import { ActivityPage } from "@/pages/activity"
import { LoginPage } from "@/pages/login"
import { OverviewPage } from "@/pages/overview"
import { ServersPage } from "@/pages/servers"
import { SettingsPage } from "@/pages/settings"
import { ToolsPage } from "@/pages/tools"
import { TunnelPage } from "@/pages/tunnel"
import { WorkspacesPage } from "@/pages/workspaces"

const pages: Record<string, React.ComponentType> = {
  overview: OverviewPage,
  workspaces: WorkspacesPage,
  tools: ToolsPage,
  servers: ServersPage,
  tunnel: TunnelPage,
  activity: ActivityPage,
  settings: SettingsPage,
}

export function App() {
  const [page, setPage] = useState("overview")
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)
  const [authRequired, setAuthRequired] = useState(true)
  const meta = useMemo(() => navItems.find((item) => item.id === page) ?? navItems[0], [page])
  const Page = pages[page] ?? OverviewPage

  useEffect(() => {
    void adminApi.health().then((health) => { setAuthRequired(health.auth_enabled); setAuthenticated(true) }).catch((value) => {
      if (value instanceof ApiError && value.status === 401) { adminToken.clear(); setAuthRequired(true); setAuthenticated(false); return }
      setAuthenticated(false)
    })
  }, [])

  useEffect(() => { document.title = `${meta.title} - chatgpt-mcp` }, [meta.title])

  if (authenticated === null) return <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />Connecting to admin API...</div>
  if (!authenticated) return <LoginPage onAuthenticated={() => { setAuthRequired(true); setAuthenticated(true) }} />

  function signOut() { adminToken.clear(); setAuthenticated(false) }

  return <TooltipProvider><SidebarProvider><AppSidebar page={page} onPageChange={setPage} /><SidebarInset className="min-w-0"><header className="sticky top-0 z-20 flex min-h-14 items-center gap-3 border-b bg-background/95 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/80"><SidebarTrigger /><div className="min-w-0 flex-1"><div className="truncate text-sm font-semibold">{meta.title}</div><div className="hidden truncate text-xs text-muted-foreground sm:block">{meta.description}</div></div>{authRequired ? <Button size="sm" variant="ghost" onClick={signOut}><LogOut />Sign out</Button> : null}</header><main className="min-w-0 flex-1 bg-muted/20"><div className="mx-auto w-full max-w-[1440px] p-4 sm:p-6 lg:p-8"><Page /></div></main></SidebarInset></SidebarProvider></TooltipProvider>
}

export default App
