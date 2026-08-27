import { useState } from "react"
import { AppSidebar } from "@/components/app-sidebar"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { OverviewPage } from "@/pages/overview"
import { ServersPage } from "@/pages/servers"
import { SettingsPage } from "@/pages/settings"
import { ToolsPage } from "@/pages/tools"

export function App() {
  const [page, setPage] = useState("overview")
  return <SidebarProvider><AppSidebar page={page} onPageChange={setPage} /><SidebarInset><header className="flex h-14 items-center border-b px-4"><SidebarTrigger /></header><main className="p-6">{page === "tools" ? <ToolsPage tools={[]} /> : page === "servers" ? <ServersPage /> : page === "settings" ? <SettingsPage /> : <OverviewPage data={{ workspaces: 0, tools: 0 }} />}</main></SidebarInset></SidebarProvider>
}

export default App
