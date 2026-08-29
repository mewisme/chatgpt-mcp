import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { adminApi, type ConfigPresetList, type PublicConfig } from "@/lib/api"

export function SettingsPage() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [presets, setPresets] = useState<ConfigPresetList | null>(null)
  const [selectedPreset, setSelectedPreset] = useState("")
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")

  useEffect(() => {
    void Promise.all([adminApi.config(), adminApi.configPresets()]).then(([nextConfig, nextPresets]) => {
      setConfig(nextConfig)
      setPresets(nextPresets)
      setSelectedPreset(nextPresets.current === "custom" ? "" : nextPresets.current)
    }).catch((value) => setError(errorText(value)))
  }, [])

  async function save() {
    if (!config) return
    setBusy(true)
    try {
      const next = await adminApi.saveConfig(config)
      const nextPresets = await adminApi.configPresets()
      setConfig(next)
      setPresets(nextPresets)
      setSelectedPreset(nextPresets.current === "custom" ? "" : nextPresets.current)
      setMessage("Saved. Restart the server to apply listener/auth changes.")
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
    } finally {
      setBusy(false)
    }
  }

  async function applyPreset() {
    if (!selectedPreset) return
    setBusy(true)
    try {
      const next = await adminApi.applyConfigPreset(selectedPreset)
      const nextPresets = await adminApi.configPresets()
      setConfig(next)
      setPresets(nextPresets)
      setSelectedPreset(nextPresets.current === "custom" ? selectedPreset : nextPresets.current)
      setMessage(`Preset ${selectedPreset} applied. Secrets were preserved. Restart the server to apply listener/auth changes.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
    } finally {
      setBusy(false)
    }
  }

  if (!config || !presets) return <div className="text-sm text-muted-foreground">{error || "Loading..."}</div>
  return <div className="space-y-6"><Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-2"><div><CardTitle>Config preset</CardTitle><CardDescription>Apply the same built-in presets used by the CLI. Token hashes and tunnel credentials/details are preserved.</CardDescription></div><Badge variant="secondary">{presets.current}</Badge></div></CardHeader><CardContent><div className="flex flex-col gap-3 sm:flex-row"><Select value={selectedPreset} onValueChange={setSelectedPreset}><SelectTrigger className="w-full sm:w-64"><SelectValue placeholder="Select preset" /></SelectTrigger><SelectContent>{presets.presets.map((preset) => <SelectItem key={preset.name} value={preset.name}>{preset.name}</SelectItem>)}</SelectContent></Select><Button disabled={busy || !selectedPreset} variant="outline" onClick={applyPreset}>Apply preset</Button></div>{selectedPreset ? <div className="mt-3 text-sm text-muted-foreground">{presets.presets.find((preset) => preset.name === selectedPreset)?.description}</div> : null}</CardContent></Card><Card><CardHeader><CardTitle>Runtime</CardTitle><CardDescription>Listener and admin UI settings.</CardDescription></CardHeader><CardContent className="space-y-5"><div className="grid gap-4 md:grid-cols-2"><Field label="Server port"><Input type="number" min={1} max={65535} value={config.server.port} onChange={(event) => setConfig({ ...config, server: { ...config.server, port: Number(event.target.value) } })} /></Field><Field label="Admin port"><Input type="number" min={1} max={65535} value={config.admin.port} onChange={(event) => setConfig({ ...config, admin: { ...config.admin, port: Number(event.target.value) } })} /></Field></div><Toggle label="Expose to local network" checked={config.server.expose} onCheckedChange={(expose) => setConfig({ ...config, server: { ...config.server, expose } })} /><Toggle label="Admin enabled" checked={config.admin.enabled} onCheckedChange={(enabled) => setConfig({ ...config, admin: { ...config.admin, enabled } })} /></CardContent></Card><Card><CardHeader><CardTitle>Authentication</CardTitle><CardDescription>Tokens are managed by the CLI and are never returned by the Admin API.</CardDescription></CardHeader><CardContent className="space-y-3"><AuthToggle label="MCP authentication" configured={config.auth.mcp_token_configured} checked={config.auth.mcp_enabled} command="chatgpt-mcp auth mcp-create" onCheckedChange={(enabled) => setConfig({ ...config, auth: { ...config.auth, mcp_enabled: enabled } })} /><AuthToggle label="Admin authentication" configured={config.auth.admin_token_configured} checked={config.auth.admin_enabled} command="chatgpt-mcp auth admin-create" onCheckedChange={(enabled) => setConfig({ ...config, auth: { ...config.auth, admin_enabled: enabled } })} /></CardContent></Card>{message ? <div className="text-sm text-muted-foreground">{message}</div> : null}{error ? <div className="text-sm text-destructive">{error}</div> : null}<Button disabled={busy} onClick={save}>Save config</Button></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
function Toggle({ label, checked, onCheckedChange }: { label: string; checked: boolean; onCheckedChange: (checked: boolean) => void }) { return <div className="flex items-center justify-between rounded-lg border p-3"><Label>{label}</Label><Switch checked={checked} onCheckedChange={onCheckedChange} /></div> }
function AuthToggle({ label, configured, command, checked, onCheckedChange }: { label: string; configured: boolean; command: string; checked: boolean; onCheckedChange: (checked: boolean) => void }) {
  return <div className="flex items-center gap-4 rounded-lg border p-3"><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><Label>{label}</Label><Badge variant={configured ? "secondary" : "outline"}>{configured ? "Token configured" : "Token missing"}</Badge></div>{!configured ? <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{command}</div> : null}</div><Switch checked={checked} disabled={!configured && !checked} onCheckedChange={onCheckedChange} /></div>
}
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
