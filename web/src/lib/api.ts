export type TunnelConfig = { enabled: boolean; id?: string; api_key?: string; command?: string; args?: string[]; origin?: string; public_url?: string }
export type TunnelStatus = { enabled: boolean; running: boolean; pid?: number; command?: string; origin?: string; public_url?: string; started_at?: string; last_error?: string }

export async function api<T>(path: string, init?: RequestInit): Promise<T> { const response = await fetch(path, { headers: { "Content-Type": "application/json" }, ...init }); if (!response.ok) throw new Error(`API ${response.status}`); return response.json() as Promise<T> }

export const adminApi = {
  health: () => api<{ ok: boolean }>("/api/health"),
  config: () => api<Record<string, unknown>>("/api/config"),
  saveConfig: (config: unknown) => api<Record<string, unknown>>("/api/config", { method: "PUT", body: JSON.stringify(config) }),
  workspaces: () => api<unknown[]>("/api/workspaces"),
  tools: () => api<string[]>("/api/tools"),
  upstream: () => api<{ id: string; name: string; transport: string; enabled: boolean }[]>("/api/upstream"),
  addUpstream: (server: unknown) => api("/api/upstream", { method: "POST", body: JSON.stringify(server) }),
  removeUpstream: (id: string) => api(`/api/upstream/${id}`, { method: "DELETE" }),
  tunnel: () => api<TunnelStatus>("/api/tunnel"),
  tunnelConfig: () => api<TunnelConfig>("/api/tunnel/config"),
  configureTunnel: (config: TunnelConfig) => api<TunnelStatus>("/api/tunnel", { method: "PUT", body: JSON.stringify(config) }),
  startTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "POST" }),
  stopTunnel: () => api<TunnelStatus>("/api/tunnel", { method: "DELETE" }),
}
