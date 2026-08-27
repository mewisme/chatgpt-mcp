export type Tool = { name: string; title?: string; description?: string; inputSchema?: unknown; outputSchema?: unknown; annotations?: Record<string, unknown> }
export type MCPServer = { id: string; name: string; transport: string; enabled: boolean }
export type PublicConfig = { server: { host: string; port: number }; admin: { enabled: boolean; port: number }; auth: { mcp_enabled: boolean; admin_enabled: boolean } }
export type TunnelConfig = { enabled: boolean; id?: string; api_key?: string; command?: string; args?: string[]; origin?: string; public_url?: string }
export type TunnelStatus = { enabled: boolean; running: boolean; pid?: number; command?: string; origin?: string; public_url?: string; started_at?: string; last_error?: string }

const adminTokenKey = "chatgpt-mcp-admin-token"
export const adminToken = { get: () => localStorage.getItem(adminTokenKey) ?? "", set: (token: string) => localStorage.setItem(adminTokenKey, token), clear: () => localStorage.removeItem(adminTokenKey) }

export function authHeaders(): HeadersInit { const token = adminToken.get(); return token ? { Authorization: `Bearer ${token}` } : {} }

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set("Content-Type", "application/json")
  const token = adminToken.get()
  if (token) headers.set("Authorization", `Bearer ${token}`)
  const response = await fetch(path, { ...init, headers })
  if (!response.ok) throw new Error(`API ${response.status}`)
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const adminApi = {
  health: () => api<{ ok: boolean }>("/api/health"),
  config: () => api<PublicConfig>("/api/config"),
  saveConfig: (config: Partial<PublicConfig>) => api<PublicConfig>("/api/config", { method: "PUT", body: JSON.stringify(config) }),
  workspaces: () => api<unknown[]>("/api/workspaces"),
  tools: () => api<Tool[]>("/api/tools"),
  upstream: () => api<MCPServer[]>("/api/upstream"),
  addUpstream: (server: MCPServer) => api<MCPServer>("/api/upstream", { method: "POST", body: JSON.stringify(server) }),
  removeUpstream: (id: string) => api<void>(`/api/upstream/${encodeURIComponent(id)}`, { method: "DELETE" }),
  tunnel: () => api<TunnelStatus>("/api/tunnel"),
  tunnelConfig: () => api<TunnelConfig>("/api/tunnel/config"),
  configureTunnel: (config: TunnelConfig) => api<TunnelStatus>("/api/tunnel", { method: "PUT", body: JSON.stringify(config) }),
  startTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "POST" }),
  stopTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "DELETE" }),
}
