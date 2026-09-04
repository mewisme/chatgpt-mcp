import { useEffect, useState, type ReactNode } from "react"
import { Activity, Cloud, KeyRound, Network, Power, RefreshCw, ShieldCheck } from "lucide-react"
import { CopyButton } from "@/components/copy-button"
import { DetailRow } from "@/components/detail-row"
import { PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { ScrollableTabsList, Tabs, TabsContent, TabsTrigger } from "@/components/ui/tabs"
import { adminApi, type TunnelAdminKeyRequest, type TunnelAdminKeyStatus, type TunnelAdminScope, type TunnelConfig, type TunnelStatus } from "@/lib/api"

const emptyConfig: TunnelConfig = { enabled: false }
type AdminScopeKind = "organization" | "workspace" | "tenant"

export function TunnelPage() {
  const [config, setConfig] = useState<TunnelConfig>(emptyConfig)
  const [status, setStatus] = useState<TunnelStatus | null>(null)
  const [adminConfigured, setAdminConfigured] = useState(false)
  const [adminTunnels, setAdminTunnels] = useState<number | undefined>()
  const [adminCurrentScope, setAdminCurrentScope] = useState<TunnelAdminScope>({})
  const [adminKey, setAdminKey] = useState("")
  const [adminScope, setAdminScope] = useState<AdminScopeKind>("workspace")
  const [adminScopeID, setAdminScopeID] = useState("")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [adminBusy, setAdminBusy] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [removeAdminOpen, setRemoveAdminOpen] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")

  function syncAdmin(value: TunnelAdminKeyStatus) {
    setAdminConfigured(value.configured); setAdminTunnels(value.tunnels); setAdminCurrentScope(value.scope)
    const [kind, id] = adminScopeValue(value.scope)
    if (kind) setAdminScope(kind)
    setAdminScopeID(id)
  }

  useEffect(() => {
    let active = true
    void Promise.all([adminApi.tunnelConfig(), adminApi.tunnel(), adminApi.tunnelAdminKey()]).then(([nextConfig, nextStatus, nextAdmin]) => {
      if (!active) return
      setConfig(nextConfig); setStatus(nextStatus); syncAdmin(nextAdmin); setError(""); setLoading(false)
    }).catch((value) => { if (active) { setError(errorText(value)); setLoading(false) } })
    const timer = window.setInterval(() => { void adminApi.tunnel().then((next) => { if (active) setStatus(next) }).catch(() => undefined) }, 3000)
    return () => { active = false; window.clearInterval(timer) }
  }, [])

  async function refresh() {
    setRefreshing(true)
    try {
      const [nextConfig, nextStatus, nextAdmin] = await Promise.all([adminApi.tunnelConfig(), adminApi.tunnel(), adminApi.tunnelAdminKey()])
      setConfig(nextConfig); setStatus(nextStatus); syncAdmin(nextAdmin); setError("")
    } catch (value) { setError(errorText(value)) } finally { setRefreshing(false) }
  }
  async function saveRuntime() {
    setBusy(true)
    try {
      setStatus(await adminApi.configureTunnel(runtimeConfig(config)))
      setConfig({ ...config, api_key: "", runtime_key_configured: Boolean(config.api_key || config.runtime_key_configured) })
      setMessage("Runtime tunnel configuration saved."); setError("")
    } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) }
  }
  async function toggle() {
    const active = status?.running || status?.restarting
    setBusy(true)
    try { setStatus(active ? await adminApi.stopTunnel() : await adminApi.startTunnel()); setMessage(active ? "Tunnel stopped." : "Tunnel start requested."); setError("") } catch (value) { setError(errorText(value)); setMessage("") } finally { setBusy(false) }
  }
  async function saveAdmin() {
    if (!adminScopeID.trim()) { setError("Admin scope ID is required."); return }
    setAdminBusy(true)
    try {
      const next = await adminApi.configureTunnelAdminKey(adminRequest(adminKey, adminScope, adminScopeID))
      setAdminKey(""); syncAdmin(next); setStatus(await adminApi.tunnel()); setMessage(adminResultMessage("Admin key verified and saved", next)); setError("")
    } catch (value) { setError(errorText(value)); setMessage("") } finally { setAdminBusy(false) }
  }
  async function verifyAdmin() {
    setAdminBusy(true)
    try { const next = await adminApi.verifyTunnelAdminKey(); syncAdmin(next); setMessage(adminResultMessage("Admin key verified", next)); setError("") } catch (value) { setError(errorText(value)); setMessage("") } finally { setAdminBusy(false) }
  }
  async function removeAdmin() {
    setAdminBusy(true)
    try {
      const next = await adminApi.removeTunnelAdminKey()
      setAdminKey(""); syncAdmin(next); setStatus(await adminApi.tunnel()); setRemoveAdminOpen(false); setMessage("Admin key removed."); setError("")
    } catch (value) { setError(errorText(value)); setMessage("") } finally { setAdminBusy(false) }
  }

  const active = status?.running || status?.restarting
  const state = status?.restarting ? "Reconnecting" : status?.running ? status.ready ? "Ready" : "Connecting" : "Stopped"
  const variant = status?.ready ? "default" : active ? "secondary" : "outline"
  return <div className="space-y-6">
    <PageHeader title="Tunnel" description="Operate the OpenAI Secure MCP Tunnel, runtime credential, and management access from one control surface." actions={<Button aria-label="Refresh tunnel status" disabled={refreshing} size="sm" variant="outline" onClick={() => void refresh()}><RefreshCw className={refreshing ? "animate-spin" : ""} />Refresh</Button>} />
    <PageError message={error} />
    {message ? <Alert><Network /><AlertDescription>{message}</AlertDescription></Alert> : null}
    {loading ? <PageLoading rows={5} /> : <>
      <TunnelHero active={active} busy={busy} config={config} state={state} status={status} variant={variant} adminConfigured={adminConfigured} onToggle={() => void toggle()} />
      <Tabs defaultValue="runtime" className="gap-4">
        <ScrollableTabsList variant="line" className="justify-start border-b"><TabsTrigger value="runtime"><Power />Runtime</TabsTrigger><TabsTrigger value="admin"><ShieldCheck />Administration</TabsTrigger><TabsTrigger value="metadata"><Activity />Metadata</TabsTrigger></ScrollableTabsList>
        <TabsContent value="runtime"><RuntimePanel busy={busy} config={config} setConfig={setConfig} onSave={() => void saveRuntime()} /></TabsContent>
        <TabsContent value="admin"><AdminPanel busy={adminBusy} configured={adminConfigured} currentScope={adminCurrentScope} tunnels={adminTunnels} keyValue={adminKey} scope={adminScope} scopeID={adminScopeID} setKey={setAdminKey} setScope={(value) => { setAdminScope(value); setAdminScopeID("") }} setScopeID={setAdminScopeID} onSave={() => void saveAdmin()} onVerify={() => void verifyAdmin()} onRemove={() => setRemoveAdminOpen(true)} /></TabsContent>
        <TabsContent value="metadata"><MetadataPanel config={config} status={status} /></TabsContent>
      </Tabs>
    </>}
    <AlertDialog open={removeAdminOpen} onOpenChange={(open) => { if (!adminBusy) setRemoveAdminOpen(open) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Remove tunnel admin key?</AlertDialogTitle><AlertDialogDescription>The stored Tunnels Manage credential and its verification scope will be removed. Runtime tunnel credentials are not changed.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={adminBusy}>Cancel</AlertDialogCancel><AlertDialogAction disabled={adminBusy} variant="destructive" onClick={() => void removeAdmin()}>{adminBusy ? "Removing..." : "Remove admin key"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>
}

function TunnelHero({ active, busy, config, state, status, variant, adminConfigured, onToggle }: { active: boolean | undefined; busy: boolean; config: TunnelConfig; state: string; status: TunnelStatus | null; variant: "default" | "secondary" | "outline"; adminConfigured: boolean; onToggle: () => void }) {
  const title = status?.metadata?.name || "OpenAI Secure MCP Tunnel"
  const description = status?.metadata?.description || "Secure connection between this runtime and ChatGPT through the OpenAI control plane."
  return <Card className="overflow-hidden"><CardHeader className="border-b"><div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div className="flex min-w-0 gap-3"><div className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted/40"><Cloud className="size-5 text-muted-foreground" /></div><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><CardTitle>{title}</CardTitle><Badge variant={variant}>{status?.restarting || (status?.running && !status.ready) ? <Spinner className="size-3" /> : null}{state}</Badge></div><CardDescription className="mt-1 max-w-3xl">{description}</CardDescription></div></div><Button disabled={busy || !config.enabled} variant={active ? "outline" : "default"} onClick={onToggle}><Power />{busy ? "Working..." : active ? "Stop tunnel" : "Start tunnel"}</Button></div></CardHeader><CardContent className="p-0"><div className="grid sm:grid-cols-2 xl:grid-cols-4"><HeroMetric label="Tunnel ID" value={<CopyValue value={status?.metadata?.id ?? status?.id ?? config.id ?? "-"} />} description={status?.provider || "openai"} /><HeroMetric label="Runtime key" value={config.runtime_key_configured ? "Configured" : "Not configured"} description={config.enabled ? "Tunnel enabled" : "Tunnel disabled"} /><HeroMetric label="Admin access" value={adminConfigured ? "Configured" : "Not configured"} description={status?.admin_key_configured ? formatAdminScope(status.admin_scope) : "Tunnels Manage"} /><HeroMetric label="Started" value={status?.started_at ? formatDate(status.started_at) : "-"} description={status?.ready ? "Connection ready" : state} /></div>{status?.last_error || status?.metadata_error ? <div className="border-t p-4"><div className="break-words rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{status.last_error || `Tunnel metadata unavailable: ${status.metadata_error}`}</div></div> : null}</CardContent></Card>
}

function HeroMetric({ label, value, description }: { label: string; value: ReactNode; description: string }) { return <div className="min-w-0 border-b p-4 last:border-b-0 sm:[&:nth-child(odd)]:border-r xl:border-b-0 xl:border-r xl:last:border-r-0"><div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</div><div className="mt-2 min-w-0 text-sm font-medium">{value}</div><div className="mt-1 truncate text-xs text-muted-foreground">{description}</div></div> }

function RuntimePanel({ busy, config, setConfig, onSave }: { busy: boolean; config: TunnelConfig; setConfig: (value: TunnelConfig) => void; onSave: () => void }) {
  return <Card><CardHeader><div className="flex flex-wrap items-start justify-between gap-3"><div><CardTitle>Runtime connectivity</CardTitle><CardDescription className="mt-1">Read + Use credential and connection settings used by the running cgm tunnel client.</CardDescription></div><Badge variant={config.runtime_key_configured ? "secondary" : "outline"}><KeyRound />{config.runtime_key_configured ? "Runtime key configured" : "Runtime key missing"}</Badge></div></CardHeader><CardContent className="space-y-6"><Field orientation="horizontal" className="rounded-lg border p-4"><div className="min-w-0 flex-1"><FieldLabel>Enable tunnel</FieldLabel><FieldDescription>Allow the managed runtime to connect to this OpenAI tunnel. Disabled tunnels stay stopped.</FieldDescription></div><Switch checked={config.enabled} onCheckedChange={(enabled) => setConfig({ ...config, enabled })} /></Field><FieldGroup><div className="grid gap-5 md:grid-cols-2"><ConfigField label="Tunnel ID" description="Assigned OpenAI tunnel identifier."><Input placeholder="tunnel_..." value={config.id ?? ""} onChange={(event) => setConfig({ ...config, id: event.target.value })} /></ConfigField><ConfigField label="Runtime key" description={config.runtime_key_configured ? "Leave blank to keep the stored Read + Use key." : "OpenAI runtime key with Read + Use permissions."}><Input autoComplete="off" placeholder={config.runtime_key_configured ? "Leave blank to keep current key" : "Runtime API key"} type="password" value={config.api_key ?? ""} onChange={(event) => setConfig({ ...config, api_key: event.target.value })} /></ConfigField><ConfigField label="Control plane base URL" description="Leave empty to use the OpenAI default endpoint."><Input placeholder="https://api.openai.com" value={config.control_plane_base_url ?? ""} onChange={(event) => setConfig({ ...config, control_plane_base_url: event.target.value })} /></ConfigField><ConfigField label="Organization ID" description="Optional runtime organization scope."><Input placeholder="org_..." value={config.organization_id ?? ""} onChange={(event) => setConfig({ ...config, organization_id: event.target.value })} /></ConfigField></div></FieldGroup></CardContent><CardFooter className="justify-end border-t"><Button disabled={busy} onClick={onSave}>{busy ? "Saving..." : "Save runtime configuration"}</Button></CardFooter></Card>
}

function AdminPanel({ busy, configured, currentScope, tunnels, keyValue, scope, scopeID, setKey, setScope, setScopeID, onSave, onVerify, onRemove }: { busy: boolean; configured: boolean; currentScope: TunnelAdminScope; tunnels?: number; keyValue: string; scope: AdminScopeKind; scopeID: string; setKey: (value: string) => void; setScope: (value: AdminScopeKind) => void; setScopeID: (value: string) => void; onSave: () => void; onVerify: () => void; onRemove: () => void }) {
  return <Card><CardHeader><div className="flex flex-wrap items-start justify-between gap-3"><div><CardTitle>Tunnel administration</CardTitle><CardDescription className="mt-1">Tunnels Manage credential for listing and managing tunnel metadata. This key is never reused by the runtime connection.</CardDescription></div><div className="flex flex-wrap gap-2"><Badge variant={configured ? "secondary" : "outline"}>{configured ? "Management configured" : "Management not configured"}</Badge>{tunnels !== undefined ? <Badge variant="outline">{tunnels} accessible tunnel{tunnels === 1 ? "" : "s"}</Badge> : null}</div></div></CardHeader><CardContent className="space-y-6">{configured ? <div className="grid gap-3 rounded-lg border bg-muted/20 p-4 md:grid-cols-2"><SummaryLine label="Current scope" value={formatAdminScope(currentScope)} /><SummaryLine label="Credential storage" value="Secret file store" /></div> : <Alert><ShieldCheck /><AlertDescription>Add an admin API key with Tunnels Manage permission and verify it against exactly one organization, workspace, or tenant scope.</AlertDescription></Alert>}<FieldGroup><ConfigField label="Admin key" description={configured ? "Leave blank to keep the stored admin key while changing or re-verifying scope." : "The key is stored only after verification succeeds."}><Input autoComplete="off" placeholder={configured ? "Leave blank to keep current key" : "Admin API key"} type="password" value={keyValue} onChange={(event) => setKey(event.target.value)} /></ConfigField><div className="grid gap-5 md:grid-cols-2"><ConfigField label="Scope type" description="Management verification uses exactly one scope."><Select value={scope} onValueChange={(value) => setScope(value as AdminScopeKind)}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="organization">Organization</SelectItem><SelectItem value="workspace">Workspace</SelectItem><SelectItem value="tenant">Tenant</SelectItem></SelectContent></Select></ConfigField><ConfigField label={`${scopeLabel(scope)} ID`} description={`OpenAI ${scope} used to verify Tunnels Manage access.`}><Input placeholder={scopePlaceholder(scope)} value={scopeID} onChange={(event) => setScopeID(event.target.value)} /></ConfigField></div></FieldGroup></CardContent><CardFooter className="flex-col items-stretch gap-3 border-t sm:flex-row sm:items-center sm:justify-between"><div className="text-xs text-muted-foreground">The secret file store keeps credentials outside tunnel.&lt;ext&gt;; configuration stores only scope and configured-state metadata.</div><ButtonGroup className="self-end sm:self-auto">{configured ? <><Button disabled={busy} variant="outline" onClick={onVerify}>Verify</Button><Button disabled={busy} variant="outline" onClick={onRemove}>Remove</Button></> : null}<Button disabled={busy || !scopeID.trim()} onClick={onSave}>{busy ? "Verifying..." : "Save & verify"}</Button></ButtonGroup></CardFooter></Card>
}

function MetadataPanel({ config, status }: { config: TunnelConfig; status: TunnelStatus | null }) {
  const metadata = status?.metadata
  return <div className="grid gap-4 xl:grid-cols-2"><Card><CardHeader><CardTitle>Connection identity</CardTitle><CardDescription>Current runtime and control-plane identity.</CardDescription></CardHeader><CardContent className="divide-y"><DetailRow label="Provider" value={status?.provider ?? "-"} mono /><DetailRow label="Tunnel ID" value={<CopyValue value={metadata?.id ?? status?.id ?? config.id ?? "-"} />} /><DetailRow label="Name" value={metadata?.name || "-"} /><DetailRow label="Description" value={metadata?.description || "-"} /><DetailRow label="Creator" value={metadata?.creator || "-"} mono /><DetailRow label="Request ID" value={<CopyValue value={metadata?.request_id || "-"} />} /><DetailRow label="Fetched" value={metadata?.fetched_at ? formatDate(metadata.fetched_at) : "-"} /><DetailRow label="Control plane" value={<CopyValue value={status?.control_plane_base_url ?? config.control_plane_base_url ?? "Default"} />} /><DetailRow label="Runtime organization" value={<CopyValue value={status?.organization_id ?? config.organization_id ?? "-"} />} /></CardContent></Card><Card><CardHeader><CardTitle>Tunnel scope</CardTitle><CardDescription>Organizations, workspaces, and tenants attached to the current tunnel metadata.</CardDescription></CardHeader><CardContent className="divide-y"><DetailRow label="Organizations" value={<CopyList values={metadata?.organization_ids} />} /><DetailRow label="Workspaces" value={<CopyList values={metadata?.workspace_ids} />} /><DetailRow label="Tenants" value={<CopyList values={metadata?.tenant_ids} />} /><DetailRow label="Admin scope" value={status?.admin_key_configured ? formatAdminScope(status.admin_scope) : "Not configured"} /></CardContent></Card></div>
}

function SummaryLine({ label, value }: { label: string; value: string }) { return <div><div className="text-xs text-muted-foreground">{label}</div><div className="mt-1 break-all text-sm font-medium">{value}</div></div> }
function ConfigField({ label, description, children }: { label: string; description: string; children: ReactNode }) { return <Field><FieldLabel>{label}</FieldLabel>{children}<FieldDescription>{description}</FieldDescription></Field> }
function CopyValue({ value }: { value: string }) { return <div className="flex min-w-0 items-start gap-1"><span className="min-w-0 flex-1 break-all font-mono text-sm font-normal">{value}</span>{value !== "-" && value !== "Default" ? <CopyButton value={value} /> : null}</div> }
function CopyList({ values }: { values?: string[] }) { return values?.length ? <div className="space-y-1">{values.map((value) => <CopyValue key={value} value={value} />)}</div> : "-" }
function runtimeConfig(config: TunnelConfig): TunnelConfig { return { enabled: config.enabled, id: config.id, api_key: config.api_key, control_plane_base_url: config.control_plane_base_url, organization_id: config.organization_id } }
function adminRequest(key: string, scope: AdminScopeKind, id: string): TunnelAdminKeyRequest { const request: TunnelAdminKeyRequest = { admin_key: key.trim() || undefined }; if (scope === "organization") request.organization_id = id.trim(); else if (scope === "workspace") request.workspace_id = id.trim(); else request.tenant_id = id.trim(); return request }
function adminScopeValue(scope?: TunnelAdminScope): [AdminScopeKind | undefined, string] { if (scope?.organization_id) return ["organization", scope.organization_id]; if (scope?.workspace_id) return ["workspace", scope.workspace_id]; if (scope?.tenant_id) return ["tenant", scope.tenant_id]; return [undefined, ""] }
function formatAdminScope(scope?: TunnelAdminScope) { const [kind, id] = adminScopeValue(scope); return kind && id ? `${kind}:${id}` : "scope unavailable" }
function adminResultMessage(message: string, result: TunnelAdminKeyStatus) { return result.tunnels === undefined ? `${message}.` : `${message} · ${result.tunnels} tunnel${result.tunnels === 1 ? "" : "s"} accessible.` }
function scopeLabel(scope: AdminScopeKind) { return scope[0].toUpperCase() + scope.slice(1) }
function scopePlaceholder(scope: AdminScopeKind) { return scope === "organization" ? "org_..." : scope === "workspace" ? "Workspace ID" : "Tenant ID" }
function formatDate(value: string) { const date = new Date(value); return Number.isNaN(date.getTime()) ? value : date.toLocaleString() }
function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
