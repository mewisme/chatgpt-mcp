# Tunnel

Tunnel contains two related surfaces: the local runtime tunnel configuration and managed OpenAI Secure MCP Tunnels.

## Runtime tunnel

The Tunnel dashboard shows whether the runtime tunnel is enabled/configured and exposes actions such as configure, enable/disable, foreground run guidance, metadata sync, and admin-key management.

**Configure runtime tunnel** opens a full-page editor. The enabled state uses a Switch. Runtime API key input uses a password-style field; when an existing secret can be reused, that behavior is explained in the placeholder rather than a separate label description.

Save is asynchronous. A backend failure keeps the exact editor draft and feedback. A successful save commits the editor baseline before returning to the dashboard.

## Admin key

The admin key enables OpenAI tunnel-management operations. The editor verifies and stores the key without rendering the secret afterward. Verify and remove remain lifecycle actions; removal requires confirmation.

## Managed tunnels

The Managed Tunnels page lists tunnels available through the OpenAI management API. Create, Update, and Configure are routed editors divided into consistent sections such as `General / Scope / Runtime`.

Editing an existing managed tunnel first fetches current remote metadata. The loading state can be cancelled with `Esc`; a late fetch result after cancellation is ignored. Fetch errors render an explicit wrapped error page instead of a blank editor.

## Configure local runtime from a managed tunnel

**Use managed tunnel** writes the selected managed tunnel into the local runtime configuration. Blank secret/key fields can preserve an existing configured secret when that is supported by the operation.

## Delete behavior

Deleting a managed tunnel is destructive and uses confirmation rather than a data-entry form. If local runtime configuration points at the tunnel being deleted, the confirmation flow can also handle clearing that local configuration explicitly. The TUI does not hide this consequence inside a generic form toggle.