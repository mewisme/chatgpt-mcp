export type Tool = {
  name: string
  title?: string
  description?: string
  inputSchema?: unknown
  outputSchema?: unknown
  annotations?: Record<string, unknown>
}
export type Workspace = { id: string; path: string; allow_dirs?: string[]; error?: string }
export type WorkspaceContainer = {
  id: string
  name: string
  workspace_ids?: string[]
}
export type ActivityEvent = {
  sequence?: number
  call_id?: string
  kind: string
  phase?: string
  method?: string
  source?: string
  tunnel_id?: string
  tunnel_name?: string
  tool?: string
  workspace_id?: string
  session_hash?: string
  session_access?: string
  session_workspace_count?: number
  received_by_instance_id?: string
  executed_by_instance_id?: string
  status?: string
  duration_ms?: number
  message?: string
  raw?: Record<string, unknown>
  timestamp: string
}
export type ExecutionStatus =
  "running" | "success" | "failed" | "cancelled" | "timed_out" | string
export type ExecutionInfo = {
  id: string
  workspace_id: string
  tool: string
  command: string
  cwd: string
  source?: string
  tunnel_id?: string
  tunnel_name?: string
  started_at: string
  finished_at?: string
  status: ExecutionStatus
  exit_code?: number
  timed_out?: boolean
}
export type ExecutionSnapshot = {
  execution: ExecutionInfo
  stdout: string
  stderr: string
  latest_sequence: number
}
export type ExecutionEvent = {
  sequence: number
  type: "output" | "completed" | string
  execution_id: string
  stream?: "stdout" | "stderr" | string
  data?: string
  status?: ExecutionStatus
  exit_code?: number
  timed_out?: boolean
  timestamp: string
}
export type ExecutionFeedEvent = {
  sequence: number
  type: "started" | "output" | "completed" | string
  execution_id: string
  workspace_id: string
  execution?: ExecutionInfo
  stream?: "stdout" | "stderr" | string
  data?: string
  status?: ExecutionStatus
  exit_code?: number
  timed_out?: boolean
  timestamp: string
}
export type ExecutionFeedSnapshot = {
  events: ExecutionFeedEvent[]
  latest_sequence: number
}
export type InstructionSourcePolicy = {
  enabled?: boolean
  context?: boolean
  rules?: boolean
  skills?: boolean
}
export type GlobalInstructionRule = {
  id: string
  name?: string
  enabled: boolean
  content: string
}
export type InstructionSource = {
  provider: string
  kind: "context" | "rules" | "skills" | string
  paths: string[]
  count: number
  enabled: boolean
  loaded: boolean
}
export type GlobalInstructions = {
  version: number
  context: string
  rules: GlobalInstructionRule[]
  source_policy: Record<string, InstructionSourcePolicy>
  detected_sources: InstructionSource[]
}
export type InstructionRule = {
  path: string
  source: string
  patterns?: string[]
  content: string
  always_apply?: boolean
}
export type InstructionSkill = {
  name: string
  description?: string
  path?: string
  source?: string
}
export type InstructionSection = {
  path: string
  kind: string
  source?: string
  content: string
  truncated: boolean
  original_bytes?: number
  loaded_bytes: number
}
export type ProjectContextResult = {
  root: string
  workspace_id: string
  summary: {
    memory_files: {
      path: string
      kind: string
      source?: string
      truncated: boolean
    }[]
    memory_bytes: number
    instruction_bytes: number
    git: {
      skipped?: boolean
      is_repo: boolean
      branch?: string
      commits: number
    }
    rules: number
    skills: number
  }
  instruction_context: {
    root: string
    workspace_id: string
    instructions_text: string
    instruction_bytes: number
    instruction_truncated?: boolean
    global_context?: string
    global_rules: InstructionRule[]
    rules: InstructionRule[]
    skills: InstructionSkill[]
    sources: InstructionSource[]
    project_memory: {
      sections: InstructionSection[]
      imports?: InstructionSection[]
      total_bytes: number
      budget_bytes: number
      budget_truncated: boolean
    }
    auto_memory: { loaded: boolean; content?: string; bytes: number }
    git: {
      skipped?: boolean
      is_repo: boolean
      root?: string
      branch?: string
      status_short?: string
      recent_commits?: string[]
      error?: string
    }
    environment: Record<string, unknown>
    [key: string]: unknown
  }
}
export type ProjectContextOptions = {
  path?: string
  include_git?: boolean
  include_memory?: boolean
  include_skills?: boolean
}
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
export type MCPServerTools = {
  server_id: string
  tools: Tool[]
  proxied_tools: string[]
}
export type PublicConfig = {
  server: {
    enabled: boolean
    port: number
    expose: {
      mode: "none" | "all" | "0.0.0.0" | "interfaces"
      interfaces: string[]
    }
    allow_insecure_http: boolean
  }
  admin: { enabled: boolean; port: number }
  auth: {
    mcp_enabled: boolean
    admin_enabled: boolean
    mcp_token_configured: boolean
    mcp_token_revealable?: boolean
    admin_token_configured: boolean
  }
  permissions: { allow_dirs: string[] }
  shell: { path: string[] }
}
export type MCPToken = {
  token?: string
  configured: boolean
  revealable: boolean
  enabled: boolean
}
export type PluginLifecycle = {
  install: boolean
  uninstall: boolean
  enable: boolean
  disable: boolean
  update: boolean
  rollback: boolean
  prune: boolean
  verify: boolean
  configure: boolean
}
export type PluginListItem = {
  id: string
  name: string
  origin: "builtin" | "installed"
  origin_label: string
  enabled: boolean
  lifecycle: PluginLifecycle
  scope?: "global" | "workspace" | string
  workspace_id?: string
}
export type PluginSettingField = {
  key: string
  type: "string" | "boolean" | "integer" | "number" | "enum"
  title?: string
  description?: string
  default?: unknown
  required?: boolean
  enum?: string[]
  sensitive?: boolean
}
export type PluginConfig = {
  id: string
  name: string
  origin: "builtin" | "installed"
  scope: string
  workspace_id?: string
  schema: { fields: PluginSettingField[] }
  values: Record<string, unknown>
}
export type NetworkAddress = {
  address: string
  interface?: string
  scope: "local" | "lan" | "network" | string
}
export type NetworkInterface = { name: string; addresses: NetworkAddress[] }
export type TunnelAdminScope = {
  organization_id?: string
  workspace_id?: string
  tenant_id?: string
}
export type TunnelMetadata = {
  id: string
  name: string
  description: string
  creator?: string
  tenant_ids?: string[]
  workspace_ids?: string[]
  organization_ids?: string[]
  request_id?: string
  fetched_at: string
}
export type ManagedTunnelCreateRequest = {
  name: string
  description: string
  tenant_ids?: string[]
  workspace_ids?: string[]
  organization_ids?: string[]
}
export type ManagedTunnelUpdateRequest = {
  name?: string
  description?: string
  tenant_ids?: string[]
  workspace_ids?: string[]
  organization_ids?: string[]
}
export type TunnelStatus = {
  provider: "openai" | string
  enabled: boolean
  running: boolean
  ready: boolean
  restarting: boolean
  id?: string
  control_plane_base_url?: string
  organization_id?: string
  started_at?: string
  last_error?: string
  metadata?: TunnelMetadata
  metadata_error?: string
  admin_key_configured?: boolean
  admin_scope?: TunnelAdminScope
}
export type LocalTunnel = {
  id: string
  enabled: boolean
  runtime_key_configured: boolean
  admin_profile_id?: string
  control_plane_base_url?: string
  organization_id?: string
  status: TunnelStatus
}
export type LocalTunnelRequest = {
  id: string
  enabled: boolean
  api_key?: string
  admin_profile_id?: string
  control_plane_base_url?: string
  organization_id?: string
}
export type TunnelAdminProfile = {
  id: string
  key_configured: boolean
  organization_id?: string
  workspace_id?: string
  tenant_id?: string
  read_access: boolean
  manage_access: boolean
  control_plane_base_url?: string
}
export type TunnelAdminProfileRequest = {
  id?: string
  admin_key?: string
  organization_id?: string
  workspace_id?: string
  tenant_id?: string
  control_plane_base_url?: string
}
export type ManagedTunnel = {
  metadata: TunnelMetadata
  admin_profiles: string[]
}
export type ManagedTunnelAttachRequest = {
  runtime_api_key?: string
  auto_generate_runtime_key?: boolean
  project_id?: string
  enabled: boolean
}

