import { useEffect, useState } from "react"
import { SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { AppSidebar } from "@/components/app-sidebar"
import { DashboardCard } from "@/components/dashboard-card"
import { adminApi } from "@/lib/api"

export function App() {
  const [data, setData] = useState({ workspaces: 0, tools: 0 })

  useEffect(() => {
    Promise.all([adminApi.workspaces(), adminApi.tools()]).then(([workspaces, tools]) => setData({ workspaces: workspaces.length, tools: tools.length }))
  }, [])

  return <SidebarProvider><AppSidebar /><main className="flex-1 p-6"><SidebarTrigger /><h1 className="mb-6 text-2xl font-semibold">Overview</h1><div className="grid gap-4 md:grid-cols-2"><DashboardCard title="Workspaces" value={data.workspaces} /><DashboardCard title="Tools" value={data.tools} /></div></main></SidebarProvider>
}

export default App
