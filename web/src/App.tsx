import { useEffect, useState } from "react"
import { AppSidebar } from "@/components/app-sidebar"
import { Button } from "@/components/ui/button"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { adminApi, adminToken } from "@/lib/api"
import { ActivityPage } from "@/pages/activity"
import { LoginPage } from "@/pages/login"
import { OverviewPage } from "@/pages/overview"
import { ServersPage } from "@/pages/servers"
import { SettingsPage } from "@/pages/settings"
import { ToolsPage } from "@/pages/tools"
import { TunnelPage } from "@/pages/tunnel"
import { WorkspacesPage } from "@/pages/workspaces"

export function App() {
  const [page, setPage] = useState("overview")
  const [authenticated, setAuthenticated] = useState<boolean | null>(() => adminToken.get() ? null : false)

  useEffect(() => {
    if (!adminToken.get()) return
    void adminApi.health().then(() => setAuthenticated(true)).catch(() => { adminToken.clear(); setAuthenticated(false) })
  }, [])

  if (authenticated === null) return <div className="flex min-h-screen items-center justify-center text-sm text-muted-foreground">Loading...</div>
  if (!authenticated) return <LoginPage onAuthenticated={() => setAuthenticated(true)} />

  function signOut() { adminToken.clear(); setAuthenticated(false) }

  return <SidebarProvider><AppSidebar page={page} onPageChange={setPage} /><SidebarInset><header className="flex h-14 items-center justify-between border-b px-4"><SidebarTrigger /><Button size="sm" variant="ghost" onClick={signOut}>Sign out</Button></header><main className="p-6">{page === "activity" ? <ActivityPage /> : page === "workspaces" ? <WorkspacesPage /> : page === "tools" ? <ToolsPage /> : page === "servers" ? <ServersPage /> : page === "tunnel" ? <TunnelPage /> : page === "settings" ? <SettingsPage /> : <OverviewPage />}</main></SidebarInset></SidebarProvider>
}

export default App
