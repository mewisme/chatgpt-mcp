export type Tool = { name: string; title?: string; description?: string; inputSchema?: unknown; outputSchema?: unknown; annotations?: Record<string, unknown> }
export type Workspace = { id: string; path: string }
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
export type PublicConfig = {
  server: { host: string; port: number }
  admin: { enabled: boolean; port: number }
  auth: { mcp_enabled: boolean; admin_enabled: boolean; mcp_token_configured: boolean; admin_token_configured: boolean }
}
export type ConfigPreset = { name: string; description: string; server: PublicConfig["server"]; admin: PublicConfig["admin"]; mcp_auth_enabled: boolean; admin_auth_enabled: boolean; tunnel_enabled: boolean }
export type ConfigPresetList = { current: string; presets: ConfigPreset[] }
export type TunnelConfig = { enabled: boolean; id?: string; api_key?: string; command?: string; args?: string[]; origin?: string; public_url?: string }
export type TunnelStatus = { enabled: boolean; running: boolean; pid?: number; command?: string; origin?: string; public_url?: string; started_at?: string; last_error?: string }

const adminTokenKey = "chatgpt-mcp-admin-token"
export const adminToken = { get: () => localStorage.getItem(adminTokenKey) ?? "", set: (token: string) => localStorage.setItem(adminTokenKey, token), clear: () => localStorage.removeItem(adminTokenKey) }

export function authHeaders(): HeadersInit { const token = adminToken.get(); return token ? { Authorization: `Bearer ${token}` } : {} }

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body !== undefined && init.body !== null) headers.set("Content-Type", "application/json")
  const token = adminToken.get()
  if (token) headers.set("Authorization", `Bearer ${token}`)
  const response = await fetch(path, { ...init, headers })
  const text = response.status === 204 ? "" : await response.text()
  if (!response.ok) throw new Error(text.trim() || `API ${response.status}`)
  if (!text) return undefined as T
  try { return JSON.parse(text) as T } catch { throw new Error(`Invalid JSON response from ${path}`) }
}

export const adminApi = {
  health: () => api<{ ok: boolean }>("/api/health"),
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
  tunnel: () => api<TunnelStatus>("/api/tunnel"),
  tunnelConfig: () => api<TunnelConfig>("/api/tunnel/config"),
  configureTunnel: (config: TunnelConfig) => api<TunnelStatus>("/api/tunnel", { method: "PUT", body: JSON.stringify(config) }),
  startTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "POST" }),
  stopTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "DELETE" }),
}
