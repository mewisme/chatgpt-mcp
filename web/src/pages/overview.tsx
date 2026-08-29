import { useEffect, useState } from "react"
import { RefreshCw } from "lucide-react"
import { DashboardCard } from "@/components/dashboard-card"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { adminApi } from "@/lib/api"

type DashboardData = {
  workspaces: number
  tools: number
  servers: number
  enabledServers: number
  tunnel: string
  preset: string
  updatedAt: Date
}

export function OverviewPage() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    let active = true
    void loadDashboard().then((next) => { if (active) { setData(next); setError("") } }).catch((value) => { if (active) setError(errorText(value)) })
    const timer = window.setInterval(() => {
      void loadDashboard().then((next) => { if (active) { setData(next); setError("") } }).catch((value) => { if (active) setError(errorText(value)) })
    }, 5000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])

  async function refresh() {
    setBusy(true)
    try { setData(await loadDashboard()); setError("") } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  return <div className="space-y-6"><div className="flex flex-wrap items-center justify-between gap-3"><div><h1 className="text-xl font-semibold">Overview</h1><div className="text-sm text-muted-foreground">{data ? `Updated ${data.updatedAt.toLocaleTimeString()}` : "Loading runtime status..."}</div></div><Button disabled={busy} size="sm" variant="outline" onClick={refresh}><RefreshCw className={busy ? "animate-spin" : ""} />Refresh</Button></div>{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4"><DashboardCard title="Workspaces" value={data?.workspaces ?? "-"} /><DashboardCard title="Tools" value={data?.tools ?? "-"} /><DashboardCard title="MCP Servers" value={data ? `${data.enabledServers}/${data.servers} enabled` : "-"} /><DashboardCard title="Tunnel" value={data?.tunnel ?? "-"} /></div><Card><CardHeader><CardTitle>Runtime config</CardTitle><CardDescription>Dashboard values refresh every five seconds.</CardDescription></CardHeader><CardContent className="grid gap-3 text-sm md:grid-cols-2"><div><span className="text-muted-foreground">Preset</span><div className="font-medium">{data?.preset ?? "-"}</div></div><div><span className="text-muted-foreground">Tunnel</span><div className="font-medium">{data?.tunnel ?? "-"}</div></div></CardContent></Card></div>
}

async function loadDashboard(): Promise<DashboardData> {
  const [workspaces, tools, servers, tunnel, presets] = await Promise.all([adminApi.workspaces(), adminApi.tools(), adminApi.upstream(), adminApi.tunnel(), adminApi.configPresets()])
  return {
    workspaces: workspaces.length,
    tools: tools.length,
    servers: servers.length,
    enabledServers: servers.filter((server) => server.enabled).length,
    tunnel: tunnel.running ? (tunnel.ready ? "Ready" : "Connecting") : "Stopped",
    preset: presets.current,
    updatedAt: new Date(),
  }
}
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
