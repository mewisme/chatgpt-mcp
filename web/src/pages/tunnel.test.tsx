import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { TunnelPage } from "@/pages/tunnel"
import { adminApi, type TunnelStatus } from "@/lib/api"

const tunnelStatus: TunnelStatus = {
  provider: "openai",
  enabled: true,
  running: true,
  ready: true,
  restarting: false,
  id: "tunnel_one",
  control_plane_base_url: "https://api.openai.com",
  organization_id: "org_runtime",
  started_at: "2026-09-05T00:00:00Z",
  admin_key_configured: true,
  admin_scope: { workspace_id: "ws_admin" },
  metadata: {
    id: "tunnel_one",
    name: "Primary tunnel",
    description: "Production ChatGPT bridge",
    creator: "user_test",
    organization_ids: ["org_one"],
    workspace_ids: ["ws_admin"],
    tenant_ids: ["tenant_one"],
    request_id: "req_meta",
    fetched_at: "2026-09-05T00:01:00Z",
  },
}

describe("TunnelPage", () => {
  beforeEach(() => {
    vi.spyOn(adminApi, "tunnelConfig").mockResolvedValue({ enabled: true, id: "tunnel_one", runtime_key_configured: true, admin_key_configured: true })
    vi.spyOn(adminApi, "tunnel").mockResolvedValue(tunnelStatus)
    vi.spyOn(adminApi, "tunnelAdminKey").mockResolvedValue({ configured: true, scope: { workspace_id: "ws_admin" }, tunnels: 2 })
    vi.spyOn(adminApi, "removeTunnelAdminKey").mockResolvedValue({ configured: false, scope: {} })
    vi.spyOn(adminApi, "startTunnel").mockResolvedValue(tunnelStatus)
    vi.spyOn(adminApi, "stopTunnel").mockResolvedValue({ ...tunnelStatus, running: false, ready: false })
  })

  afterEach(() => vi.restoreAllMocks())

  it("separates runtime, administration, and metadata and confirms admin-key removal", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)

    expect(await screen.findByText("Primary tunnel")).toBeInTheDocument()
    expect(screen.getByText("Runtime connectivity")).toBeInTheDocument()
    expect(screen.getByText("Ready")).toBeInTheDocument()

    await user.click(screen.getByRole("tab", { name: "Administration" }))
    expect(screen.getByText("Tunnel administration")).toBeInTheDocument()
    expect(screen.getByText("2 accessible tunnels")).toBeInTheDocument()
    expect(screen.getByText("Secret file store")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Remove" }))
    expect(screen.getByText("Remove tunnel admin key?")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Remove admin key" }))
    await waitFor(() => expect(adminApi.removeTunnelAdminKey).toHaveBeenCalledOnce())
    expect(await screen.findByText("Admin key removed.")).toBeInTheDocument()

    await user.click(screen.getByRole("tab", { name: "Metadata" }))
    expect(screen.getByText("Connection identity")).toBeInTheDocument()
    expect(screen.getByText("Tunnel scope")).toBeInTheDocument()
    expect(screen.getByText("tenant_one")).toBeInTheDocument()
    expect(screen.getByText("req_meta")).toBeInTheDocument()
  })
})