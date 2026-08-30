# Connect ChatGPT with OpenAI Secure MCP Tunnel

This is the recommended end-to-end path for connecting a private or local `chatgpt-mcp` runtime to ChatGPT without exposing the MCP server directly to the public internet.

`chatgpt-mcp` embeds OpenAI's Secure MCP Tunnel client, so you do **not** need to install a separate `tunnel-client`, `cloudflared`, or `ngrok` process for the normal setup.

> OpenAI changes product availability and UI over time. This guide follows the current official Secure MCP Tunnel and ChatGPT Developer Mode documentation. If labels differ, use the official links at the bottom of this page as the source of truth.

## How the connection works

```text
ChatGPT
   │
   │ OpenAI-hosted MCP tunnel endpoint
   │
   ▼
OpenAI Tunnel control plane
   ▲
   │ outbound HTTPS :443
   │
chatgpt-mcp
   ├─ embedded Secure MCP Tunnel client
   ├─ MCP runtime
   └─ private/local workspaces and tools
```

The connection is outbound-only from your machine to OpenAI. You do not need to open an inbound firewall port for the tunnel path.

## What you need

Before configuring `chatgpt-mcp`, you need:

1. Access to OpenAI Platform tunnel settings.
2. A tunnel with an ID such as `tunnel_...`.
3. A **runtime API key** allowed to use that tunnel.
4. The tunnel associated with the ChatGPT workspace/account that should discover it.
5. ChatGPT Developer Mode access for the user creating the app.
6. A configured and running `chatgpt-mcp` instance.

## Keep these values separate

| Value | Example | Secret? | Purpose |
| --- | --- | --- | --- |
| Tunnel ID | `tunnel_...` | No | Identifies the OpenAI-hosted tunnel object |
| Runtime API key | `sk-...` | **Yes** | Authenticates the long-lived tunnel runtime |
| Admin API key | `sk-admin-...` / Platform admin key | **Yes** | Tunnel CRUD/admin operations only; not needed by `chatgpt-mcp` runtime |
| MCP token | `mcp_...` | **Yes** | Authenticates direct access to the local/public MCP HTTP endpoint when MCP auth is enabled |
| Admin token | `admin_...` | **Yes** | Authenticates the embedded admin UI/API when Admin auth is enabled |

The OpenAI runtime key is **not** used by `chatgpt-mcp` to call a language model. It only authenticates the Secure MCP Tunnel client to OpenAI's tunnel control plane.

## 1. Get the required Platform permissions

OpenAI tunnel permissions are organization-level.

Recommended split:

### Runtime user

Needs:

```text
Tunnels: Read
Tunnels: Use
```

This is the minimum role for the principal creating the runtime key used by `chatgpt-mcp`, and for users who need to select/use an existing tunnel.

### Tunnel manager

Needs:

```text
Tunnels: Read
Tunnels: Manage
```

Add `Use` as well if the same operator also runs the tunnel or attaches it in ChatGPT.

OpenAI Platform role management:

- Roles: https://platform.openai.com/settings/organization/people/roles
- Groups: https://platform.openai.com/settings/organization/people/groups

If your organization uses RBAC, prefer assigning tunnel roles to a group instead of directly broadening individual permissions.

## 2. Create the tunnel and copy its ID

Open:

https://platform.openai.com/settings/organization/tunnels

Create a tunnel and give it a recognizable name, for example:

```text
mew-dev-machine
```

After creation, copy the tunnel ID:

```text
tunnel_0123456789abcdef0123456789abcdef
```

You will pass this value to `cmcp tunnel configure` and select the same tunnel in ChatGPT.

### Associate the tunnel with the right workspace

This step is important.

A tunnel can be associated with Platform organizations and ChatGPT workspaces. If you want the tunnel to appear when creating a ChatGPT developer-mode app, include the target **ChatGPT workspace** in the tunnel's associations.

A tunnel associated only with a Platform organization does not automatically appear in every ChatGPT Enterprise/Edu workspace.

## 3. Create the runtime API key

Open:

https://platform.openai.com/settings/organization/api-keys

Create a **Restricted** runtime API key for the tunnel daemon. Grant only:

```text
Tunnels: Read
Tunnels: Use
```

Copy the key when OpenAI displays it. Treat it as a secret.

Do **not** use an OpenAI Admin API key as the long-lived tunnel runtime key.

A useful mental model is:

```text
tunnel_... = which tunnel to use
sk-...     = permission for the local runtime to use it
```

## 4. Initialize chatgpt-mcp

If you have not initialized the runtime yet:

```bash
cmcp init
```

Optionally register the project(s) ChatGPT should work with:

```bash
cmcp workspace register ~/projects/my-project
```

## 5. Configure the embedded tunnel

```bash
cmcp tunnel configure \
  --enabled \
  --id tunnel_0123456789abcdef0123456789abcdef \
  --api-key 'sk-...'
```

Optional OpenAI-specific settings:

```bash
cmcp tunnel configure \
  --control-plane-base-url https://api.openai.com \
  --organization-id org_...
```

The runtime API key is stored separately from the main configuration using the selected config format. It is never returned by normal config/status commands.

Inspect the result:

```bash
cmcp tunnel status
```

## 6. Start chatgpt-mcp

For interactive testing:

