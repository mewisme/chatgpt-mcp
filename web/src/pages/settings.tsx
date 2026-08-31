import { useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { adminApi, type ConfigPresetList, type NetworkInterface, type PublicConfig } from "@/lib/api"

export function SettingsPage() {
  const [config, setConfig] = useState<PublicConfig | null>(null)
  const [presets, setPresets] = useState<ConfigPresetList | null>(null)
  const [interfaces, setInterfaces] = useState<NetworkInterface[]>([])
  const [selectedPreset, setSelectedPreset] = useState("")
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")

  useEffect(() => {
    void Promise.all([adminApi.config(), adminApi.configPresets(), adminApi.networkInterfaces()]).then(([nextConfig, nextPresets, nextInterfaces]) => {
      setConfig(nextConfig); setPresets(nextPresets); setInterfaces(nextInterfaces); setSelectedPreset(nextPresets.current === "custom" ? "" : nextPresets.current)
    }).catch((value) => setError(errorText(value)))
  }, [])

  async function save() {
    if (!config) return
    setBusy(true)
    try {
      const next = await adminApi.saveConfig(config)
      const nextPresets = await adminApi.configPresets()
      setConfig(next); setPresets(nextPresets); setSelectedPreset(nextPresets.current === "custom" ? "" : nextPresets.current)
      setMessage("Saved. Feature, auth, and filesystem access changes apply immediately; restart the server for listener changes."); setError("")
    } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) }
  }

  async function applyPreset() {
    if (!selectedPreset) return
    setBusy(true)
    try {
      const next = await adminApi.applyConfigPreset(selectedPreset)
      const nextPresets = await adminApi.configPresets()
      setConfig(next); setPresets(nextPresets); setSelectedPreset(nextPresets.current === "custom" ? selectedPreset : nextPresets.current)
      setMessage(`Preset ${selectedPreset} applied. Secrets were preserved. Feature and auth changes apply immediately; restart the server for listener changes.`); setError("")
    } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) }
  }

  function setExposureMode(mode: PublicConfig["server"]["expose"]["mode"]) {
    if (!config) return
    const current = config.server.expose.interfaces
    const exposed = mode !== "none"
    setConfig({ ...config, server: { ...config.server, expose: { mode, interfaces: mode === "interfaces" ? current : [] } }, auth: exposed ? { ...config.auth, mcp_enabled: true, admin_enabled: config.admin.enabled ? true : config.auth.admin_enabled } : config.auth })
  }

  function toggleInterface(name: string, checked: boolean) {
    if (!config) return
    const selected = new Set(config.server.expose.interfaces)
    if (checked) selected.add(name); else selected.delete(name)
    setConfig({ ...config, server: { ...config.server, expose: { mode: "interfaces", interfaces: [...selected].sort() } } })
  }

  if (!config || !presets) return <div className="text-sm text-muted-foreground">{error || "Loading..."}</div>
  const selectedInterfaces = new Set(config.server.expose.interfaces)
  const exposed = config.server.expose.mode !== "none"
  const exposureAuthReady = !exposed || (config.auth.mcp_enabled && config.auth.mcp_token_configured && (!config.admin.enabled || (config.auth.admin_enabled && config.auth.admin_token_configured)))
  return <div className="space-y-6"><Card><CardHeader><div className="flex flex-wrap items-center justify-between gap-2"><div><CardTitle>Config preset</CardTitle><CardDescription>Apply the same built-in presets used by the CLI. Token hashes and tunnel credentials/details are preserved.</CardDescription></div><Badge variant="secondary">{presets.current}</Badge></div></CardHeader><CardContent><div className="flex flex-col gap-3 sm:flex-row"><Select value={selectedPreset} onValueChange={setSelectedPreset}><SelectTrigger className="w-full sm:w-64"><SelectValue placeholder="Select preset" /></SelectTrigger><SelectContent>{presets.presets.map((preset) => <SelectItem key={preset.name} value={preset.name}>{preset.name}</SelectItem>)}</SelectContent></Select><Button disabled={busy || !selectedPreset} variant="outline" onClick={applyPreset}>Apply preset</Button></div>{selectedPreset ? <div className="mt-3 text-sm text-muted-foreground">{presets.presets.find((preset) => preset.name === selectedPreset)?.description}</div> : null}</CardContent></Card><Card><CardHeader><CardTitle>Runtime</CardTitle><CardDescription>Listener and admin UI settings.</CardDescription></CardHeader><CardContent className="space-y-5"><div className="grid gap-4 md:grid-cols-2"><Field label="Server port"><Input type="number" min={1} max={65535} value={config.server.port} onChange={(event) => setConfig({ ...config, server: { ...config.server, port: Number(event.target.value) } })} /></Field><Field label="Admin port"><Input type="number" min={1} max={65535} value={config.admin.port} onChange={(event) => setConfig({ ...config, admin: { ...config.admin, port: Number(event.target.value) } })} /></Field></div><div className="space-y-3"><Label>Network exposure</Label><RadioGroup value={config.server.expose.mode} onValueChange={(value) => setExposureMode(value as PublicConfig["server"]["expose"]["mode"])}><ExposureOption value="none" title="Local only" description="Bind MCP and Admin only to 127.0.0.1." /><ExposureOption value="all" title="All active interfaces" description="Bind loopback plus every currently active eligible IPv4 address." /><ExposureOption value="interfaces" title="Selected interfaces" description="Keep loopback and additionally bind only to selected interface IPv4 addresses." /><ExposureOption value="0.0.0.0" title="Wildcard 0.0.0.0" description="Bind every IPv4 interface, including interfaces that appear later. MCP and Admin authentication are mandatory." /></RadioGroup>{config.server.expose.mode === "interfaces" ? <div className="space-y-2 rounded-lg border p-3">{interfaces.length === 0 ? <div className="text-sm text-muted-foreground">No eligible active IPv4 interfaces detected.</div> : interfaces.map((iface) => <label className="flex cursor-pointer items-start gap-3 rounded-md px-2 py-2 hover:bg-muted/50" key={iface.name}><Checkbox checked={selectedInterfaces.has(iface.name)} onCheckedChange={(checked) => toggleInterface(iface.name, checked === true)} /><div className="min-w-0 flex-1"><div className="font-mono text-sm">{iface.name}</div><div className="mt-1 flex flex-wrap gap-2">{iface.addresses.map((address) => <Badge key={address.address} variant="outline">{address.address} · {address.scope}</Badge>)}</div></div></label>)}</div> : null}{exposed && !exposureAuthReady ? <div className="text-sm text-destructive">Network exposure requires configured MCP authentication and, when Admin is enabled, configured Admin authentication.</div> : null}{exposed ? <div className="space-y-2"><Toggle label="Allow authenticated HTTP beyond loopback" checked={config.server.allow_insecure_http} onCheckedChange={(allow_insecure_http) => setConfig({ ...config, server: { ...config.server, allow_insecure_http } })} />{!config.server.allow_insecure_http ? <div className="text-sm text-destructive">Direct non-loopback listeners use unencrypted HTTP. Enable this only on a trusted or encrypted network, or use Secure MCP Tunnel / a TLS reverse proxy.</div> : null}</div> : null}</div><Toggle label="Admin enabled" checked={config.admin.enabled} onCheckedChange={(enabled) => setConfig({ ...config, admin: { ...config.admin, enabled } })} /></CardContent></Card><Card><CardHeader><CardTitle>Filesystem access</CardTitle><CardDescription>Global directories available to every registered workspace in addition to its own root.</CardDescription></CardHeader><CardContent><Field label="Allowed directories"><Input placeholder="/tmp, /var/tmp/chatgpt-mcp" value={config.permissions.allow_dirs.join(", ")} onChange={(event) => setConfig({ ...config, permissions: { allow_dirs: parseAllowDirs(event.target.value) } })} /></Field><div className="mt-2 text-xs text-muted-foreground">Use absolute existing directories. Workspace-specific access can be managed with chatgpt-mcp workspace access.</div></CardContent></Card><Card><CardHeader><CardTitle>Built-in features</CardTitle><CardDescription>Feature tool registration updates immediately when you save.</CardDescription></CardHeader><CardContent className="space-y-3"><Toggle label="Ponytail" checked={config.features.ponytail.enabled} onCheckedChange={(enabled) => setConfig({ ...config, features: { ...config.features, ponytail: { enabled } } })} /><Toggle label="Caveman" checked={config.features.caveman.enabled} onCheckedChange={(enabled) => setConfig({ ...config, features: { ...config.features, caveman: { enabled } } })} /></CardContent></Card><Card><CardHeader><CardTitle>Authentication</CardTitle><CardDescription>Authentication is mandatory for direct network exposure. Admin authentication is required when the Admin endpoint is exposed.</CardDescription></CardHeader><CardContent className="space-y-3"><AuthToggle locked={exposed} label="MCP authentication" configured={config.auth.mcp_token_configured} checked={config.auth.mcp_enabled} command="chatgpt-mcp auth mcp create" onCheckedChange={(enabled) => setConfig({ ...config, auth: { ...config.auth, mcp_enabled: enabled } })} /><AuthToggle locked={exposed && config.admin.enabled} label="Admin authentication" configured={config.auth.admin_token_configured} checked={config.auth.admin_enabled} command="chatgpt-mcp auth admin create" onCheckedChange={(enabled) => setConfig({ ...config, auth: { ...config.auth, admin_enabled: enabled } })} /></CardContent></Card>{message ? <div className="text-sm text-muted-foreground">{message}</div> : null}{error ? <div className="text-sm text-destructive">{error}</div> : null}<Button disabled={busy || (config.server.expose.mode === "interfaces" && config.server.expose.interfaces.length === 0) || !exposureAuthReady || (exposed && !config.server.allow_insecure_http)} onClick={save}>Save config</Button></div>
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <div className="space-y-2"><Label>{label}</Label>{children}</div> }
function Toggle({ label, checked, onCheckedChange }: { label: string; checked: boolean; onCheckedChange: (checked: boolean) => void }) { return <div className="flex items-center justify-between rounded-lg border p-3"><Label>{label}</Label><Switch checked={checked} onCheckedChange={onCheckedChange} /></div> }
function ExposureOption({ value, title, description }: { value: string; title: string; description: string }) { return <label className="flex cursor-pointer items-start gap-3 rounded-lg border p-3"><RadioGroupItem className="mt-0.5" value={value} /><div><div className="text-sm font-medium">{title}</div><div className="text-xs text-muted-foreground">{description}</div></div></label> }
function AuthToggle({ label, configured, command, checked, locked = false, onCheckedChange }: { label: string; configured: boolean; command: string; checked: boolean; locked?: boolean; onCheckedChange: (checked: boolean) => void }) { return <div className="flex items-center gap-4 rounded-lg border p-3"><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><Label>{label}</Label><Badge variant={configured ? "secondary" : "outline"}>{configured ? "Token configured" : "Token missing"}</Badge></div>{!configured ? <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{command}</div> : null}</div><Switch checked={checked} disabled={locked || (!configured && !checked)} onCheckedChange={onCheckedChange} /></div> }
function parseAllowDirs(value: string) { return value.split(",").map((item) => item.trim()).filter(Boolean) }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
