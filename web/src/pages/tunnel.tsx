import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { adminApi, type TunnelConfig, type TunnelStatus } from "@/lib/api"

const emptyConfig: TunnelConfig = { enabled: false }

export function TunnelPage() {
  const [config, setConfig] = useState<TunnelConfig>(emptyConfig)
  const [args, setArgs] = useState("")
  const [status, setStatus] = useState<TunnelStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  async function load() {
    try {
      const [nextConfig, nextStatus] = await Promise.all([adminApi.tunnelConfig(), adminApi.tunnel()])
      setConfig(nextConfig)
      setArgs((nextConfig.args ?? []).join("\n"))
      setStatus(nextStatus)
      setError("")
    } catch (value) { setError(value instanceof Error ? value.message : String(value)) }
  }

  useEffect(() => { void load(); const timer = window.setInterval(() => { void adminApi.tunnel().then(setStatus).catch(() => undefined) }, 3000); return () => window.clearInterval(timer) }, [])

  async function save() {
    setBusy(true)
    try {
      const next = await adminApi.configureTunnel({ ...config, args: args.split("\n").map((value) => value.trim()).filter(Boolean) })
      setStatus(next)
      setError("")
    } catch (value) { setError(value instanceof Error ? value.message : String(value)) } finally { setBusy(false) }
  }

  async function toggle() {
    setBusy(true)
    try { setStatus(status?.running ? await adminApi.stopTunnel() : await adminApi.startTunnel()); setError("") } catch (value) { setError(value instanceof Error ? value.message : String(value)) } finally { setBusy(false) }
  }

  return <div className="space-y-6"><Card><CardHeader className="flex-row items-center justify-between"><CardTitle>Tunnel</CardTitle><Badge variant={status?.running ? "default" : "secondary"}>{status?.running ? `Running${status.pid ? ` · PID ${status.pid}` : ""}` : "Stopped"}</Badge></CardHeader><CardContent className="space-y-4"><div className="flex items-center justify-between rounded-lg border p-3"><div><div className="font-medium">Enabled</div><div className="text-sm text-muted-foreground">Start the tunnel with the MCP server.</div></div><Switch checked={config.enabled} onCheckedChange={(enabled) => setConfig({ ...config, enabled })} /></div><div className="grid gap-4 md:grid-cols-2"><Field label="Tunnel ID"><Input value={config.id ?? ""} onChange={(event) => setConfig({ ...config, id: event.target.value })} /></Field><Field label="API key"><Input type="password" placeholder="Leave blank to keep current key" value={config.api_key ?? ""} onChange={(event) => setConfig({ ...config, api_key: event.target.value })} /></Field><Field label="Command"><Input placeholder="cloudflared, ngrok, custom client..." value={config.command ?? ""} onChange={(event) => setConfig({ ...config, command: event.target.value })} /></Field><Field label="Origin"><Input placeholder="http://127.0.0.1:3000" value={config.origin ?? ""} onChange={(event) => setConfig({ ...config, origin: event.target.value })} /></Field><Field label="Public URL"><Input value={config.public_url ?? ""} onChange={(event) => setConfig({ ...config, public_url: event.target.value })} /></Field></div><Field label="Arguments (one per line)"><Textarea rows={5} value={args} onChange={(event) => setArgs(event.target.value)} /></Field>{status?.last_error ? <div className="text-sm text-destructive">{status.last_error}</div> : null}{error ? <div className="text-sm text-destructive">{error}</div> : null}<div className="flex gap-2"><Button disabled={busy} onClick={save}>Save</Button><Button disabled={busy || !config.enabled} variant="outline" onClick={toggle}>{status?.running ? "Stop" : "Start"}</Button></div></CardContent></Card></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
