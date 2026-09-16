import { useEffect, useMemo, useState } from "react"
import {
  Cloud,
  KeyRound,
  Network,
  Plus,
  Power,
  RefreshCw,
  ShieldCheck,
} from "lucide-react"
import { PageError, PageLoading } from "@/components/page-state"
import { PageHeader } from "@/components/page-header"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
  ScrollableTabsList,
  Tabs,
  TabsContent,
  TabsTrigger,
} from "@/components/ui/tabs"
import {
  adminApi,
  type LocalTunnel,
  type LocalTunnelRequest,
  type ManagedTunnel,
  type ManagedTunnelCreateRequest,
  type ManagedTunnelUpdateRequest,
  type TunnelAdminProfile,
  type TunnelAdminProfileRequest,
  type TunnelProvider,
} from "@/lib/api"

type AdminScopeKind = "organization" | "workspace" | "tenant"
type RuntimeMode = "auto" | "manual"

export function TunnelPage() {
  const [locals, setLocals] = useState<LocalTunnel[]>([])
  const [admins, setAdmins] = useState<TunnelAdminProfile[]>([])
  const [managed, setManaged] = useState<ManagedTunnel[]>([])
  const [providers, setProviders] = useState<TunnelProvider[]>([])
  const [managedProfile, setManagedProfile] = useState("all")
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState("")
  const [refreshing, setRefreshing] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")

  async function refresh(profile = managedProfile) {
    setRefreshing(true)
    try {
      const [nextLocals, nextAdmins, nextManaged, nextProviders] = await Promise.all([
        adminApi.localTunnels(),
        adminApi.tunnelAdminProfiles(),
        adminApi.managedTunnelCollection(profile === "all" ? "" : profile),
        adminApi.tunnelProviders(),
      ])
      setLocals(nextLocals)
      setAdmins(nextAdmins)
      setManaged(nextManaged)
      setProviders(nextProviders)
      setError("")
    } catch (value) {
      setError(errorText(value))
    } finally {
      setRefreshing(false)
      setLoading(false)
    }
  }

  useEffect(() => {
    let active = true
    void Promise.all([
      adminApi.localTunnels(),
      adminApi.tunnelAdminProfiles(),
      adminApi.managedTunnelCollection(),
      adminApi.tunnelProviders(),
    ])
      .then(([nextLocals, nextAdmins, nextManaged, nextProviders]) => {
        if (!active) return
        setLocals(nextLocals)
        setAdmins(nextAdmins)
        setManaged(nextManaged)
        setProviders(nextProviders as TunnelProvider[])
        setLoading(false)
      })
      .catch((value) => {
        if (active) {
          setError(errorText(value))
          setLoading(false)
        }
      })
    const timer = window.setInterval(() => {
      void adminApi
        .localTunnels()
        .then((items) => {
          if (active) setLocals(items)
        })
        .catch(() => undefined)
    }, 3000)
    return () => {
      active = false
      window.clearInterval(timer)
    }
  }, [])

  async function localAction(
    id: string,
    action: "enable" | "disable" | "start" | "stop" | "detach"
  ) {
    setBusy(`${action}:${id}`)
    try {
      if (action === "detach") {
        await adminApi.detachLocalTunnel(id)
        setLocals((items) => items.filter((item) => item.id !== id))
      } else {
        const item =
          action === "enable"
            ? await adminApi.enableLocalTunnel(id)
            : action === "disable"
              ? await adminApi.disableLocalTunnel(id)
              : action === "start"
                ? await adminApi.startLocalTunnel(id)
                : await adminApi.stopLocalTunnel(id)
        setLocals((items) =>
          items.map((current) => (current.id === item.id ? item : current))
        )
      }
      setMessage(
        `Tunnel ${id} ${action === "detach" ? "detached" : `${action}d`}.`
      )
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
    } finally {
      setBusy("")
    }
  }

  async function providerAction(provider: string, action: "start" | "stop") {
    setBusy(`${action}:${provider}`)
    try {
      const item =
        action === "start"
          ? await adminApi.startTunnelProvider(provider)
          : await adminApi.stopTunnelProvider(provider)
      setProviders((items) => [
        ...items.filter((current) => current.provider !== item.provider),
        item,
      ])
      setMessage(
        `${item.name || item.provider} ${action} requested.`
      )
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
    } finally {
      setBusy("")
    }
  }

  async function attachLocal(request: LocalTunnelRequest) {
    setBusy("local:attach")
    try {
      const item = await adminApi.attachLocalTunnel(request)
      setLocals((items) => [
        ...items.filter((current) => current.id !== item.id),
        item,
      ])
      setMessage(`Tunnel ${item.id} attached.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
      throw value
    } finally {
      setBusy("")
    }
  }

  async function updateLocal(id: string, request: LocalTunnelRequest) {
    setBusy(`local:update:${id}`)
    try {
      const item = await adminApi.updateLocalTunnel(id, request)
      setLocals((items) =>
        items.map((current) => (current.id === item.id ? item : current))
      )
      setMessage(`Tunnel ${item.id} updated.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
      throw value
    } finally {
      setBusy("")
    }
  }

  async function addAdmin(request: TunnelAdminProfileRequest) {
    setBusy("admin:add")
    try {
      const item = await adminApi.addTunnelAdminProfile(request)
      setAdmins((items) => [...items, item])
      setMessage(`Admin profile ${item.id} added.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      throw value
    } finally {
      setBusy("")
    }
  }

  async function updateAdmin(id: string, request: TunnelAdminProfileRequest) {
    setBusy(`admin:update:${id}`)
    try {
      const item = await adminApi.updateTunnelAdminProfile(id, request)
      setAdmins((items) =>
        items.map((current) => (current.id === id ? item : current))
      )
      setMessage(`Admin profile ${item.id} updated.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      setMessage("")
      throw value
    } finally {
      setBusy("")
    }
  }

  async function verifyAdmin(id: string) {
    setBusy(`admin:verify:${id}`)
    try {
      const item = await adminApi.verifyTunnelAdminProfile(id)
      setAdmins((items) =>
        items.map((current) => (current.id === id ? item : current))
      )
      setMessage(`Admin profile ${id} verified.`)
      setError("")
      await loadManaged(managedProfile)
    } catch (value) {
      setError(errorText(value))
      setMessage("")
    } finally {
      setBusy("")
    }
  }

  async function removeAdmin(id: string) {
    setBusy(`admin:remove:${id}`)
    try {
      await adminApi.removeTunnelAdminProfile(id)
      setAdmins((items) => items.filter((item) => item.id !== id))
      setMessage(`Admin profile ${id} removed.`)
      setError("")
      if (managedProfile === id) {
        setManagedProfile("all")
        await loadManaged("all")
      }
    } catch (value) {
      setError(errorText(value))
      setMessage("")
    } finally {
      setBusy("")
    }
  }

  async function loadManaged(profile = managedProfile) {
    setBusy("managed:refresh")
    try {
      setManaged(
        await adminApi.managedTunnelCollection(profile === "all" ? "" : profile)
      )
      setError("")
    } catch (value) {
      setError(errorText(value))
    } finally {
      setBusy("")
    }
  }

  async function createManaged(
    profile: string,
    request: ManagedTunnelCreateRequest
  ) {
    setBusy("managed:create")
    try {
      await adminApi.createManagedTunnelByProfile(request, profile)
      await loadManaged(managedProfile)
      setMessage("Managed tunnel created.")
      setError("")
    } catch (value) {
      setError(errorText(value))
      throw value
    } finally {
      setBusy("")
    }
  }

  async function updateManaged(
    id: string,
    profile: string,
    request: ManagedTunnelUpdateRequest
  ) {
    setBusy(`managed:update:${id}`)
    try {
      await adminApi.updateManagedTunnelByProfile(id, request, profile)
      await loadManaged(managedProfile)
      setMessage(`Managed tunnel ${id} updated.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      throw value
    } finally {
      setBusy("")
    }
  }

  async function deleteManaged(id: string, profile: string) {
    setBusy(`managed:delete:${id}`)
    try {
      await adminApi.deleteManagedTunnelByProfile(id, profile)
      await loadManaged(managedProfile)
      setMessage(`Managed tunnel ${id} deleted.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      throw value
    } finally {
      setBusy("")
    }
  }

  async function attachManaged(
    id: string,
    profile: string,
    request: {
      runtime_api_key?: string
      auto_generate_runtime_key?: boolean
      project_id?: string
      enabled: boolean
    }
  ) {
    setBusy(`managed:attach:${id}`)
    try {
      const item = await adminApi.attachManagedTunnel(id, request, profile)
      setLocals((items) => [
        ...items.filter((current) => current.id !== item.id),
        item,
      ])
      setMessage(`Managed tunnel ${id} attached.`)
      setError("")
    } catch (value) {
      setError(errorText(value))
      throw value
    } finally {
      setBusy("")
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="Tunnels"
        description="Many independent OpenAI Secure MCP Tunnel ingress connections share one local runtime."
        actions={
          <Button
            aria-label="Refresh tunnels"
            disabled={refreshing}
            size="sm"
            variant="outline"
            onClick={() => void refresh()}
          >
            <RefreshCw className={refreshing ? "animate-spin" : ""} />
            Refresh
          </Button>
        }
      />
      <PageError message={error} />
      {message ? (
        <Alert>
          <Network />
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      ) : null}
      {loading ? (
        <PageLoading rows={5} />
      ) : (
        <Tabs defaultValue="local" className="gap-4">
          <ScrollableTabsList variant="line" className="justify-start border-b">
            <TabsTrigger value="local">
              <Power />
              Local instances
            </TabsTrigger>
            <TabsTrigger value="providers">
              <Network />
              Providers
            </TabsTrigger>
            <TabsTrigger value="admin">
              <ShieldCheck />
              Admin profiles
            </TabsTrigger>
            <TabsTrigger value="managed">
              <Cloud />
              Managed tunnels
            </TabsTrigger>
          </ScrollableTabsList>
          <TabsContent value="local">
            <LocalTunnelsPanel
              items={locals}
              admins={admins}
              busy={busy}
              onAction={localAction}
              onAttach={attachLocal}
              onUpdate={updateLocal}
            />
          </TabsContent>
          <TabsContent value="providers">
            <TunnelProvidersPanel
              items={providers}
              busy={busy}
              onStart={(provider) => providerAction(provider, "start")}
              onStop={(provider) => providerAction(provider, "stop")}
            />
          </TabsContent>
          <TabsContent value="admin">
            <AdminProfilesPanel
              items={admins}
              locals={locals}
              busy={busy}
              onAdd={addAdmin}
              onUpdate={updateAdmin}
              onVerify={verifyAdmin}
              onRemove={removeAdmin}
            />
          </TabsContent>
          <TabsContent value="managed">
            <ManagedTunnelsPanel
              items={managed}
              locals={locals}
              admins={admins}
              profile={managedProfile}
              busy={busy}
              onProfileChange={(value) => {
                setManagedProfile(value)
                void loadManaged(value)
              }}
              onRefresh={() => void loadManaged()}
              onCreate={createManaged}
              onUpdate={updateManaged}
              onDelete={deleteManaged}
              onAttach={attachManaged}
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  )
}

function TunnelProvidersPanel({
  items,
  busy,
  onStart,
  onStop,
}: {
  items: TunnelProvider[]
  busy: string
  onStart: (provider: string) => Promise<void>
  onStop: (provider: string) => Promise<void>
}) {
  if (!items.length) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Tunnel providers</CardTitle>
          <CardDescription>
            Installed runtime plugins that expose MCP or Admin HTTP through a
            generic tunnel capability.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            No tunnel providers are installed. Install one with{" "}
            <code>cgm plugin install &lt;id&gt;</code>.
          </p>
        </CardContent>
      </Card>
    )
  }
  return (
    <div className="grid gap-4">
      {items.map((item) => (
        <Card key={item.provider}>
          <CardHeader>
            <CardTitle>{item.name || item.provider}</CardTitle>
            <CardDescription>
              {item.plugin_id || item.provider}
              {item.enabled ? " · enabled" : " · disabled"}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {(item.targets ?? []).map((target) => (
              <div key={target.target} className="flex justify-between gap-4">
                <span className="font-medium">{target.target}</span>
                <span className="text-muted-foreground">
                  {target.url ||
                    target.last_error ||
                    (target.ready
                      ? "ready"
                      : target.running
                        ? "connecting"
                        : target.desired
                          ? "offline"
                          : "disabled")}
                </span>
              </div>
            ))}
          </CardContent>
          <CardFooter className="gap-2">
            <Button
              size="sm"
              disabled={busy !== ""}
              onClick={() => void onStart(item.provider)}
            >
              Start
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={busy !== ""}
              onClick={() => void onStop(item.provider)}
            >
              Stop
            </Button>
          </CardFooter>
        </Card>
      ))}
    </div>
  )
}

function LocalTunnelsPanel({
  items,
  admins,
  busy,
  onAction,
  onAttach,
  onUpdate,
}: {
  items: LocalTunnel[]
  admins: TunnelAdminProfile[]
  busy: string
  onAction: (
    id: string,
    action: "enable" | "disable" | "start" | "stop" | "detach"
  ) => Promise<void>
  onAttach: (request: LocalTunnelRequest) => Promise<void>
  onUpdate: (id: string, request: LocalTunnelRequest) => Promise<void>
}) {
  const [editing, setEditing] = useState("")
  const [id, setID] = useState("")
  const [key, setKey] = useState("")
  const [enabled, setEnabled] = useState(true)
  const [admin, setAdmin] = useState("none")
  const [org, setOrg] = useState("")
  const [controlPlane, setControlPlane] = useState("")
  const creating = editing === "new"
  function closeForm() {
    setEditing("")
    setID("")
    setKey("")
    setEnabled(true)
    setAdmin("none")
    setOrg("")
    setControlPlane("")
  }
  function openCreate() {
    setEditing("new")
    setID("")
    setKey("")
    setEnabled(true)
    setAdmin("none")
    setOrg("")
    setControlPlane("")
  }
  function openEdit(item: LocalTunnel) {
    setEditing(item.id)
    setID(item.id)
    setKey("")
    setEnabled(item.enabled)
    setAdmin(item.admin_profile_id || "none")
    setOrg(item.organization_id || "")
    setControlPlane(item.control_plane_base_url || "")
  }
  async function save() {
    const request: LocalTunnelRequest = {
      id: id.trim(),
      enabled,
      api_key: key.trim() || undefined,
      admin_profile_id: admin === "none" ? undefined : admin,
      organization_id: org.trim() || undefined,
      control_plane_base_url: controlPlane.trim() || undefined,
    }
    if (creating) await onAttach(request)
    else await onUpdate(editing, request)
    closeForm()
  }
  const labels = uniqueTunnelLabels(
    items.map((item) => ({
      id: item.id,
      name: item.status.metadata?.name ?? "",
    }))
  )
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex items-start justify-between gap-3">
            <div>
              <CardTitle>Local tunnel instances</CardTitle>
              <CardDescription>
                {items.length
                  ? "Each attached runtime key is an independent ingress."
                  : "No tunnels are attached. Add a local runtime key or attach one from Managed tunnels."}
              </CardDescription>
            </div>
            <Button
              disabled={Boolean(busy)}
              size="sm"
              onClick={() => (editing ? closeForm() : openCreate())}
            >
              {editing ? (
                "Cancel"
              ) : (
                <>
                  <Plus />
                  Add local tunnel
                </>
              )}
            </Button>
          </div>
        </CardHeader>
        {editing ? (
          <CardContent className="grid gap-4 md:grid-cols-2">
            <ConfigField
              label="Tunnel ID"
              description="OpenAI tunnel identifier for this local attachment."
            >
              <Input
                aria-label="Tunnel ID"
                disabled={!creating}
                value={id}
                onChange={(event) => setID(event.target.value)}
                placeholder="tun_..."
              />
            </ConfigField>
            <ConfigField
              label="Runtime API key"
              description={
                creating
                  ? "OpenAI key with Tunnels Read + Use permissions."
                  : "Leave blank to keep the stored key."
              }
            >
              <Input
                aria-label="Runtime API key"
                type="password"
                autoComplete="off"
                value={key}
                onChange={(event) => setKey(event.target.value)}
              />
            </ConfigField>
            <ConfigField
              label="Admin profile"
              description="Optional management credential linked to this attachment."
            >
              <Select value={admin} onValueChange={setAdmin}>
                <SelectTrigger aria-label="Admin profile" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">None</SelectItem>
                  {admins.map((item) => (
                    <SelectItem key={item.id} value={item.id}>
                      {item.id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </ConfigField>
            <ConfigField
              label="Organization ID"
              description="Optional organization pinned on this runtime key."
            >
              <Input
                aria-label="Organization ID"
                value={org}
                onChange={(event) => setOrg(event.target.value)}
              />
            </ConfigField>
            <ConfigField
              label="Control plane base URL"
              description="Optional control plane override."
            >
              <Input
                aria-label="Control plane base URL"
                value={controlPlane}
                onChange={(event) => setControlPlane(event.target.value)}
                placeholder="Default"
              />
            </ConfigField>
            <Field orientation="horizontal" className="rounded-lg border p-3">
              <div className="min-w-0 flex-1">
                <FieldLabel>Enabled</FieldLabel>
                <FieldDescription>
                  Disabled tunnels stay attached without connecting.
                </FieldDescription>
              </div>
              <Switch
                aria-label="Enabled"
                checked={enabled}
                onCheckedChange={setEnabled}
              />
            </Field>
          </CardContent>
        ) : null}
        {editing ? (
          <CardFooter className="justify-end border-t">
            <Button
              disabled={
                Boolean(busy) || !id.trim() || (creating && !key.trim())
              }
              onClick={() => void save()}
            >
              {creating ? "Add local tunnel" : "Save local tunnel"}
            </Button>
          </CardFooter>
        ) : null}
      </Card>
      {items.length ? (
        <div className="grid gap-4 xl:grid-cols-2">
          {items.map((item, index) => {
            const active = item.status.running || item.status.restarting
            const state = tunnelState(item)
            const name = item.status.metadata?.name?.trim()
            const label = labels[index]
            return (
              <Card key={item.id}>
                <CardHeader>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <CardTitle
                          className={name ? "text-base" : "font-mono text-base"}
                        >
                          {label}
                        </CardTitle>
                        <Badge
                          variant={
                            item.status.ready
                              ? "default"
                              : active
                                ? "secondary"
                                : "outline"
                          }
                        >
                          {item.status.restarting ||
                          (item.status.running && !item.status.ready) ? (
                            <Spinner className="size-3" />
                          ) : null}
                          {state}
                        </Badge>
                      </div>
                      <CardDescription className="mt-1">
                        {name ? `${item.id} · ` : ""}
                        {item.admin_profile_id
                          ? `Admin profile: ${item.admin_profile_id}`
                          : "Runtime-only attachment"}
                      </CardDescription>
                    </div>
                    <Cloud className="size-5 text-muted-foreground" />
                  </div>
                </CardHeader>
                <CardContent className="grid gap-3 sm:grid-cols-2">
                  <Metric
                    label="Runtime key"
                    value={
                      item.runtime_key_configured ? "Configured" : "Missing"
                    }
                  />
                  <Metric
                    label="Organization"
                    value={item.organization_id || "-"}
                  />
                  <Metric
                    label="Control plane"
                    value={item.control_plane_base_url || "Default"}
                  />
                  <Metric
                    label="Last error"
                    value={item.status.last_error || "-"}
                  />
                </CardContent>
                <CardFooter className="flex flex-wrap justify-end gap-2 border-t">
                  <Button
                    aria-label={`Edit ${item.id}`}
                    disabled={Boolean(busy)}
                    size="sm"
                    variant="outline"
                    onClick={() => openEdit(item)}
                  >
                    Edit
                  </Button>
                  <Button
                    aria-label={`${item.enabled ? "Disable" : "Enable"} ${item.id}`}
                    disabled={Boolean(busy)}
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      void onAction(
                        item.id,
                        item.enabled ? "disable" : "enable"
                      )
                    }
                  >
                    {item.enabled ? "Disable" : "Enable"}
                  </Button>
                  <Button
                    aria-label={`${active ? "Stop" : "Start"} ${item.id}`}
                    disabled={
                      Boolean(busy) ||
                      !item.enabled ||
                      !item.runtime_key_configured
                    }
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      void onAction(item.id, active ? "stop" : "start")
                    }
                  >
                    {active ? "Stop" : "Start"}
                  </Button>
                  <Button
                    aria-label={`Detach ${item.id}`}
                    disabled={Boolean(busy)}
                    size="sm"
                    variant="outline"
                    onClick={() => void onAction(item.id, "detach")}
                  >
                    Detach
                  </Button>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}

function AdminProfilesPanel({
  items,
  locals,
  busy,
  onAdd,
  onUpdate,
  onVerify,
  onRemove,
}: {
  items: TunnelAdminProfile[]
  locals: LocalTunnel[]
  busy: string
  onAdd: (request: TunnelAdminProfileRequest) => Promise<void>
  onUpdate: (id: string, request: TunnelAdminProfileRequest) => Promise<void>
  onVerify: (id: string) => Promise<void>
  onRemove: (id: string) => Promise<void>
}) {
  const [editing, setEditing] = useState("")
  const [id, setID] = useState("")
  const [key, setKey] = useState("")
  const [scope, setScope] = useState<AdminScopeKind>("workspace")
  const [scopeID, setScopeID] = useState("")
  const [controlPlane, setControlPlane] = useState("")
  const creating = editing === "new"
  function closeForm() {
    setEditing("")
    setID("")
    setKey("")
    setScope("workspace")
    setScopeID("")
    setControlPlane("")
  }
  function openCreate() {
    setEditing("new")
    setID("")
    setKey("")
    setScope("workspace")
    setScopeID("")
    setControlPlane("")
  }
  function openEdit(item: TunnelAdminProfile) {
    setEditing(item.id)
    setID(item.id)
    setKey("")
    setScope(
      item.organization_id
        ? "organization"
        : item.tenant_id
          ? "tenant"
          : "workspace"
    )
    setScopeID(
      item.organization_id || item.workspace_id || item.tenant_id || ""
    )
    setControlPlane(item.control_plane_base_url || "")
  }
  async function save() {
    const request: TunnelAdminProfileRequest = {
      id: (creating ? id : editing).trim(),
      admin_key: key.trim() || undefined,
      control_plane_base_url: controlPlane.trim() || undefined,
    }
    if (scope === "organization") request.organization_id = scopeID.trim()
    else if (scope === "workspace") request.workspace_id = scopeID.trim()
    else request.tenant_id = scopeID.trim()
    if (creating) await onAdd(request)
    else await onUpdate(editing, request)
    closeForm()
  }
  const localLabelByID = Object.fromEntries(
    uniqueTunnelLabels(
      locals.map((item) => ({
        id: item.id,
        name: item.status.metadata?.name ?? "",
      }))
    ).map((label, index) => [locals[index].id, label])
  )
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex items-start justify-between gap-3">
            <div>
              <CardTitle>Admin profiles</CardTitle>
              <CardDescription>
                Management credentials are independent from runtime keys and can
                manage different OpenAI scopes.
              </CardDescription>
            </div>
            <Button
              disabled={Boolean(busy)}
              size="sm"
              onClick={() => (editing ? closeForm() : openCreate())}
            >
              {editing ? "Cancel" : "Add profile"}
            </Button>
          </div>
        </CardHeader>
        {editing ? (
          <CardContent className="grid gap-4 md:grid-cols-2">
            {creating ? (
              <ConfigField
                label="Profile ID"
                description="Local stable name used when choosing a management credential."
              >
                <Input
                  aria-label="Profile ID"
                  value={id}
                  onChange={(event) => setID(event.target.value)}
                  placeholder="work"
                />
              </ConfigField>
            ) : null}
            <ConfigField
              label="Admin key"
              description={
                creating
                  ? "Stored in the tunnel secret store."
                  : "Leave blank to keep the stored key."
              }
            >
              <Input
                aria-label="Admin key"
                type="password"
                autoComplete="off"
                value={key}
                onChange={(event) => setKey(event.target.value)}
              />
            </ConfigField>
            <ConfigField
              label="Scope type"
              description="Exactly one management scope."
            >
              <Select
                value={scope}
                onValueChange={(value) => {
                  setScope(value as AdminScopeKind)
                  setScopeID("")
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="organization">Organization</SelectItem>
                  <SelectItem value="workspace">Workspace</SelectItem>
                  <SelectItem value="tenant">Tenant</SelectItem>
                </SelectContent>
              </Select>
            </ConfigField>
            <ConfigField
              label="Scope ID"
              description="OpenAI organization, workspace, or tenant ID."
            >
              <Input
                aria-label="Scope ID"
                value={scopeID}
                onChange={(event) => setScopeID(event.target.value)}
              />
            </ConfigField>
            <ConfigField
              label="Control plane base URL"
              description="Optional profile-specific control plane override."
            >
              <Input
                value={controlPlane}
                onChange={(event) => setControlPlane(event.target.value)}
                placeholder="Default"
              />
            </ConfigField>
          </CardContent>
        ) : null}
        {editing ? (
          <CardFooter className="justify-end border-t">
            <Button
              disabled={
                Boolean(busy) ||
                !scopeID.trim() ||
                (creating && (!id.trim() || !key.trim()))
              }
              onClick={() => void save()}
            >
              {creating ? "Add admin profile" : "Save admin profile"}
            </Button>
          </CardFooter>
        ) : null}
      </Card>
      <div className="grid gap-4 xl:grid-cols-2">
        {items.map((item) => {
          const usedBy = locals
            .filter((local) => local.admin_profile_id === item.id)
            .map((local) => localLabelByID[local.id])
          return (
            <Card key={item.id}>
              <CardHeader>
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="flex flex-wrap items-center gap-2">
                      <CardTitle>{item.id}</CardTitle>
                      <Badge
                        variant={
                          item.manage_access
                            ? "default"
                            : item.read_access
                              ? "secondary"
                              : "outline"
                        }
                      >
                        {item.manage_access
                          ? "Manage"
                          : item.read_access
                            ? "Read"
                            : "Unverified"}
                      </Badge>
                    </div>
                    <CardDescription className="mt-1">
                      {adminScope(item)}
                    </CardDescription>
                  </div>
                  <KeyRound className="size-5 text-muted-foreground" />
                </div>
              </CardHeader>
              <CardContent className="space-y-2">
                <Metric
                  label="Key"
                  value={item.key_configured ? "Configured" : "Missing"}
                />
                <Metric
                  label="Local tunnels"
                  value={usedBy.length ? usedBy.join(", ") : "None"}
                />
              </CardContent>
              <CardFooter className="justify-end gap-2 border-t">
                <Button
                  aria-label={`Edit ${item.id}`}
                  disabled={Boolean(busy)}
                  size="sm"
                  variant="outline"
                  onClick={() => openEdit(item)}
                >
                  Edit
                </Button>
                <Button
                  aria-label={`Verify ${item.id}`}
                  disabled={Boolean(busy) || !item.key_configured}
                  size="sm"
                  variant="outline"
                  onClick={() => void onVerify(item.id)}
                >
                  Verify
                </Button>
                <Button
                  aria-label={`Remove ${item.id}`}
                  disabled={Boolean(busy) || usedBy.length > 0}
                  size="sm"
                  variant="outline"
                  onClick={() => void onRemove(item.id)}
                >
                  Remove
                </Button>
              </CardFooter>
            </Card>
          )
        })}
      </div>
    </div>
  )
}

function ManagedTunnelsPanel({
  items,
  locals,
  admins,
  profile,
  busy,
  onProfileChange,
  onRefresh,
  onCreate,
  onUpdate,
  onDelete,
  onAttach,
}: {
  items: ManagedTunnel[]
  locals: LocalTunnel[]
  admins: TunnelAdminProfile[]
  profile: string
  busy: string
  onProfileChange: (value: string) => void
  onRefresh: () => void
  onCreate: (
    profile: string,
    request: ManagedTunnelCreateRequest
  ) => Promise<void>
  onUpdate: (
    id: string,
    profile: string,
    request: ManagedTunnelUpdateRequest
  ) => Promise<void>
  onDelete: (id: string, profile: string) => Promise<void>
  onAttach: (
    id: string,
    profile: string,
    request: {
      runtime_api_key?: string
      auto_generate_runtime_key?: boolean
      project_id?: string
      enabled: boolean
    }
  ) => Promise<void>
}) {
  const [creating, setCreating] = useState(false)
  const [createProfile, setCreateProfile] = useState("")
  const [createName, setCreateName] = useState("")
  const [createDescription, setCreateDescription] = useState("")
  const [createOrganizations, setCreateOrganizations] = useState("")
  const [createWorkspaces, setCreateWorkspaces] = useState("")
  const [editID, setEditID] = useState("")
  const [editProfile, setEditProfile] = useState("")
  const [editName, setEditName] = useState("")
  const [editDescription, setEditDescription] = useState("")
  const [deleteItem, setDeleteItem] = useState<ManagedTunnel | null>(null)
  const [deleteProfile, setDeleteProfile] = useState("")
  const [attachItem, setAttachItem] = useState<ManagedTunnel | null>(null)
  const [attachProfile, setAttachProfile] = useState("")
  const [runtimeMode, setRuntimeMode] = useState<RuntimeMode>("auto")
  const [runtimeKey, setRuntimeKey] = useState("")
  const [projectID, setProjectID] = useState("")
  const [enabled, setEnabled] = useState(true)
  const attached = useMemo(
    () => new Set(locals.map((item) => item.id)),
    [locals]
  )
  const manageProfiles = admins.filter((item) => item.manage_access)
  function preferredProfile(item: ManagedTunnel, manage: boolean) {
    const allowed = item.admin_profiles.filter((id) =>
      admins.some(
        (admin) =>
          admin.id === id &&
          (manage
            ? admin.manage_access
            : admin.read_access || admin.manage_access)
      )
    )
    if (profile !== "all" && allowed.includes(profile)) return profile
    return allowed.length === 1 ? allowed[0] : ""
  }
  function beginEdit(item: ManagedTunnel) {
    setEditID(item.metadata.id)
    setEditProfile(preferredProfile(item, true))
    setEditName(item.metadata.name)
    setEditDescription(item.metadata.description)
  }
  async function saveEdit() {
    if (!editID || !editProfile) return
    await onUpdate(editID, editProfile, {
      name: editName.trim(),
      description: editDescription.trim(),
    })
    setEditID("")
  }
  async function create() {
    if (!createProfile) return
    await onCreate(createProfile, {
      name: createName.trim(),
      description: createDescription.trim(),
      organization_ids: splitIDs(createOrganizations),
      workspace_ids: splitIDs(createWorkspaces),
    })
    setCreating(false)
    setCreateName("")
    setCreateDescription("")
    setCreateOrganizations("")
    setCreateWorkspaces("")
  }
  function openAttach(item: ManagedTunnel) {
    setAttachItem(item)
    setAttachProfile(preferredProfile(item, false))
    setRuntimeMode("auto")
    setRuntimeKey("")
    setProjectID("")
    setEnabled(true)
  }
  async function attach() {
    if (
      !attachItem ||
      !attachProfile ||
      (runtimeMode === "manual" && !runtimeKey.trim())
    )
      return
    await onAttach(
      attachItem.metadata.id,
      attachProfile,
      runtimeMode === "auto"
        ? {
            auto_generate_runtime_key: true,
            project_id: projectID.trim() || undefined,
            enabled,
          }
        : { runtime_api_key: runtimeKey.trim(), enabled }
    )
    setAttachItem(null)
  }
  async function remove() {
    if (!deleteItem || !deleteProfile) return
    await onDelete(deleteItem.metadata.id, deleteProfile)
    setDeleteItem(null)
  }
  const labels = uniqueTunnelLabels(
    items.map((item) => ({
      id: item.metadata.id,
      name: item.metadata.name ?? "",
    }))
  )
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle>Managed tunnels</CardTitle>
              <CardDescription>
                Remote tunnels stay separate from local attachments. Aggregate
                discovery keeps admin-profile provenance.
              </CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              <Select value={profile} onValueChange={onProfileChange}>
                <SelectTrigger className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All profiles</SelectItem>
                  {admins
                    .filter((item) => item.read_access || item.manage_access)
                    .map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.id}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
              <Button
                disabled={Boolean(busy)}
                size="sm"
                variant="outline"
                onClick={onRefresh}
              >
                <RefreshCw
                  className={busy === "managed:refresh" ? "animate-spin" : ""}
                />
                Refresh
              </Button>
              {manageProfiles.length ? (
                <Button
                  disabled={Boolean(busy)}
                  size="sm"
                  onClick={() => {
                    setCreating((value) => !value)
                    setCreateProfile(
                      profile !== "all" &&
                        manageProfiles.some((item) => item.id === profile)
                        ? profile
                        : manageProfiles.length === 1
                          ? manageProfiles[0].id
                          : ""
                    )
                  }}
                >
                  {creating ? "Cancel create" : "Create tunnel"}
                </Button>
              ) : null}
            </div>
          </div>
        </CardHeader>
        {creating ? (
          <CardContent className="grid gap-4 md:grid-cols-2">
            <ProfileField
              label="Admin profile"
              value={createProfile}
              profiles={manageProfiles.map((item) => item.id)}
              onChange={setCreateProfile}
            />
            <ConfigField label="Name" description="Remote tunnel name.">
              <Input
                value={createName}
                onChange={(event) => setCreateName(event.target.value)}
              />
            </ConfigField>
            <ConfigField
              label="Description"
              description="Remote tunnel description."
            >
              <Input
                value={createDescription}
                onChange={(event) => setCreateDescription(event.target.value)}
              />
            </ConfigField>
            <ConfigField
              label="Organization IDs"
              description="Comma separated."
            >
              <Input
                value={createOrganizations}
                onChange={(event) => setCreateOrganizations(event.target.value)}
              />
            </ConfigField>
            <ConfigField label="Workspace IDs" description="Comma separated.">
              <Input
                value={createWorkspaces}
                onChange={(event) => setCreateWorkspaces(event.target.value)}
              />
            </ConfigField>
          </CardContent>
        ) : null}
        {creating ? (
          <CardFooter className="justify-end border-t">
            <Button
              disabled={
                Boolean(busy) ||
                !createProfile ||
                !createName.trim() ||
                !createDescription.trim() ||
                (!createOrganizations.trim() && !createWorkspaces.trim())
              }
              onClick={() => void create()}
            >
              Create
            </Button>
          </CardFooter>
        ) : null}
      </Card>
      {!items.length ? (
        <Card>
          <CardHeader>
            <CardDescription>
              No managed tunnels are visible through the selected admin profile.
            </CardDescription>
          </CardHeader>
        </Card>
      ) : (
        <div className="grid gap-4 xl:grid-cols-2">
          {items.map((item, index) => {
            const metadata = item.metadata
            const isAttached = attached.has(metadata.id)
            const editing = editID === metadata.id
            const canManage = item.admin_profiles.some((id) =>
              admins.some((admin) => admin.id === id && admin.manage_access)
            )
            return (
              <Card key={metadata.id}>
                <CardHeader>
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <CardTitle>{labels[index]}</CardTitle>
                        {isAttached ? (
                          <Badge variant="secondary">Attached</Badge>
                        ) : null}
                      </div>
                      <CardDescription className="mt-1 font-mono">
                        {metadata.id}
                      </CardDescription>
                    </div>
                    <Cloud className="size-5 text-muted-foreground" />
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  {metadata.description ? (
                    <div className="text-sm text-muted-foreground">
                      {metadata.description}
                    </div>
                  ) : null}
                  <div className="flex flex-wrap gap-2">
                    {item.admin_profiles.map((id) => (
                      <Badge key={id} variant="outline">
                        {id}
                      </Badge>
                    ))}
                  </div>
                  {editing ? (
                    <div className="space-y-3 rounded-lg border p-3">
                      <ProfileField
                        label="Admin profile"
                        value={editProfile}
                        profiles={item.admin_profiles.filter((id) =>
                          admins.some(
                            (admin) => admin.id === id && admin.manage_access
                          )
                        )}
                        onChange={setEditProfile}
                      />
                      <Input
                        value={editName}
                        onChange={(event) => setEditName(event.target.value)}
                      />
                      <Input
                        value={editDescription}
                        onChange={(event) =>
                          setEditDescription(event.target.value)
                        }
                      />
                      <div className="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setEditID("")}
                        >
                          Cancel
                        </Button>
                        <Button
                          disabled={
                            Boolean(busy) ||
                            !editProfile ||
                            !editName.trim() ||
                            !editDescription.trim()
                          }
                          size="sm"
                          onClick={() => void saveEdit()}
                        >
                          Save
                        </Button>
                      </div>
                    </div>
                  ) : null}
                </CardContent>
                <CardFooter className="flex-wrap justify-end gap-2 border-t">
                  {canManage ? (
                    <>
                      <Button
                        disabled={Boolean(busy)}
                        size="sm"
                        variant="outline"
                        onClick={() => beginEdit(item)}
                      >
                        Edit
                      </Button>
                      <Button
                        disabled={Boolean(busy) || isAttached}
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setDeleteItem(item)
                          setDeleteProfile(preferredProfile(item, true))
                        }}
                      >
                        Delete
                      </Button>
                    </>
                  ) : null}
                  <Button
                    disabled={Boolean(busy) || isAttached}
                    size="sm"
                    onClick={() => openAttach(item)}
                  >
                    {isAttached ? "Attached" : "Attach"}
                  </Button>
                </CardFooter>
              </Card>
            )
          })}
        </div>
      )}
      <AlertDialog
        open={Boolean(attachItem)}
        onOpenChange={(open) => {
          if (!open && !busy) setAttachItem(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Attach managed tunnel?</AlertDialogTitle>
            <AlertDialogDescription>
              This adds another ingress connection to the same shared local
              runtime. It does not replace existing tunnels.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="space-y-4">
            {attachItem ? (
              <ProfileField
                label="Admin profile"
                value={attachProfile}
                profiles={attachItem.admin_profiles.filter((id) =>
                  admins.some(
                    (admin) =>
                      admin.id === id &&
                      (admin.read_access || admin.manage_access)
                  )
                )}
                onChange={setAttachProfile}
              />
            ) : null}
            <ConfigField
              label="Runtime credential"
              description="Use a dedicated Read + Use key or generate one through a Manage profile."
            >
              <Select
                value={runtimeMode}
                onValueChange={(value) => setRuntimeMode(value as RuntimeMode)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">
                    Auto generate runtime key
                  </SelectItem>
                  <SelectItem value="manual">
                    Enter runtime key manually
                  </SelectItem>
                </SelectContent>
              </Select>
            </ConfigField>
            {runtimeMode === "auto" ? (
              <ConfigField
                label="OpenAI project ID"
                description="Optional project for automatic service-account key generation."
              >
                <Input
                  value={projectID}
                  onChange={(event) => setProjectID(event.target.value)}
                  placeholder="proj_..."
                />
              </ConfigField>
            ) : (
              <ConfigField
                label="Runtime API key"
                description="OpenAI key with Tunnels Read + Use permissions."
              >
                <Input
                  type="password"
                  autoComplete="off"
                  value={runtimeKey}
                  onChange={(event) => setRuntimeKey(event.target.value)}
                />
              </ConfigField>
            )}
            <Field orientation="horizontal" className="rounded-lg border p-3">
              <div className="min-w-0 flex-1">
                <FieldLabel>Enable after attach</FieldLabel>
                <FieldDescription>
                  Start this ingress when runtime config reloads.
                </FieldDescription>
              </div>
              <Switch checked={enabled} onCheckedChange={setEnabled} />
            </Field>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={Boolean(busy)}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              disabled={
                Boolean(busy) ||
                !attachProfile ||
                (runtimeMode === "manual" && !runtimeKey.trim())
              }
              onClick={() => void attach()}
            >
              Attach tunnel
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog
        open={Boolean(deleteItem)}
        onOpenChange={(open) => {
          if (!open && !busy) setDeleteItem(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete managed tunnel?</AlertDialogTitle>
            <AlertDialogDescription>
              The remote tunnel will be permanently deleted. Attached local
              tunnels must be detached first.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {deleteItem ? (
            <ProfileField
              label="Admin profile"
              value={deleteProfile}
              profiles={deleteItem.admin_profiles.filter((id) =>
                admins.some((admin) => admin.id === id && admin.manage_access)
              )}
              onChange={setDeleteProfile}
            />
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={Boolean(busy)}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={Boolean(busy) || !deleteProfile}
              onClick={() => void remove()}
            >
              Delete tunnel
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ProfileField({
  label,
  value,
  profiles,
  onChange,
}: {
  label: string
  value: string
  profiles: string[]
  onChange: (value: string) => void
}) {
  return (
    <ConfigField
      label={label}
      description="Explicitly selects the management credential for this operation."
    >
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger className="w-full">
          <SelectValue placeholder="Select profile" />
        </SelectTrigger>
        <SelectContent>
          {profiles.map((id) => (
            <SelectItem key={id} value={id}>
              {id}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </ConfigField>
  )
}
function ConfigField({
  label,
  description,
  children,
}: {
  label: string
  description: string
  children: React.ReactNode
}) {
  return (
    <Field>
      <FieldLabel>{label}</FieldLabel>
      {children}
      <FieldDescription>{description}</FieldDescription>
    </Field>
  )
}
function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-sm font-medium break-words">{value}</div>
    </div>
  )
}
function adminScope(item: TunnelAdminProfile) {
  if (item.organization_id) return `organization:${item.organization_id}`
  if (item.workspace_id) return `workspace:${item.workspace_id}`
  if (item.tenant_id) return `tenant:${item.tenant_id}`
  return "No scope"
}
function uniqueTunnelLabels(items: { id: string; name: string }[]) {
  const counts = new Map<string, number>()
  for (const item of items) {
    const name = item.name.trim()
    if (name) counts.set(name, (counts.get(name) || 0) + 1)
  }
  return items.map((item) => {
    const name = item.name.trim()
    if (name && (counts.get(name) || 0) > 1) {
      const short = item.id.trim().replace(/^tunnel_/, "")
      return `${name} · ${short.length <= 8 ? short : short.slice(-8)}`
    }
    return name || item.id
  })
}
function tunnelState(item: LocalTunnel) {
  if (!item.enabled) return "Disabled"
  if (!item.runtime_key_configured) return "Not configured"
  if (item.status.ready) return "Ready"
  if (item.status.restarting) return "Reconnecting"
  if (item.status.running) return "Connecting"
  if (item.status.last_error) return "Degraded"
  return "Offline"
}
function splitIDs(value: string) {
  return [
    ...new Set(
      value
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean)
    ),
  ]
}
function errorText(value: unknown) {
  return value instanceof Error ? value.message : String(value)
}
