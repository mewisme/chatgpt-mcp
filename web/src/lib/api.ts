export type Tool = { name: string; title?: string; description?: string; inputSchema?: unknown; outputSchema?: unknown; annotations?: Record<string, unknown> }
export type Workspace = { id: string; path: string; allow_dirs?: string[] }
export type MCPAuth = { type?: "auto" | "oauth" | "none" | string; scope?: string }
export type MCPServer = {
  id: string
  name: string
  transport: "http" | "stdio" | string
  enabled: boolean
  command?: string
  args?: string[]
  env?: Record<string, string>
  cwd?: string
  url?: string
  headers?: Record<string, string>
  bearer_token_env_var?: string
  auth?: MCPAuth
  tool_prefix?: string
  expose?: "all" | "allowlist" | "meta_only" | "none" | string
  tools?: string[]
  disabled_tools?: string[]
  idle_timeout_sec?: number
}
export type MCPServerStatus = {
  id: string
  name: string
  enabled: boolean
  transport: string
  auth: string
  health: "unknown" | "connected" | "unreachable" | "disabled" | string
  connected: boolean
  tool_count: number
  expose: string
  proxied_tools: string[]
  last_error?: string
  pid?: number
}
export type MCPServerTools = { server_id: string; tools: Tool[]; proxied_tools: string[] }
export type MCPServerOAuthStatus = {
  server_id: string
  configured: boolean
  issuer?: string
  resource?: string
  registration?: string
  client_id?: string
  scopes?: string[]
  has_refresh_token: boolean
  expires_at?: string
  expired: boolean
}
export type MCPServerOAuthLogin = {
  redirect_origin: string
  issuer?: string
  client_id?: string
  client_secret_env_var?: string
  client_metadata_url?: string
  scope?: string
}
export type MCPServerOAuthSession = { session_id: string; authorization_url: string; expires_at: string }
export type PublicConfig = {
  server: { port: number; expose: { mode: "none" | "all" | "0.0.0.0" | "interfaces"; interfaces: string[] }; allow_insecure_http: boolean }
  admin: { enabled: boolean; port: number }
  auth: { mcp_enabled: boolean; admin_enabled: boolean; mcp_token_configured: boolean; admin_token_configured: boolean }
  permissions: { allow_dirs: string[] }
  shell: { path: string[] }
  features: { ponytail: { enabled: boolean }; caveman: { enabled: boolean } }
}
export type NetworkAddress = { address: string; interface?: string; scope: "local" | "lan" | "network" | string }
export type NetworkInterface = { name: string; addresses: NetworkAddress[] }
export type ConfigPreset = { name: string; description: string; server: PublicConfig["server"]; admin: PublicConfig["admin"]; mcp_auth_enabled: boolean; admin_auth_enabled: boolean; tunnel_enabled: boolean; features: PublicConfig["features"] }
export type ConfigPresetList = { current: string; presets: ConfigPreset[] }
export type TunnelAdminScope = { organization_id?: string; workspace_id?: string; tenant_id?: string }
export type TunnelConfig = { enabled: boolean; id?: string; api_key?: string; runtime_key_configured?: boolean; admin_key_configured?: boolean; admin_organization_id?: string; admin_workspace_id?: string; admin_tenant_id?: string; control_plane_base_url?: string; organization_id?: string }
export type TunnelAdminKeyRequest = { admin_key?: string; organization_id?: string; workspace_id?: string; tenant_id?: string }
export type TunnelAdminKeyStatus = { configured: boolean; scope: TunnelAdminScope; tunnels?: number }
export type TunnelMetadata = { id: string; name: string; description: string; creator?: string; tenant_ids?: string[]; workspace_ids?: string[]; organization_ids?: string[]; request_id?: string; fetched_at: string }
export type TunnelStatus = { provider: "openai" | string; enabled: boolean; running: boolean; ready: boolean; restarting: boolean; id?: string; control_plane_base_url?: string; organization_id?: string; started_at?: string; last_error?: string; metadata?: TunnelMetadata; metadata_error?: string; admin_key_configured?: boolean; admin_scope?: TunnelAdminScope }
export type ClusterMember = { instance_id: string; name: string; catalog_hash?: string; workspaces?: string[]; online: boolean; connected_at?: string; last_seen?: string }
export type ClusterWorkspaceOwner = { workspace_id: string; instance_id: string; online: boolean }
export type ClusterStatus = { enabled: boolean; connected: boolean; relay_url?: string; instance_id?: string; name?: string; member_count: number; online_member_count: number; workspace_count: number; catalog_hash?: string; catalog_compatible: boolean; catalog_error?: string; tunnel_role?: string; leader_instance_id?: string; leader_epoch?: number; lease_expires_at?: string; last_error?: string; members?: ClusterMember[]; workspaces?: ClusterWorkspaceOwner[] }

