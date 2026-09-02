import { useEffect, useState } from "react"
import { Boxes, FolderGit2, Network, RefreshCw, Server, Share2, ShieldCheck, Wrench } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { DashboardCard } from "@/components/dashboard-card"
import { DetailRow } from "@/components/detail-row"
import { PageError } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import { adminApi, type ClusterStatus } from "@/lib/api"

type DashboardData = { workspaces: number; tools: number; servers: number; enabledServers: number; tunnel: "Ready" | "Connecting" | "Stopped"; tunnelName: string; cluster: ClusterStatus; preset: string; mcpEndpoint: string; adminEndpoint: string; mcpAuth: boolean; adminAuth: boolean; updatedAt: Date }

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

  async function refresh() { setBusy(true); try { setData(await loadDashboard()); setError("") } catch (value) { setError(errorText(value)) } finally { setBusy(false) } }
  const clusterState = data ? clusterLabel(data.cluster) : "-"

  return <div className="space-y-6"><PageHeader title="Overview" description="Runtime health, exposure, authentication, cluster federation, and registered resources at a glance." actions={<><Badge variant={data?.tunnel === "Ready" ? "default" : "secondary"}>{data?.tunnel === "Ready" ? "Tunnel ready" : data?.tunnel === "Connecting" ? "Tunnel connecting" : "Tunnel stopped"}</Badge><Button disabled={busy} size="sm" variant="outline" onClick={() => void refresh()}><RefreshCw className={busy ? "animate-spin" : ""} />Refresh</Button></>} /><div className="text-xs text-muted-foreground">{data ? `Updated ${data.updatedAt.toLocaleTimeString()}` : "Loading runtime status..."}</div><PageError message={error} /><div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5"><DashboardCard title="Workspaces" value={data?.workspaces ?? "-"} description="Registered project roots" icon={FolderGit2} /><DashboardCard title="Tools" value={data?.tools ?? "-"} description="Currently exposed tools" icon={Wrench} /><DashboardCard title="MCP Servers" value={data ? `${data.enabledServers}/${data.servers}` : "-"} description="Enabled upstream servers" icon={Server} /><DashboardCard title="Cluster" value={clusterState} description={data?.cluster.name || "Runtime federation"} icon={Share2} /><DashboardCard title="Tunnel" value={data?.tunnel ?? "-"} description={data?.tunnelName || "OpenAI Secure MCP Tunnel"} icon={Network} /></div><div className="grid gap-4 xl:grid-cols-3"><Card><CardHeader><CardTitle className="flex items-center gap-2"><Boxes className="size-4" />Runtime</CardTitle><CardDescription>Active listeners and configuration preset.</CardDescription></CardHeader><CardContent className="divide-y"><DetailRow label="Preset" value={data?.preset ?? "-"} /><EndpointDetail label="MCP endpoint" value={data?.mcpEndpoint ?? "-"} /><EndpointDetail label="Admin endpoint" value={data?.adminEndpoint ?? "-"} /></CardContent></Card><ClusterCard cluster={data?.cluster} /><Card><CardHeader><CardTitle className="flex items-center gap-2"><ShieldCheck className="size-4" />Authentication</CardTitle><CardDescription>Protection applied to the local MCP and Admin listeners.</CardDescription></CardHeader><CardContent className="space-y-0"><AuthState label="MCP authentication" enabled={data?.mcpAuth} /><Separator /><AuthState label="Admin authentication" enabled={data?.adminAuth} /></CardContent></Card></div></div>
}

function ClusterCard({ cluster }: { cluster?: ClusterStatus }) {
  const catalog = !cluster ? "-" : cluster.catalog_compatible ? "Compatible" : "Incompatible"
  const role = cluster?.tunnel_role ? cluster.tunnel_role.replaceAll("_", " ") : "-"
  return <Card><CardHeader><CardTitle className="flex items-center gap-2"><Share2 className="size-4" />Cluster</CardTitle><CardDescription>Instance identity, membership, catalog, and tunnel leadership.</CardDescription></CardHeader><CardContent className="divide-y"><DetailRow label="Status" value={<Badge variant={cluster?.connected ? "secondary" : "outline"}>{cluster ? clusterLabel(cluster) : "-"}</Badge>} /><DetailRow label="Instance" value={cluster?.name || "-"} /><DetailRow label="Instance ID" value={cluster?.instance_id || "-"} mono /><DetailRow label="Members" value={cluster?.connected ? `${cluster.online_member_count}/${cluster.member_count} online` : "-"} /><DetailRow label="Workspaces" value={cluster?.workspace_count ?? "-"} /><DetailRow label="Catalog" value={catalog} /><DetailRow label="Tunnel role" value={role} /><DetailRow label="Leader" value={cluster?.leader_instance_id || "-"} mono /><DetailRow label="Epoch" value={cluster?.leader_epoch ?? "-"} mono />{cluster?.catalog_error ? <DetailRow label="Catalog error" value={cluster.catalog_error} /> : null}{cluster?.last_error ? <DetailRow label="Cluster error" value={cluster.last_error} /> : null}</CardContent></Card>
}

function clusterLabel(cluster: ClusterStatus) { return !cluster.enabled ? "Disabled" : cluster.connected ? "Connected" : "Offline" }
function EndpointDetail({ label, value }: { label: string; value: string }) { return <DetailRow label={label} value={<div className="flex min-w-0 items-start justify-end gap-1"><span className="min-w-0 break-all font-mono text-sm font-normal">{value}</span>{value !== "-" && value !== "Disabled" ? <CopyButton label={`Copy ${label}`} value={value} /> : null}</div>} /> }
function AuthState({ label, enabled }: { label: string; enabled?: boolean }) { return <div className="flex items-center justify-between gap-3 py-3"><span className="text-sm text-muted-foreground">{label}</span><Badge variant={enabled ? "secondary" : "outline"}>{enabled === undefined ? "-" : enabled ? "Enabled" : "Disabled"}</Badge></div> }

async function loadDashboard(): Promise<DashboardData> {
  const [workspaces, tools, servers, tunnel, cluster, presets, config] = await Promise.all([adminApi.workspaces(), adminApi.tools(), adminApi.upstream(), adminApi.tunnel(), adminApi.cluster(), adminApi.configPresets(), adminApi.config()])
  const host = window.location.hostname || "127.0.0.1"
  return { workspaces: workspaces.length, tools: tools.length, servers: servers.length, enabledServers: servers.filter((server) => server.enabled).length, tunnel: tunnel.running ? (tunnel.ready ? "Ready" : "Connecting") : "Stopped", tunnelName: tunnel.metadata?.name ?? "", cluster, preset: presets.current, mcpEndpoint: `http://${host}:${config.server.port}/mcp`, adminEndpoint: config.admin.enabled ? `http://${host}:${config.admin.port}` : "Disabled", mcpAuth: config.auth.mcp_enabled, adminAuth: config.auth.admin_enabled, updatedAt: new Date() }
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