export type ApprovalStatus =
  | "pending"
  | "approved"
  | "denied"
  | "expired"
  | "cancelled"
  | "consumed"
  | string
export type ApprovalRequest = {
  id: string
  status: ApprovalStatus
  workspace_id: string
  session_hash?: string
  source?: string
  tunnel_id?: string
  tunnel_name?: string
  target_tool: string
  arguments?: Record<string, unknown>
  digest?: string
  guard_code?: string
  guard_reason?: string
  title: string
  created_at: string
  expires_at: string
  resolved_at?: string
  resolved_by?: string
  reason?: string
  retry_until?: string
  consumed_at?: string
}
export type ApprovalEvent = {
  sequence?: number
  name: string
  request_id: string
  workspace_id: string
  session_hash?: string
  source?: string
  tunnel_id?: string
  tunnel_name?: string
  target_tool: string
  title: string
  status: ApprovalStatus
  created_at: string
  expires_at: string
  retry_until?: string
  timestamp: string
}

const adminTokenKey = "chatgpt-mcp-admin-token"
try {
  localStorage.removeItem(adminTokenKey)
} catch {
  /* storage may be unavailable */
}
export const adminToken = {
  get: () => sessionStorage.getItem(adminTokenKey) ?? "",
  set: (token: string) => sessionStorage.setItem(adminTokenKey, token),
  clear: () => sessionStorage.removeItem(adminTokenKey),
}
export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

