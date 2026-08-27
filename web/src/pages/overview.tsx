import { useEffect, useState } from "react"
import { DashboardCard } from "@/components/dashboard-card"
import { adminApi } from "@/lib/api"

export function OverviewPage() {
  const [data, setData] = useState({ workspaces: 0, tools: 0, servers: 0, tunnel: "Stopped" })
  const [error, setError] = useState("")
  useEffect(() => {
    void Promise.all([adminApi.workspaces(), adminApi.tools(), adminApi.upstream(), adminApi.tunnel()]).then(([workspaces, tools, servers, tunnel]) => {
      setData({ workspaces: workspaces.length, tools: tools.length, servers: servers.length, tunnel: tunnel.running ? "Running" : "Stopped" })
      setError("")
    }).catch((value) => setError(value instanceof Error ? value.message : String(value)))
  }, [])
  return <div className="space-y-4">{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4"><DashboardCard title="Workspaces" value={data.workspaces} /><DashboardCard title="Tools" value={data.tools} /><DashboardCard title="MCP Servers" value={data.servers} /><DashboardCard title="Tunnel" value={data.tunnel} /></div></div>
}
