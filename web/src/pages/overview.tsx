import { useEffect, useState } from "react"
import { Boxes, FolderGit2, Network, RefreshCw, Server, ShieldCheck, Wrench } from "lucide-react"
import { DashboardCard } from "@/components/dashboard-card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { adminApi } from "@/lib/api"

type DashboardData = {
  workspaces: number
  tools: number
  servers: number
  enabledServers: number
  tunnel: "Ready" | "Connecting" | "Stopped"
  preset: string
  mcpEndpoint: string
  adminEndpoint: string
  mcpAuth: boolean
  adminAuth: boolean
  updatedAt: Date
}

export function OverviewPage() {
  const [data, setData] = useState<DashboardData | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    let active = true
    const update = () => void loadDashboard().then((next) => { if (active) { setData(next); setError("") } }).catch((value) => { if (active) setError(errorText(value)) })
    update()
    const timer = window.setInterval(update, 5000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])

  async function refresh() {
    setBusy(true)
    try { setData(await loadDashboard()); setError("") } catch (value) { setError(errorText(value)) } finally { setBusy(false) }
  }

  return <div className="space-y-6"><div className="flex flex-wrap items-center justify-between gap-3"><div className="flex flex-wrap items-center gap-2"><Badge variant={data?.tunnel === "Ready" ? "default" : "secondary"}>{data?.tunnel === "Ready" ? "Tunnel ready" : data?.tunnel === "Connecting" ? "Tunnel connecting" : "Tunnel stopped"}</Badge>{data ? <span className="text-xs text-muted-foreground">Updated {data.updatedAt.toLocaleTimeString()}</span> : <span className="text-xs text-muted-foreground">Loading runtime status...</span>}</div><Button disabled={busy} size="sm" variant="outline" onClick={refresh}><RefreshCw className={busy ? "animate-spin" : ""} />Refresh</Button></div>{error ? <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{error}</div> : null}<div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4"><DashboardCard title="Workspaces" value={data?.workspaces ?? "-"} description="Registered project roots" icon={FolderGit2} /><DashboardCard title="Tools" value={data?.tools ?? "-"} description="Currently exposed tools" icon={Wrench} /><DashboardCard title="MCP Servers" value={data ? `${data.enabledServers}/${data.servers}` : "-"} description="Enabled upstream servers" icon={Server} /><DashboardCard title="Tunnel" value={data?.tunnel ?? "-"} description="OpenAI Secure MCP Tunnel" icon={Network} /></div><div className="grid gap-4 lg:grid-cols-2"><Card><CardHeader><CardTitle className="flex items-center gap-2"><Boxes className="size-4" />Runtime</CardTitle><CardDescription>Active listeners and configuration preset.</CardDescription></CardHeader><CardContent className="space-y-4"><Detail label="Preset" value={data?.preset ?? "-"} /><Detail label="MCP endpoint" value={data?.mcpEndpoint ?? "-"} mono /><Detail label="Admin endpoint" value={data?.adminEndpoint ?? "-"} mono /></CardContent></Card><Card><CardHeader><CardTitle className="flex items-center gap-2"><ShieldCheck className="size-4" />Authentication</CardTitle><CardDescription>Current listener authentication state.</CardDescription></CardHeader><CardContent className="space-y-4"><AuthState label="MCP authentication" enabled={data?.mcpAuth} /><AuthState label="Admin authentication" enabled={data?.adminAuth} /></CardContent></Card></div></div>
}

function Detail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between"><span className="text-sm text-muted-foreground">{label}</span><span className={mono ? "break-all font-mono text-sm" : "text-sm font-medium"}>{value}</span></div>
}

function AuthState({ label, enabled }: { label: string; enabled?: boolean }) {
  return <div className="flex items-center justify-between gap-3"><span className="text-sm text-muted-foreground">{label}</span><Badge variant={enabled ? "secondary" : "outline"}>{enabled === undefined ? "-" : enabled ? "Enabled" : "Disabled"}</Badge></div>
}

async function loadDashboard(): Promise<DashboardData> {
  const [workspaces, tools, servers, tunnel, presets, config] = await Promise.all([adminApi.workspaces(), adminApi.tools(), adminApi.upstream(), adminApi.tunnel(), adminApi.configPresets(), adminApi.config()])
  const host = window.location.hostname || "127.0.0.1"
  return {
    workspaces: workspaces.length,
    tools: tools.length,
    servers: servers.length,
    enabledServers: servers.filter((server) => server.enabled).length,
    tunnel: tunnel.running ? (tunnel.ready ? "Ready" : "Connecting") : "Stopped",
    preset: presets.current,
    mcpEndpoint: `http://${host}:${config.server.port}/mcp`,
    adminEndpoint: config.admin.enabled ? `http://${host}:${config.admin.port}` : "Disabled",
    mcpAuth: config.auth.mcp_enabled,
    adminAuth: config.auth.admin_enabled,
    updatedAt: new Date(),
  }
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
