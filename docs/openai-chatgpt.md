# Connect ChatGPT with OpenAI Secure MCP Tunnel

OpenAI Secure MCP Tunnel is the default way to connect ChatGPT to `chatgpt-mcp`. Tunnel connections are established outbound from your machine, so the local MCP runtime does not need a public inbound port. One `chatgpt-mcp` runtime can attach multiple tunnels concurrently; every tunnel ingress reaches the same local tools, workspaces, approvals, processes, upstream MCP servers, and plugin registry.

```text
ChatGPT
   │
   ▼
OpenAI Secure MCP Tunnel A/B/...
   │ outbound HTTPS
   ▼
chatgpt-mcp
   │
   └─ registered local workspaces and tools
```

## What you need

- A running `chatgpt-mcp` installation.
- An OpenAI Platform tunnel (`tunnel_...`).
- A restricted runtime API key with **Tunnels Read + Use** for each attached tunnel.
- A Platform admin key/scope when using `cgm` to discover/manage tunnels or automatically create runtime keys.
- The tunnel associated with the ChatGPT workspace/account that should discover it.
- ChatGPT Developer Mode access for the user creating the custom app.

The runtime API key is only for the tunnel transport. It is not used to call a language model.

## Keep these values separate

| Value | Secret? | Purpose |
| --- | --- | --- |
| Tunnel ID (`tunnel_...`) | No | Selects the OpenAI-hosted tunnel |
| Runtime API key (`sk-...`) | Yes | Lets one local tunnel instance use its OpenAI tunnel |
| Platform Admin API key | Yes | Stored in an admin profile for management; never used as the runtime credential |
| Direct MCP HTTP token | Yes | Authenticates direct local `/mcp` HTTP only. Not used by Secure MCP Tunnel. Reuse it when adding this MCP server to ChatGPT. |
| Admin token | Yes | Authenticates Admin API/UI. Independent of Direct MCP HTTP and tunnel keys. |

Runtime and management credentials remain separate. Multiple admin profiles may coexist, and each local tunnel instance may reference the profile that discovered/managed it while keeping its own runtime key.

## 1. Create the tunnel

Open OpenAI Platform tunnel settings:

https://platform.openai.com/settings/organization/tunnels

Create a tunnel and copy its ID:

```text
tunnel_...
```

Associate the tunnel with the ChatGPT workspace that should use it. A tunnel that exists in Platform but is not associated with the target ChatGPT workspace may not appear when creating the custom app.

## 2. Create the runtime API key

Open:

https://platform.openai.com/settings/organization/api-keys

Create a **Restricted** key for the tunnel runtime with:

```text
Tunnels: Read
Tunnels: Use
```

Do not use a Platform Admin API key as the long-lived runtime key.

Tunnel creation/editing is an administrative task and requires **Tunnels Read + Manage** for the operator performing it. That permission does not need to be granted to the daemon's runtime key.

## 3. Initialize and register a workspace

If needed:

```bash
cgm init
cgm workspace register ~/projects/my-project
```

See [Workspaces](workspaces.md) for how workspace scope works.

## 4. Add an admin profile and attach the tunnel

```bash
cgm tunnel admin add personal --admin-key 'sk-admin-...' --organization-id org_...
cgm tunnel admin verify personal
cgm tunnel attach tunnel_... --admin personal --runtime-api-key 'sk-...'
```

For a workspace/tenant scoped admin profile, use `--workspace-id` or `--tenant-id` instead of `--organization-id`. Exactly one management scope is required.

If the selected profile may create a restricted runtime key:

```bash
cgm tunnel attach tunnel_... --admin personal --auto-runtime-key
```

Use `--project-id` when automatic key generation cannot resolve a single project.

Inspect the result:

```bash
cgm tunnel list
cgm tunnel status tunnel_...
```