```bash
cmcp serve
```

For normal background use:

```bash
cmcp up
```

Then verify:

```bash
cmcp status
cmcp tunnel status
cmcp logs --component TUNNEL -f
```

The tunnel must remain connected while ChatGPT scans tools or invokes them.

### Remote Linux server / SSH

If you run only:

```bash
cmcp serve
```

it is a foreground process tied to that terminal/session.

For a user-level managed service:

```bash
cmcp up
```

On Linux, this uses `systemd --user`. If user lingering is disabled, the CLI warns that the user manager may stop after the final login/SSH session ends.

For a machine-level systemd service that starts with the machine:

```bash
sudo cmcp up
```

The systemd unit is system-level, but `chatgpt-mcp` itself still runs as the invoking `SUDO_USER`, not as root.

See [Runtime and services](runtime.md) for the complete lifecycle.

## 7. Enable ChatGPT Developer Mode

OpenAI treats ChatGPT Developer Mode permission separately from Platform tunnel permissions.

Current OpenAI guidance:

- Business: admins/owners can enable Developer Mode and create/test custom MCP apps.
- Enterprise/Edu: workspace admins can grant Developer Mode access through workspace permissions/RBAC; authorized users can then enable it for their account.
- OpenAI currently documents full MCP/write support for Business and Enterprise/Edu, while Pro has more limited custom MCP support. Check the current Help Center article before relying on plan-specific behavior.

Depending on workspace plan and role, enable Developer Mode from one of the current OpenAI settings surfaces:

```text
Settings → Apps → Advanced Settings → Developer Mode
```

or start from:

```text
Workspace Settings → Apps → Create
```

OpenAI may show the enablement prompt as part of creating a custom app.

## 8. Create the ChatGPT MCP app

Open ChatGPT app/connector settings:

https://chatgpt.com/#settings/Connectors

Then:

1. Choose **Create** / the plus button for a developer-mode custom app.
2. Enter the app name and metadata you want ChatGPT users to see.
3. Under **Connection**, choose **Tunnel**.
4. Select your tunnel from the list, or paste the `tunnel_...` ID when allowed.
5. Choose the app authentication mechanism if your MCP server requires app-level auth. The Secure MCP Tunnel runtime key is transport authentication and is not pasted here as the app's bearer token.
6. Click **Scan Tools**.
7. Wait for tool discovery to finish.
8. Review the discovered tools and permissions.
9. Click **Create**.

For workspace plans, the app may first appear as a draft. Admins/owners can review and publish it according to the workspace's app policy.

## 9. Verify from ChatGPT

Before testing a prompt, verify locally:

```bash
cmcp status
cmcp tunnel status
cmcp logs --component TUNNEL -n 100
```

Then enable/select the custom app in ChatGPT and ask for a read-only action first, for example listing registered workspaces or getting runtime version/status.

For live inspection while testing:

```bash
cmcp logs -f
```

For full diagnostics:

```bash
cmcp logs --debug -f
```

## Tunnel lifecycle commands

```bash
cmcp tunnel status
cmcp tunnel enable
cmcp tunnel disable
cmcp tunnel run
```

`tunnel run` runs only the builtin tunnel in the foreground. Normal `cmcp serve` / `cmcp up` automatically starts the tunnel when it is configured and enabled.

Unexpected tunnel failures are supervised with bounded exponential reconnect backoff. Explicit disable/reconfigure/shutdown does not trigger an unwanted reconnect.

## What not to do

- Do not expose the local MCP port publicly just to make Secure MCP Tunnel work.
- Do not paste your runtime API key into the ChatGPT app's normal auth field.
- Do not use an OpenAI Admin API key as the daemon runtime key.
- Do not commit runtime keys, MCP tokens, admin tokens, or exported config secrets.
- Do not give the runtime key `Manage` permission unless the same principal genuinely needs tunnel CRUD.
- Do not assume a tunnel visible in Platform is automatically associated with the correct ChatGPT workspace.

## If the tunnel does not appear in ChatGPT

Check, in this order:

1. The tunnel is associated with the target ChatGPT workspace.
2. Your Platform principal has **Tunnels Read + Use**.
3. Your ChatGPT user has Developer Mode access enabled.
4. `cmcp tunnel status` reports the configured tunnel.
5. `cmcp` is still running.
6. `cmcp logs --component TUNNEL --debug -n 200` does not show authentication or polling failures.
7. Wait briefly after creating/changing the tunnel or RBAC assignment; OpenAI notes that permission changes can take time to propagate.

See [Troubleshooting](troubleshooting.md) for more cases.

## Official OpenAI references

- Secure MCP Tunnel: https://developers.openai.com/api/docs/guides/secure-mcp-tunnels
- Developer Mode and MCP apps in ChatGPT: https://help.openai.com/en/articles/12584461
- Platform Tunnels: https://platform.openai.com/settings/organization/tunnels
- Runtime API keys: https://platform.openai.com/settings/organization/api-keys
- Organization roles: https://platform.openai.com/settings/organization/people/roles
- Organization groups: https://platform.openai.com/settings/organization/people/groups
- ChatGPT app/connector settings: https://chatgpt.com/#settings/Connectors
- OpenAI tunnel-client source/docs: https://github.com/openai/tunnel-client
