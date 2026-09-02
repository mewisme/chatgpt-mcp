# Cluster federation

Cluster federation lets multiple `chatgpt-mcp` runtimes behave like one workspace directory. Each runtime keeps its own local workspace roots, connects outbound to one relay, and routes workspace-bound tool calls to the runtime that owns the requested `workspace_id`.

```text
ChatGPT / MCP client
        │
        ▼
 runtime A ─────────────┐
   ws_a                 │
                       ▼
                  cluster relay
                       ▲
 runtime B ─────────────┘
   ws_b
```

The relay never receives local filesystem paths as ownership authority. Runtimes advertise stable instance IDs, catalog hashes, and instance-scoped workspace IDs; actual tool execution stays on the owning runtime.

## What the cluster coordinates

- runtime membership and online/offline state
- workspace ownership and remote tool routing
- tool-catalog compatibility
- RPC transport between runtimes
- a single OpenAI Secure MCP Tunnel leader when cluster members share one configured tunnel ID
- tunnel leadership lease/fencing state for the lifetime of the relay

When the relay connection drops, a runtime fails pending cluster RPCs, marks itself disconnected, and reconnects with exponential backoff and jitter. After reconnect it re-advertises its current instance/workspace/catalog state. A tunnel leader demotes immediately on relay loss and leadership is reconciled again after reconnect.

## 1. Run a relay

Create a relay token in a trusted terminal:

```bash
cgm init
cgm config set cluster.relay_token 'replace-with-a-long-random-secret'
cgm cluster relay
```

Defaults:

```text
listen: 127.0.0.1:37423
WebSocket: /cluster
health: /health
metrics: /metrics
```

For a service or container, prefer a secret file instead of putting a token on the command line:

```bash
cgm cluster relay --token-file /run/secrets/cluster-token
```

`--token-file` does not require an initialized config root. The file is read once at startup and should be readable only by the relay service identity.

Useful hardening flags:

```text
--max-connections 256
--max-requests-per-second 256
--hello-timeout 10s
--idle-timeout 30s
--write-timeout 10s
```

The request limit is per WebSocket connection. The relay also caps incoming WebSocket messages at 4 MiB.

## 2. Configure each runtime

On every participating runtime, configure the same relay URL/token:

```bash
cgm config set cluster.relay_url wss://relay.example.com/cluster
cgm config set cluster.relay_token 'replace-with-the-relay-secret'
cgm config set cluster.enabled true
cgm config reload
```

For a loopback-only development relay, `ws://127.0.0.1:37423/cluster` is accepted. Remote relay URLs must use `wss://`.

Cluster settings are also available in **Admin → Settings → Cluster**. Relay tokens are never returned to the browser; the UI only receives whether a token is configured. Saving Admin settings applies through the same transactional live-reload path and rolls back if the new live cluster configuration cannot be applied.

## 3. Register local workspaces

Each runtime registers only the roots physically available to that runtime:

```bash
# runtime A
cgm workspace register ~/projects/a

# runtime B
cgm workspace register ~/projects/b
```

Workspace IDs are scoped by the stable runtime instance identity. Two runtimes can therefore register the same textual path without producing the same workspace ID.

Once both runtimes are connected, a workspace-bound tool call received by runtime A for runtime B's `workspace_id` is routed to B. Mutating tools execute only on the owner; the receiving runtime does not mirror the mutation locally.

## Inspect cluster state

```bash
cgm cluster status
cgm status
cgm --log-format=json cluster status
```

Status includes:

- connected/disconnected state
- local instance ID/name
- relay URL
- known and online member counts
- workspace directory
- catalog compatibility/hash
- tunnel role (`leader`, `standby`, `disabled`, or `not configured`)
- leader instance and fencing epoch when available
- reconnect/catalog errors

`cluster status` is read-only and is permitted from MCP tool execution context. `cluster relay` remains a control-plane operation and is denied there.

## Health and metrics

Health is unauthenticated so a local reverse proxy/orchestrator can probe it:

```bash
curl http://127.0.0.1:37423/health
```

Example fields:

```json
{
  "ok": true,
  "started_at": "2026-09-02T00:00:00Z",
  "uptime_seconds": 120,
  "active_connections": 2
}
```

Metrics are JSON and require the relay bearer token:

```bash
curl -H 'Authorization: Bearer <relay-token>' http://127.0.0.1:37423/metrics
```

Metrics include active/accepted/rejected connections, request/frame/error counters, member/workspace/leader counts, and catalog compatibility. `/metrics` is intentionally not Prometheus text format.

## WSS deployment

The relay itself serves HTTP/WebSocket, not TLS. Keep it on loopback or a private container network and terminate TLS in a reverse proxy. Remote runtimes should use `wss://`.

A minimal Caddy example is included at [`../deploy/Caddyfile.cluster-relay`](../deploy/Caddyfile.cluster-relay):

```caddy
relay.example.com {
    reverse_proxy 127.0.0.1:37423
}
```

Caddy handles WebSocket upgrades automatically. Nginx/other proxies must preserve WebSocket upgrade headers and should use sensible idle timeouts above the cluster heartbeat interval.

## systemd deployment

Repository example: [`../deploy/cluster-relay.service`](../deploy/cluster-relay.service).

Create a token file:

```bash
sudo install -d -m 0750 /etc/chatgpt-mcp
openssl rand -hex 32 | sudo tee /etc/chatgpt-mcp/cluster-relay.token >/dev/null
sudo chmod 0600 /etc/chatgpt-mcp/cluster-relay.token
```

Install/adapt the example unit and reverse proxy, then point runtimes at `wss://relay.example.com/cluster`.

The sample unit uses systemd credentials so the token is exposed to the process as a service credential file rather than a command-line value.

## Docker Compose deployment

Repository examples:

- [`../deploy/cluster-relay.Dockerfile`](../deploy/cluster-relay.Dockerfile)
- [`../deploy/compose.cluster-relay.yml`](../deploy/compose.cluster-relay.yml)
- [`../deploy/Caddyfile.cluster-relay.docker`](../deploy/Caddyfile.cluster-relay.docker)

Create a local secret before starting the stack:

```bash
mkdir -p .secrets
openssl rand -hex 32 > .secrets/cluster-token
chmod 0600 .secrets/cluster-token
CLUSTER_RELAY_DOMAIN=relay.example.com docker compose -f deploy/compose.cluster-relay.yml up -d --build
```

The relay listens over plain HTTP only inside the Compose network; Caddy exposes TLS/WSS publicly.

## Tunnel leadership

If cluster members are configured with the same enabled OpenAI tunnel ID, the cluster coordinator allows only one member to run that tunnel at a time. Other members remain standby.

Leadership uses a renewable relay lease. Catalog compatibility is required before leadership is acquired. Losing relay connectivity immediately demotes the local tunnel. When the active runtime disappears, another compatible member can acquire the lease and start the tunnel.

The built-in relay backend is in-memory. Leadership epochs and membership state are therefore scoped to one relay process lifetime. After a relay restart all runtimes reconnect/re-advertise and a fresh election occurs.

## Relay restart and recovery

The supported recovery sequence is automatic:

1. relay disappears
2. runtimes mark cluster disconnected and pending RPCs fail closed
3. current tunnel leader stops its tunnel
4. runtimes retry relay connection with bounded exponential backoff + jitter
5. relay returns
6. runtimes reconnect and re-advertise current workspaces/catalog
7. catalog compatibility is recomputed
8. tunnel leadership is elected again

No runtime restart is required for a normal relay restart.

## Relay-token rotation

Because the relay token authenticates every runtime, rotate it as a coordinated maintenance operation when a shared OpenAI tunnel is enabled. Do not temporarily split members across two independent relays: each relay would have its own leadership state.

Safe sequence:

1. stop participating runtimes
2. replace the relay token/keyring secret or `--token-file` content
3. update `cluster.relay_token` on every stopped runtime
4. restart the relay
5. start the runtimes
6. verify `cgm cluster status` on at least two members

Admin live reload intentionally rolls back a token change when it cannot authenticate to the currently configured relay.

## Catalog compatibility

Every connected runtime advertises a hash of its active tool catalog. A multi-member cluster becomes incompatible when online runtimes advertise different hashes or a legacy runtime omits the hash.

Routing/discovery remains observable, but tunnel leadership is blocked while catalogs are incompatible. Upgrade/reconfigure runtimes until their enabled features/upstream-generated tool surface matches, then verify:

```bash
cgm cluster status
```

## Relay HA and backend scope

The relay server is implemented against a `RelayBackend` abstraction, so membership/lease state is no longer coupled to the WebSocket server implementation. The shipped backend is still `MemoryRelay` and one relay process is the supported deployment model.

There is currently no bundled Redis/etcd backend and no active-active relay HA claim. Use a process supervisor/restart policy plus runtime reconnect for availability. A future distributed backend can implement the same backend contract without changing runtime WebSocket protocol behavior.

## Verification in CI

Linux CI runs `scripts/cluster-e2e.mjs` against the built binary. It starts a real relay plus two real runtime processes with isolated config roots, verifies remote read/write routing, restarts the relay and waits for reconnect/re-advertisement, restarts a workspace owner, and verifies routing recovers.

Tunnel-leader failover itself is covered by WebSocket integration tests with a fake tunnel runtime so CI never requires a real OpenAI tunnel/API key.