Runtime keys and admin-profile keys are kept in the selected config root's managed secret store and are not printed by normal status/config output. `attach` adds a local tunnel instance; it does not select/replace a global tunnel.

## 5. Start the runtime

For normal use:

```bash
cgm up
```

Verify:

```bash
cgm status
cgm tunnel status
```

The intended tunnel should reach its ready/connected state before ChatGPT scans or invokes tools. Each attached tunnel reconnects independently; a degraded tunnel does not stop another ready tunnel from serving the shared runtime.

For foreground testing only:

```bash
cgm serve
```

See [Runtime and operations](runtime.md) for service behavior, remote Linux usage, and logs.

## 6. Enable ChatGPT Developer Mode

OpenAI controls Developer Mode access separately from Platform tunnel permissions. Availability and the exact settings surface vary by ChatGPT plan/workspace policy, so use the current Help Center article below as the source of truth.

Current settings commonly appear under:

```text
Settings → Apps → Advanced Settings → Developer Mode
```

or from the workspace app-creation flow.

If the UI differs, use OpenAI's current Help Center guidance linked below.

## 7. Create the ChatGPT app

Open **Settings → Apps → Create** (or **Workspace Settings → Apps → Create** when your workspace policy uses the admin surface).

Then:

1. Create a developer-mode custom app.
2. Enter the app metadata you want users to see.
3. Choose **Tunnel** for the connection.
4. Select or enter one of the `tunnel_...` IDs attached to `chatgpt-mcp`.
5. Configure app-level authentication only if your MCP surface requires it. Do **not** paste the tunnel runtime key into the app's normal bearer-auth field.
6. Run **Scan Tools**.
7. Review the discovered tools and create/enable the app.

## 8. Verify from ChatGPT

Before the first prompt, check locally:

```bash
cgm status
cgm tunnel status
```

Then try a read-only action in ChatGPT, such as listing registered workspaces or reading runtime/version information.

For live diagnostics while testing:

```bash
cgm logs --component TUNNEL -f
```

For full diagnostics:

```bash
cgm logs --component TUNNEL --debug -f
```

## Common problems

### The tunnel does not appear in ChatGPT

Check:

1. The tunnel is associated with the target ChatGPT workspace.
2. Your Platform principal has the required tunnel permissions.
3. Your ChatGPT user has Developer Mode access.
4. `cgm tunnel status <id>` shows the intended tunnel.
5. The runtime is still running and the tunnel is connected.

Permission/association changes can take time to propagate.

### Tunnel authentication fails

The runtime key likely lacks **Tunnels Read + Use**, belongs to the wrong scope, or is no longer valid. Detach/reattach the affected tunnel with a corrected runtime key, or use the admin profile's automatic runtime-key flow.

### Scan Tools fails

Keep `chatgpt-mcp` running during discovery and inspect `cgm tunnel status` plus tunnel logs. See [Troubleshooting](troubleshooting.md) for more cases.

## What not to do

- Do not expose the local MCP HTTP port publicly just to use Secure MCP Tunnel.
- Do not use an OpenAI Admin API key as the long-lived runtime key.
- Do not paste the tunnel runtime key into the ChatGPT app's normal auth field.
- Do not use the Direct MCP HTTP token as a tunnel runtime or admin key. Secure MCP Tunnel neither requires nor accepts it.
- Do not commit runtime keys, Direct MCP HTTP tokens, Admin tokens, or exported secrets.
- Do not grant `Manage` to the runtime key unless the same principal genuinely needs tunnel administration.

## Official OpenAI references

- Secure MCP Tunnel: https://developers.openai.com/api/docs/guides/secure-mcp-tunnels
- Developer Mode and MCP apps in ChatGPT: https://help.openai.com/en/articles/12584461
- Platform Tunnels: https://platform.openai.com/settings/organization/tunnels
- Runtime API keys: https://platform.openai.com/settings/organization/api-keys
- Organization roles: https://platform.openai.com/settings/organization/people/roles
