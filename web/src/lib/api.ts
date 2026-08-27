export async function api<T>(path: string): Promise<T> {
  const response = await fetch(path)
  if (!response.ok) throw new Error(`API ${response.status}`)
  return response.json() as Promise<T>
}

export const adminApi = {
  health: () => api<{ ok: boolean }>("/api/health"),
  config: () => api<Record<string, unknown>>("/api/config"),
  workspaces: () => api<unknown[]>("/api/workspaces"),
  tools: () => api<string[]>("/api/tools"),
}
