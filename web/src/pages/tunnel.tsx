import { useEffect, useState } from "react"
import { Network, RefreshCw } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { DetailRow } from "@/components/detail-row"
import { PageError } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
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
    void Promise.all([adminApi.tunnelConfig(), adminApi.tunnel()]).then(([nextConfig, nextStatus]) => { if (active) { setConfig(nextConfig); setStatus(nextStatus); setError(""); setLoading(false) } }).catch((value) => { if (active) { setError(errorText(value)); setLoading(false) } })
    const timer = window.setInterval(() => { void adminApi.tunnel().then((next) => { if (active) setStatus(next) }).catch(() => undefined) }, 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])

  async function refresh() { setRefreshing(true); try { setStatus(await adminApi.tunnel()); setError("") } catch (value) { setError(errorText(value)) } finally { setRefreshing(false) } }
  async function save() { setBusy(true); try { setStatus(await adminApi.configureTunnel(config)); setMessage("Tunnel configuration saved."); setError("") } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) } }
  async function toggle() { const active = status?.running || status?.restarting; setBusy(true); try { setStatus(active ? await adminApi.stopTunnel() : await adminApi.startTunnel()); setMessage(active ? "Tunnel stopped." : "Tunnel start requested."); setError("") } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) } }

  const active = status?.running || status?.restarting
  const state = status?.restarting ? "Reconnecting" : status?.running ? status.ready ? "Ready" : "Connecting" : "Stopped"
  const variant = status?.ready ? "default" : active ? "secondary" : "outline"
  return <div className="space-y-6"><PageHeader title="Tunnel" description="Configure and monitor the builtin OpenAI Secure MCP Tunnel without exposing a public MCP listener." actions={<Button aria-label="Refresh tunnel status" disabled={refreshing} size="sm" variant="outline" onClick={() => void refresh()}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button>} /><PageError message={error} />{message ? <Alert><Network /><AlertDescription>{message}</AlertDescription></Alert> : null}<div className="grid gap-6 xl:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)]"><Card className="h-fit"><CardHeader><div className="flex items-start justify-between gap-3"><div><CardTitle>{status?.metadata?.name || "OpenAI Secure MCP Tunnel"}</CardTitle><CardDescription className="mt-1">{status?.metadata?.description || "Current control-plane connection and runtime state."}</CardDescription></div><Badge variant={variant}>{status?.restarting || (status?.running && !status.ready) ? <Spinner className="size-3" /> : null}{loading ? "Loading" : state}</Badge></div></CardHeader><CardContent className="divide-y"><DetailRow label="Provider" value={status?.provider ?? "-"} /><DetailRow label="Tunnel ID" value={<CopyValue value={status?.metadata?.id ?? status?.id ?? config.id ?? "-"} />} /><DetailRow label="Started" value={status?.started_at ? formatDate(status.started_at) : "-"} /><DetailRow label="Creator" value={status?.metadata?.creator ?? "-"} /><DetailRow label="Workspaces" value={<CopyList values={status?.metadata?.workspace_ids} />} /><DetailRow label="Organizations" value={<CopyList values={status?.metadata?.organization_ids} />} /><DetailRow label="Control plane" value={<CopyValue value={status?.control_plane_base_url ?? config.control_plane_base_url ?? "Default"} />} /></CardContent>{status?.last_error || status?.metadata_error ? <CardFooter><div className="w-full break-words rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{status.last_error || `Tunnel metadata unavailable: ${status.metadata_error}`}</div></CardFooter> : null}<CardFooter className="justify-end border-t"><ButtonGroup><Button disabled={busy || loading || !config.enabled} variant="outline" onClick={() => void toggle()}>{active ? "Stop tunnel" : "Start tunnel"}</Button><Button disabled={busy || loading} onClick={() => void save()}>{busy ? "Working..." : "Save configuration"}</Button></ButtonGroup></CardFooter></Card><Card><CardHeader><CardTitle>Configuration</CardTitle><CardDescription>Connection identity and startup behavior. Blank secrets keep their current stored value.</CardDescription></CardHeader><CardContent><FieldGroup><Field orientation="horizontal"><div className="min-w-0 flex-1"><FieldLabel>Start automatically</FieldLabel><FieldDescription>Connect to the OpenAI Tunnel control plane when the runtime starts.</FieldDescription></div><Switch checked={config.enabled} disabled={loading} onCheckedChange={(enabled) => setConfig({ ...config, enabled })} /></Field><div className="grid gap-5 md:grid-cols-2"><ConfigField label="Tunnel ID" description="Assigned OpenAI tunnel identifier."><Input placeholder="tunnel_..." value={config.id ?? ""} onChange={(event) => setConfig({ ...config, id: event.target.value })} /></ConfigField><ConfigField label="Runtime key" description="Leave blank to keep the current stored secret."><Input autoComplete="off" placeholder="Leave blank to keep current key" type="password" value={config.api_key ?? ""} onChange={(event) => setConfig({ ...config, api_key: event.target.value })} /></ConfigField><ConfigField label="Control plane base URL" description="Usually left empty for the OpenAI default."><Input placeholder="https://api.openai.com" value={config.control_plane_base_url ?? ""} onChange={(event) => setConfig({ ...config, control_plane_base_url: event.target.value })} /></ConfigField><ConfigField label="Organization ID" description="Optional OpenAI organization scope."><Input placeholder="org_..." value={config.organization_id ?? ""} onChange={(event) => setConfig({ ...config, organization_id: event.target.value })} /></ConfigField></div></FieldGroup></CardContent></Card></div></div>
}

function ConfigField({ label, description, children }: { label: string; description: string; children: React.ReactNode }) { return <Field><FieldLabel>{label}</FieldLabel>{children}<FieldDescription>{description}</FieldDescription></Field> }
function CopyValue({ value }: { value: string }) { return <div className="flex min-w-0 items-start gap-1"><span className="min-w-0 flex-1 break-all font-mono text-sm font-normal">{value}</span>{value !== "-" && value !== "Default" ? <CopyButton value={value} /> : null}</div> }
function CopyList({ values }: { values?: string[] }) { return values?.length ? <div className="space-y-1">{values.map((value) => <CopyValue key={value} value={value} />)}</div> : "-" }
function formatDate(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }