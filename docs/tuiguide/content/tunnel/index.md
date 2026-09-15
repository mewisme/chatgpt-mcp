# Tunnel

Tunnel is collection-first: one local runtime can receive traffic from multiple OpenAI Secure MCP Tunnels at the same time.

## Local tunnel instances

The top-level Tunnel page lists every attached local tunnel instance. Each row/detail is keyed by tunnel ID and shows enabled/configured/live state without rendering the runtime key.

Actions are instance-scoped:

- enable/disable changes only that tunnel's desired state;
- start/stop changes only that live tunnel connection;
- detach removes only the local attachment/runtime key and leaves the remote OpenAI tunnel unchanged.

All instances share the same local MCP runtime, tools, workspaces, approvals, processes, upstream servers, and plugins. Connection/reconnect/error state remains independent per tunnel.

## Admin profiles

Admin profiles are named management credentials. A profile stores one OpenAI admin key plus exactly one organization/workspace/tenant scope and optional control-plane override.

Verification records demonstrated read/manage capability without showing the key. Multiple profiles may coexist. They do not create runtime connections and their keys are never substituted for per-tunnel runtime keys.

## Managed tunnels

Managed Tunnels is the remote OpenAI resource browser. Discovery can aggregate all admin profiles while retaining which profiles can access each remote tunnel.

If one remote tunnel is visible through multiple profiles, mutations require choosing a specific admin profile. Create/update/delete similarly use an explicit profile when selection is not unambiguous.

## Attach

Attach converts a readable remote managed tunnel into a new local tunnel instance. It does not select or replace an existing local tunnel.

The attach flow chooses an admin profile and a runtime-key strategy:

- provide a separate restricted **Tunnels Read + Use** runtime key; or
- ask the selected admin profile to generate a restricted runtime key, optionally choosing an OpenAI project.

The new instance can be attached enabled or disabled. Its runtime key is stored as a managed secret and is never rendered afterward.

CLI equivalents include `cgm tunnel attach <id>`, `cgm tunnel detach <id>`, `cgm tunnel status <id>`, and the ID-scoped lifecycle commands.

## Remote delete

Deleting a managed tunnel deletes the remote OpenAI resource. An attached local instance must be detached first so remote deletion cannot silently invalidate a currently configured local connection.
