import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { adminApi, type TunnelConfig, type TunnelStatus } from "@/lib/api"

const emptyConfig: TunnelConfig = { enabled: false }

export function TunnelPage() {
  const [config, setConfig] = useState<TunnelConfig>(emptyConfig)
  const [status, setStatus] = useState<TunnelStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    void Promise.all([adminApi.tunnelConfig(), adminApi.tunnel()]).then(([nextConfig, nextStatus]) => { setConfig(nextConfig); setStatus(nextStatus) }).catch((value) => setError(value instanceof Error ? value.message : String(value)))
    const timer = window.setInterval(() => { void adminApi.tunnel().then(setStatus).catch(() => undefined) }, 3000)
    return () => window.clearInterval(timer)
  }, [])

  async function save() {
    setBusy(true)
    try {
      setStatus(await adminApi.configureTunnel(config))
      setError("")
    } catch (value) { setError(value instanceof Error ? value.message : String(value)) } finally { setBusy(false) }
  }

  async function toggle() {
    setBusy(true)
    try { setStatus(status?.running ? await adminApi.stopTunnel() : await adminApi.startTunnel()); setError("") } catch (value) { setError(value instanceof Error ? value.message : String(value)) } finally { setBusy(false) }
  }

  const state = status?.running ? status.ready ? "Ready" : "Connecting" : "Stopped"

  return <div className="space-y-6"><Card><CardHeader className="flex-row items-center justify-between"><div><CardTitle>OpenAI Secure MCP Tunnel</CardTitle><div className="mt-1 text-sm text-muted-foreground">Builtin Go client. No external tunnel binary or public MCP port is required.</div></div><Badge variant={status?.ready ? "default" : "secondary"}>{state}</Badge></CardHeader><CardContent className="space-y-4"><div className="flex items-center justify-between rounded-lg border p-3"><div><div className="font-medium">Enabled</div><div className="text-sm text-muted-foreground">Connect this MCP runtime to the OpenAI Tunnel control plane when the server starts.</div></div><Switch checked={config.enabled} onCheckedChange={(enabled) => setConfig({ ...config, enabled })} /></div><div className="grid gap-4 md:grid-cols-2"><Field label="Tunnel ID"><Input placeholder="tunnel_..." value={config.id ?? ""} onChange={(event) => setConfig({ ...config, id: event.target.value })} /></Field><Field label="Runtime API key"><Input type="password" placeholder="Leave blank to keep current key" value={config.api_key ?? ""} onChange={(event) => setConfig({ ...config, api_key: event.target.value })} /></Field><Field label="Control plane base URL"><Input placeholder="https://api.openai.com" value={config.control_plane_base_url ?? ""} onChange={(event) => setConfig({ ...config, control_plane_base_url: event.target.value })} /></Field><Field label="Organization ID"><Input placeholder="Optional" value={config.organization_id ?? ""} onChange={(event) => setConfig({ ...config, organization_id: event.target.value })} /></Field></div>{status?.last_error ? <div className="text-sm text-destructive">{status.last_error}</div> : null}{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="flex gap-2"><Button disabled={busy} onClick={save}>Save</Button><Button disabled={busy || !config.enabled} variant="outline" onClick={toggle}>{status?.running ? "Stop" : "Start"}</Button></div></CardContent></Card></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
