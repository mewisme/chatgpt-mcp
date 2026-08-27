export async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, options)
  if (!response.ok) throw new Error(`API ${response.status}`)
  return response.json() as Promise<T>
}

export const adminApi = {
  health: () => api<{ ok: boolean }>("/api/health"),
  config: () => api<Record<string, unknown>>("/api/config"),
  saveConfig: (config: Record<string, unknown>) => api<{ ok: boolean }>("/api/config", { method: "PUT", body: JSON.stringify(config), headers: { "Content-Type": "application/json" } }),
  workspaces: () => api<unknown[]>("/api/workspaces"),
  tools: () => api<string[]>("/api/tools"),
  upstream: () => api<{ name: string; status: string }[]>("/api/upstream"),
}
