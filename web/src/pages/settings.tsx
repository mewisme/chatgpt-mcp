import { useEffect, useState } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { adminApi, type PublicConfig } from "@/lib/api"

export function SettingsPage() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")
  useEffect(() => { void adminApi.config().then(setConfig).catch((value) => setError(value instanceof Error ? value.message : String(value))) }, [])

  async function save() {
    if (!config) return
    try { setConfig(await adminApi.saveConfig(config)); setMessage("Saved. Restart the server to apply listener/auth changes."); setError("") } catch (value) { setError(value instanceof Error ? value.message : String(value)); setMessage("") }
  }

  if (!config) return <div className="text-sm text-muted-foreground">{error || "Loading..."}</div>
  return <Card><CardHeader><CardTitle>Settings</CardTitle><CardDescription>Runtime listener and authentication settings.</CardDescription></CardHeader><CardContent className="space-y-6"><div className="grid gap-4 md:grid-cols-2"><Field label="Server host"><Input value={config.server.host} onChange={(event) => setConfig({ ...config, server: { ...config.server, host: event.target.value } })} /></Field><Field label="Server port"><Input type="number" min={1} max={65535} value={config.server.port} onChange={(event) => setConfig({ ...config, server: { ...config.server, port: Number(event.target.value) } })} /></Field><Field label="Admin port"><Input type="number" min={1} max={65535} value={config.admin.port} onChange={(event) => setConfig({ ...config, admin: { ...config.admin, port: Number(event.target.value) } })} /></Field></div><Toggle label="Admin enabled" checked={config.admin.enabled} onCheckedChange={(enabled) => setConfig({ ...config, admin: { ...config.admin, enabled } })} /><Toggle label="MCP authentication" checked={config.auth.mcp_enabled} onCheckedChange={(enabled) => setConfig({ ...config, auth: { ...config.auth, mcp_enabled: enabled } })} /><Toggle label="Admin authentication" checked={config.auth.admin_enabled} onCheckedChange={(enabled) => setConfig({ ...config, auth: { ...config.auth, admin_enabled: enabled } })} />{message ? <div className="text-sm text-muted-foreground">{message}</div> : null}{error ? <div className="text-sm text-destructive">{error}</div> : null}<Button onClick={save}>Save config</Button></CardContent></Card>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
function Toggle({ label, checked, onCheckedChange }: { label: string; checked: boolean; onCheckedChange: (checked: boolean) => void }) { return <div className="flex items-center justify-between rounded-lg border p-3"><Label>{label}</Label><Switch checked={checked} onCheckedChange={onCheckedChange} /></div> }
