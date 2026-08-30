import { useEffect, useState } from "react"
import { RefreshCw } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { adminApi, type TunnelConfig, type TunnelStatus } from "@/lib/api"

const emptyConfig: TunnelConfig = { enabled: false }

export function TunnelPage() {
  const [config, setConfig] = useState<TunnelConfig>(emptyConfig)
  const [status, setStatus] = useState<TunnelStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")

  useEffect(() => {
    let active = true
    void Promise.all([adminApi.tunnelConfig(), adminApi.tunnel()]).then(([nextConfig, nextStatus]) => {
      if (!active) return
      setConfig(nextConfig)
      setStatus(nextStatus)
      setError("")
      setLoading(false)
    }).catch((value) => { if (active) { setError(errorText(value)); setLoading(false) } })
    const timer = window.setInterval(() => { void adminApi.tunnel().then((next) => { if (active) setStatus(next) }).catch(() => undefined) }, 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])

  async function refresh() {
    setRefreshing(true)
    try { setStatus(await adminApi.tunnel()); setError("") } catch (value) { setError(errorText(value)) } finally { setRefreshing(false) }
  }

  async function save() {
    setBusy(true)
    try { setStatus(await adminApi.configureTunnel(config)); setMessage("Tunnel configuration saved."); setError("") } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) }
  }

  async function toggle() {
    const active = status?.running || status?.restarting
    setBusy(true)
    try { setStatus(active ? await adminApi.stopTunnel() : await adminApi.startTunnel()); setMessage(active ? "Tunnel stopped." : "Tunnel start requested."); setError("") } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) }
  }

  const active = status?.running || status?.restarting
  const state = status?.restarting ? "Reconnecting" : status?.running ? status.ready ? "Ready" : "Connecting" : "Stopped"
  const variant = status?.ready ? "default" : active ? "secondary" : "outline"

  return <div className="space-y-6"><Card><CardHeader className="gap-3 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle>OpenAI Secure MCP Tunnel</CardTitle><CardDescription className="mt-1">Builtin Go client. No external tunnel binary or public MCP port is required.</CardDescription></div><div className="flex items-center gap-2"><Badge variant={variant}>{loading ? "Loading" : state}</Badge><Button aria-label="Refresh tunnel status" disabled={refreshing} size="icon-sm" variant="ghost" onClick={() => void refresh()}><RefreshCw className={refreshing ? "animate-spin" : ""} /></Button></div></CardHeader><CardContent className="space-y-5"><div className="flex items-center justify-between gap-4 rounded-lg border p-4"><div><div className="font-medium">Start automatically</div><div className="text-sm text-muted-foreground">Connect this runtime to the OpenAI Tunnel control plane when the server starts.</div></div><Switch checked={config.enabled} disabled={loading} onCheckedChange={(enabled) => setConfig({ ...config, enabled })} /></div><div className="grid gap-4 md:grid-cols-2"><Field label="Tunnel ID" hint="Assigned OpenAI tunnel identifier."><Input placeholder="tunnel_..." value={config.id ?? ""} onChange={(event) => setConfig({ ...config, id: event.target.value })} /></Field><Field label="Runtime API key" hint="Leave blank to keep the current secret."><Input autoComplete="off" type="password" placeholder="Leave blank to keep current key" value={config.api_key ?? ""} onChange={(event) => setConfig({ ...config, api_key: event.target.value })} /></Field><Field label="Control plane base URL" hint="Usually left empty for the OpenAI default."><Input placeholder="https://api.openai.com" value={config.control_plane_base_url ?? ""} onChange={(event) => setConfig({ ...config, control_plane_base_url: event.target.value })} /></Field><Field label="Organization ID" hint="Optional OpenAI organization scope."><Input placeholder="org_..." value={config.organization_id ?? ""} onChange={(event) => setConfig({ ...config, organization_id: event.target.value })} /></Field></div><div className="grid gap-3 rounded-lg border bg-muted/20 p-4 text-sm sm:grid-cols-2"><StatusDetail label="Provider" value={status?.provider ?? "-"} /><StatusDetail label="Tunnel ID" value={status?.id ?? config.id ?? "-"} mono /><StatusDetail label="Started" value={status?.started_at ? formatDate(status.started_at) : "-"} /><StatusDetail label="Organization" value={status?.organization_id ?? "-"} mono /></div>{status?.last_error ? <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{status.last_error}</div> : null}{error ? <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">{error}</div> : null}{message ? <div className="rounded-lg border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">{message}</div> : null}<div className="flex flex-wrap gap-2"><Button disabled={busy || loading} onClick={() => void save()}>{busy ? "Working..." : "Save configuration"}</Button><Button disabled={busy || loading || !config.enabled} variant="outline" onClick={() => void toggle()}>{active ? "Stop tunnel" : "Start tunnel"}</Button></div></CardContent></Card></div>
}

function Field({ label, hint, children }: { label: string; hint: string; children: React.ReactNode }) {
  return <div className="space-y-2"><Label>{label}</Label>{children}<div className="text-xs text-muted-foreground">{hint}</div></div>
}

function StatusDetail({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div><div className="text-xs text-muted-foreground">{label}</div><div className={mono ? "mt-1 break-all font-mono" : "mt-1 font-medium"}>{value}</div></div>
}

function formatDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
