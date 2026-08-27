import { useEffect, useState } from "react"
import { DashboardCard } from "@/components/dashboard-card"
import { api } from "@/lib/api"

export function App() {
  const [stats, setStats] = useState({ workspaces: 0, tools: 0 })

  useEffect(() => {
    Promise.all([api.workspaces(), api.tools()]).then(([workspaces, tools]) => setStats({ workspaces: workspaces.length, tools: tools.length }))
  }, [])

  return <main className="min-h-svh p-6 space-y-6"><div><h1 className="text-2xl font-semibold">ChatGPT MCP</h1><p className="text-muted-foreground">Admin dashboard</p></div><div className="grid gap-4 md:grid-cols-2"><DashboardCard title="Workspaces" value={String(stats.workspaces)} /><DashboardCard title="Tools" value={String(stats.tools)} /></div></main>
}

export default App
