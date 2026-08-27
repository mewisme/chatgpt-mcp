import { useEffect, useState } from "react"
import { AppSidebar } from "@/components/app-sidebar"
import { DashboardCard } from "@/components/dashboard-card"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { adminApi } from "@/lib/api"

export function App() {
  const [data, setData] = useState<{ workspaces: unknown[]; tools: string[] }>({ workspaces: [], tools: [] })
  useEffect(() => { Promise.all([adminApi.workspaces(), adminApi.tools()]).then(([workspaces, tools]) => setData({ workspaces, tools })) }, [])
  return <SidebarProvider><AppSidebar/><SidebarInset><SidebarTrigger/><main className="grid gap-4 p-6 md:grid-cols-2"><DashboardCard title="Workspaces" value={data.workspaces.length}/><DashboardCard title="Tools" value={data.tools.length}/></main></SidebarInset></SidebarProvider>
}

export default App
