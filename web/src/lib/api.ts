const base = ""

async function request<T>(path: string): Promise<T> {
  const response = await fetch(`${base}${path}`)
  if (!response.ok) throw new Error(response.statusText)
  return response.json()
}

export const api = {
  health: () => request<{ ok: boolean }>("/api/health"),
  config: () => request<Record<string, unknown>>("/api/config"),
  workspaces: () => request<unknown[]>("/api/workspaces"),
  tools: () => request<unknown[]>("/api/tools"),
}