export function authHeaders(): HeadersInit {
  const token = adminToken.get()
  return token ? { Authorization: `Bearer ${token}` } : {}
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body !== undefined && init.body !== null)
    headers.set("Content-Type", "application/json")
  const token = adminToken.get()
  if (token) headers.set("Authorization", `Bearer ${token}`)
  const response = await fetch(path, { ...init, headers })
  const text = response.status === 204 ? "" : await response.text()
  if (!response.ok)
    throw new ApiError(text.trim() || `API ${response.status}`, response.status)
  if (!text) return undefined as T
  try {
    return JSON.parse(text) as T
  } catch {
    throw new Error(`Invalid JSON response from ${path}`)
  }
}

function withAdmin(path: string, admin: string) {
  const value = admin.trim()
  return value ? `${path}?admin=${encodeURIComponent(value)}` : path
}

function pluginQuery(scope?: string, workspaceID?: string) {
  const query = new URLSearchParams()
  if (scope) query.set("scope", scope)
  if (workspaceID) query.set("workspace_id", workspaceID)
  const text = query.toString()
  return text ? `?${text}` : ""
}

export const adminApi = {
  health: () => api<{ ok: boolean; auth_enabled: boolean }>("/api/health"),
  activityCall: (callID: string) =>
    api<ActivityEvent>(`/api/activity/${encodeURIComponent(callID)}`),
  networkInterfaces: () => api<NetworkInterface[]>("/api/network/interfaces"),
  config: () => api<PublicConfig>("/api/config"),
  saveConfig: (config: Partial<PublicConfig>) =>
    api<PublicConfig>("/api/config", {
      method: "PUT",
      body: JSON.stringify(config),
    }),
  mcpToken: () => api<MCPToken>("/api/auth/mcp-token"),
  rotateMCPToken: () =>
    api<MCPToken>("/api/auth/mcp-token", { method: "POST" }),
  plugins: (scope?: string, workspaceID?: string) =>
    api<PluginListItem[]>(`/api/plugins${pluginQuery(scope, workspaceID)}`),
  pluginConfig: (id: string, scope?: string, workspaceID?: string) =>
    api<PluginConfig>(`/api/plugins/${encodeURIComponent(id)}/config${pluginQuery(scope, workspaceID)}`),
  savePluginConfig: (id: string, values: Record<string, unknown>, scope?: string, workspaceID?: string) =>
    api<PluginConfig>(`/api/plugins/${encodeURIComponent(id)}/config${pluginQuery(scope, workspaceID)}`, {
      method: "PUT",
      body: JSON.stringify({ values }),
    }),
  resetPluginConfig: (id: string, scope?: string, workspaceID?: string) =>
    api<PluginConfig>(`/api/plugins/${encodeURIComponent(id)}/config/reset${pluginQuery(scope, workspaceID)}`, {
      method: "POST",
    }),
  workspaces: () => api<Workspace[]>("/api/workspaces"),
  workspace: (id: string) =>
    api<Workspace>(`/api/workspaces/${encodeURIComponent(id)}`),
  registerWorkspace: (path: string) =>
    api<Workspace>("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({ path }),
    }),
	removeWorkspace: (id: string) =>
    api<void>(`/api/workspaces/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  deleteWorkspaceState: (id: string) =>
    api<void>(`/api/workspaces/${encodeURIComponent(id)}/state`, {
      method: "DELETE",
    }),
  workspaceContainers: () =>
    api<WorkspaceContainer[]>("/api/workspace-containers"),
  workspaceContainer: (id: string) =>
    api<WorkspaceContainer>(
      `/api/workspace-containers/${encodeURIComponent(id)}`
    ),
  createWorkspaceContainer: (name: string) =>
    api<WorkspaceContainer>("/api/workspace-containers", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
  renameWorkspaceContainer: (id: string, name: string) =>
    api<WorkspaceContainer>(
      `/api/workspace-containers/${encodeURIComponent(id)}`,
      { method: "PATCH", body: JSON.stringify({ name }) }
    ),
  removeWorkspaceContainer: (id: string) =>
    api<void>(`/api/workspace-containers/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  workspaceContainersForWorkspace: (id: string) =>
    api<WorkspaceContainer[]>(
      `/api/workspaces/${encodeURIComponent(id)}/containers`
    ),
  addWorkspaceContainers: (id: string, containerIDs: string[]) =>
    api<WorkspaceContainer[]>(
      `/api/workspaces/${encodeURIComponent(id)}/containers`,
      { method: "POST", body: JSON.stringify({ container_ids: containerIDs }) }
    ),
  removeWorkspaceContainers: (id: string, containerIDs: string[]) =>
    api<WorkspaceContainer[]>(
      `/api/workspaces/${encodeURIComponent(id)}/containers`,
      {
        method: "DELETE",
        body: JSON.stringify({ container_ids: containerIDs }),
      }
    ),
  globalInstructions: () => api<GlobalInstructions>("/api/instructions/global"),
  saveGlobalInstructions: (
    patch: Partial<
      Pick<GlobalInstructions, "context" | "rules" | "source_policy">
    >
  ) =>
    api<GlobalInstructions>("/api/instructions/global", {
      method: "PUT",
      body: JSON.stringify(patch),
    }),
  workspaceContext: (id: string, options: ProjectContextOptions = {}) => {
    const query = new URLSearchParams()
    if (options.path?.trim()) query.set("path", options.path.trim())
    if (options.include_git !== undefined)
      query.set("include_git", String(options.include_git))
    if (options.include_memory !== undefined)
      query.set("include_memory", String(options.include_memory))
    if (options.include_skills !== undefined)
      query.set("include_skills", String(options.include_skills))
    const suffix = query.size ? `?${query}` : ""
    return api<ProjectContextResult>(
      `/api/workspaces/${encodeURIComponent(id)}/context${suffix}`
    )
  },
  workspaceExecutions: (id: string, limit = 50) =>
    api<ExecutionInfo[]>(
      `/api/workspaces/${encodeURIComponent(id)}/executions?limit=${limit}`
    ),
  workspaceExecution: (id: string, executionID: string) =>
    api<ExecutionSnapshot>(
      `/api/workspaces/${encodeURIComponent(id)}/executions/${encodeURIComponent(executionID)}`
    ),
  tools: () => api<Tool[]>("/api/tools"),
  upstream: () => api<MCPServer[]>("/api/upstream"),
  upstreamServer: (id: string) =>
    api<MCPServer>(`/api/upstream/${encodeURIComponent(id)}`),
  addUpstream: (server: MCPServer) =>
    api<MCPServer>("/api/upstream", {
      method: "POST",
      body: JSON.stringify(server),
    }),
  updateUpstream: (id: string, server: MCPServer) =>
    api<MCPServer>(`/api/upstream/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(server),
    }),
  removeUpstream: (id: string) =>
    api<void>(`/api/upstream/${encodeURIComponent(id)}`, { method: "DELETE" }),
  upstreamStatus: (id: string, refresh = true) =>
    api<MCPServerStatus>(
      `/api/upstream/${encodeURIComponent(id)}/status?refresh=${refresh}`
    ),
  upstreamTools: (id: string, refresh = false) =>
    api<MCPServerTools>(
      `/api/upstream/${encodeURIComponent(id)}/tools?refresh=${refresh}`
    ),
  localTunnels: () => api<LocalTunnel[]>("/api/tunnels"),
  attachLocalTunnel: (request: LocalTunnelRequest) =>
    api<LocalTunnel>("/api/tunnels", {
      method: "POST",
      body: JSON.stringify(request),
    }),
  localTunnel: (id: string) =>
    api<LocalTunnel>(`/api/tunnels/${encodeURIComponent(id)}`),
  updateLocalTunnel: (id: string, request: LocalTunnelRequest) =>
    api<LocalTunnel>(`/api/tunnels/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(request),
    }),
  detachLocalTunnel: (id: string) =>
    api<void>(`/api/tunnels/${encodeURIComponent(id)}`, { method: "DELETE" }),
  enableLocalTunnel: (id: string) =>
    api<LocalTunnel>(`/api/tunnels/${encodeURIComponent(id)}/enable`, {
      method: "POST",
    }),
  disableLocalTunnel: (id: string) =>
    api<LocalTunnel>(`/api/tunnels/${encodeURIComponent(id)}/disable`, {
      method: "POST",
    }),
  startLocalTunnel: (id: string) =>
    api<LocalTunnel>(`/api/tunnels/${encodeURIComponent(id)}/start`, {
      method: "POST",
    }),
  stopLocalTunnel: (id: string) =>
    api<LocalTunnel>(`/api/tunnels/${encodeURIComponent(id)}/stop`, {
      method: "POST",
    }),
  tunnelAdminProfiles: () => api<TunnelAdminProfile[]>("/api/tunnel-admins"),
  addTunnelAdminProfile: (request: TunnelAdminProfileRequest) =>
    api<TunnelAdminProfile>("/api/tunnel-admins", {
      method: "POST",
      body: JSON.stringify(request),
    }),
  tunnelAdminProfile: (id: string) =>
    api<TunnelAdminProfile>(`/api/tunnel-admins/${encodeURIComponent(id)}`),
  updateTunnelAdminProfile: (id: string, request: TunnelAdminProfileRequest) =>
    api<TunnelAdminProfile>(`/api/tunnel-admins/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(request),
    }),
  verifyTunnelAdminProfile: (id: string) =>
    api<TunnelAdminProfile>(
      `/api/tunnel-admins/${encodeURIComponent(id)}/verify`,
      { method: "POST" }
    ),
  removeTunnelAdminProfile: (id: string) =>
    api<void>(`/api/tunnel-admins/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  managedTunnelCollection: (admin = "") =>
    api<ManagedTunnel[]>(withAdmin("/api/managed-tunnels", admin)),
  managedTunnelByProfile: (id: string, admin: string) =>
    api<ManagedTunnel>(
      withAdmin(`/api/managed-tunnels/${encodeURIComponent(id)}`, admin)
    ),
  createManagedTunnelByProfile: (
    request: ManagedTunnelCreateRequest,
    admin: string
  ) =>
    api<ManagedTunnel>(withAdmin("/api/managed-tunnels", admin), {
      method: "POST",
      body: JSON.stringify(request),
    }),
  updateManagedTunnelByProfile: (
    id: string,
    request: ManagedTunnelUpdateRequest,
    admin: string
  ) =>
    api<ManagedTunnel>(
      withAdmin(`/api/managed-tunnels/${encodeURIComponent(id)}`, admin),
      { method: "PUT", body: JSON.stringify(request) }
    ),
  deleteManagedTunnelByProfile: (id: string, admin: string) =>
    api<ManagedTunnel>(
      withAdmin(`/api/managed-tunnels/${encodeURIComponent(id)}`, admin),
      { method: "DELETE" }
    ),
  attachManagedTunnel: (
    id: string,
    request: ManagedTunnelAttachRequest,
    admin: string
  ) =>
    api<LocalTunnel>(
      withAdmin(`/api/managed-tunnels/${encodeURIComponent(id)}`, admin),
      { method: "POST", body: JSON.stringify(request) }
    ),
  approvalRequests: (status = "pending", workspaceID = "") => {
    const query = new URLSearchParams({ status })
    if (workspaceID) query.set("workspace_id", workspaceID)
    return api<ApprovalRequest[]>(`/api/requests?${query}`)
  },
  approvalRequest: (id: string) =>
    api<ApprovalRequest>(`/api/requests/${encodeURIComponent(id)}`),
  approveRequest: (id: string, reason = "") =>
    api<ApprovalRequest>(`/api/requests/${encodeURIComponent(id)}/approve`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
  denyRequest: (id: string, reason = "") =>
    api<ApprovalRequest>(`/api/requests/${encodeURIComponent(id)}/deny`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
}
