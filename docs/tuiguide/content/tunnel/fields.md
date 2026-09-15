# Tunnel Editor Fields

## Admin Profile

### Profile ID

Stable local name used to select a management credential, for example `personal` or `work`.

### OpenAI admin API key

Sensitive management credential. It is stored in the managed secret store and is never used as a tunnel runtime key.

### Scope type and Scope ID

Exactly one management scope is required: Organization, Workspace, or Tenant. Scope ID contains the matching OpenAI identifier.

### Control plane base URL

Optional profile-specific control-plane override. Empty uses the normal OpenAI control-plane endpoint.

## Managed Tunnel — General

### Name

Human-readable managed tunnel name. Required when creating a tunnel.

### Description

Managed tunnel description. Required on create; update allows the existing description to be changed without the create-only requirement.

## Managed Tunnel — Scope

### Organization IDs

Optional multiline list of organization IDs, one per line. Values are normalized before being sent to the management API.

### Workspace IDs

Optional multiline list of workspace IDs, one per line.

### Tenant IDs

Optional multiline list of tenant IDs, one per line.

## Attach Managed Tunnel

### Admin profile

Chooses the management credential/provenance used to read the remote tunnel and, when requested, generate a runtime key. Explicit selection is required when multiple profiles can manage the same tunnel.

### Runtime key mode

Choose a manually supplied restricted runtime key or automatic generation through the selected admin profile.

### Runtime API key

Sensitive **Tunnels Read + Use** credential for the new local tunnel instance. It is password-style and stored separately from admin-profile credentials.

### Project ID

Optional OpenAI project used when automatic runtime-key generation cannot resolve one unambiguously.

### Enabled

Controls whether the newly attached local instance participates in runtime startup immediately. At least one MCP transport must remain enabled overall.

## Delete Managed Tunnel

### Admin profile

Chooses the management profile used for the remote destructive operation. A local attachment with the same tunnel ID must be detached before remote deletion.