const adminTokenKey = "chatgpt-mcp-admin-token"
try { localStorage.removeItem(adminTokenKey) } catch { /* storage may be unavailable */ }
export const adminToken = { get: () => sessionStorage.getItem(adminTokenKey) ?? "", set: (token: string) => sessionStorage.setItem(adminTokenKey, token), clear: () => sessionStorage.removeItem(adminTokenKey) }
export class ApiError extends Error { status: number; constructor(message: string, status: number) { super(message); this.name = "ApiError"; this.status = status } }

export function authHeaders(): HeadersInit { const token = adminToken.get(); return token ? { Authorization: `Bearer ${token}` } : {} }

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body !== undefined && init.body !== null) headers.set("Content-Type", "application/json")
  const token = adminToken.get()
  if (token) headers.set("Authorization", `Bearer ${token}`)
  const response = await fetch(path, { ...init, headers })
  const text = response.status === 204 ? "" : await response.text()
  if (!response.ok) throw new ApiError(text.trim() || `API ${response.status}`, response.status)
  if (!text) return undefined as T
  try { return JSON.parse(text) as T } catch { throw new Error(`Invalid JSON response from ${path}`) }
}

export const adminApi = {
  health: () => api<{ ok: boolean; auth_enabled: boolean }>("/api/health"),
  networkInterfaces: () => api<NetworkInterface[]>("/api/network/interfaces"),
  cluster: () => api<ClusterStatus>("/api/cluster"),
  config: () => api<PublicConfig>("/api/config"),
  saveConfig: (config: Partial<PublicConfig>) => api<PublicConfig>("/api/config", { method: "PUT", body: JSON.stringify(config) }),
  configPresets: () => api<ConfigPresetList>("/api/config/presets"),
  configPreset: (name: string) => api<ConfigPreset>(`/api/config/presets/${encodeURIComponent(name)}`),
  applyConfigPreset: (name: string) => api<PublicConfig>(`/api/config/presets/${encodeURIComponent(name)}`, { method: "POST" }),
  workspaces: () => api<Workspace[]>("/api/workspaces"),
  workspace: (id: string) => api<Workspace>(`/api/workspaces/${encodeURIComponent(id)}`),
  registerWorkspace: (path: string) => api<Workspace>("/api/workspaces", { method: "POST", body: JSON.stringify({ path }) }),
  removeWorkspace: (id: string) => api<void>(`/api/workspaces/${encodeURIComponent(id)}`, { method: "DELETE" }),
  tools: () => api<Tool[]>("/api/tools"),
  upstream: () => api<MCPServer[]>("/api/upstream"),
  upstreamServer: (id: string) => api<MCPServer>(`/api/upstream/${encodeURIComponent(id)}`),
  addUpstream: (server: MCPServer) => api<MCPServer>("/api/upstream", { method: "POST", body: JSON.stringify(server) }),
  updateUpstream: (id: string, server: MCPServer) => api<MCPServer>(`/api/upstream/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(server) }),
  removeUpstream: (id: string) => api<void>(`/api/upstream/${encodeURIComponent(id)}`, { method: "DELETE" }),
  upstreamStatus: (id: string, refresh = true) => api<MCPServerStatus>(`/api/upstream/${encodeURIComponent(id)}/status?refresh=${refresh}`),
  upstreamTools: (id: string, refresh = false) => api<MCPServerTools>(`/api/upstream/${encodeURIComponent(id)}/tools?refresh=${refresh}`),
  upstreamOAuthStatus: (id: string) => api<MCPServerOAuthStatus>(`/api/upstream/${encodeURIComponent(id)}/auth/status`),
  beginUpstreamOAuth: (id: string, request: MCPServerOAuthLogin) => api<MCPServerOAuthSession>(`/api/upstream/${encodeURIComponent(id)}/auth/login`, { method: "POST", body: JSON.stringify(request) }),
  logoutUpstreamOAuth: (id: string) => api<void>(`/api/upstream/${encodeURIComponent(id)}/auth/logout`, { method: "DELETE" }),
  tunnel: () => api<TunnelStatus>("/api/tunnel"),
  tunnelConfig: () => api<TunnelConfig>("/api/tunnel/config"),
  configureTunnel: (config: TunnelConfig) => api<TunnelStatus>("/api/tunnel", { method: "PUT", body: JSON.stringify(config) }),
  tunnelAdminKey: () => api<TunnelAdminKeyStatus>("/api/tunnel/admin/key"),
  configureTunnelAdminKey: (request: TunnelAdminKeyRequest) => api<TunnelAdminKeyStatus>("/api/tunnel/admin/key", { method: "PUT", body: JSON.stringify(request) }),
  verifyTunnelAdminKey: () => api<TunnelAdminKeyStatus>("/api/tunnel/admin/key", { method: "POST" }),
  removeTunnelAdminKey: () => api<TunnelAdminKeyStatus>("/api/tunnel/admin/key", { method: "DELETE" }),
  startTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "POST" }),
  stopTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "DELETE" }),
}
