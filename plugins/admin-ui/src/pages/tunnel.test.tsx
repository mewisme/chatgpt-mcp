import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  adminApi,
  type LocalTunnel,
  type ManagedTunnel,
  type TunnelAdminProfile,
} from "@/lib/api"
import { TunnelPage } from "@/pages/tunnel"

const localTunnels: LocalTunnel[] = [
  {
    id: "tunnel_a",
    enabled: true,
    runtime_key_configured: true,
    admin_profile_id: "work",
    organization_id: "org_work",
    status: {
      provider: "openai",
      enabled: true,
      running: true,
      ready: true,
      restarting: false,
      id: "tunnel_a",
      metadata: {
        id: "tunnel_a",
        name: "Work tunnel",
        description: "Primary workspace",
        fetched_at: "2026-09-16T00:00:00Z",
      },
    },
  },
  {
    id: "tunnel_b",
    enabled: false,
    runtime_key_configured: true,
    status: {
      provider: "openai",
      enabled: false,
      running: false,
      ready: false,
      restarting: false,
      id: "tunnel_b",
    },
  },
]

const adminProfiles: TunnelAdminProfile[] = [
  {
    id: "work",
    key_configured: true,
    organization_id: "org_work",
    read_access: true,
    manage_access: true,
  },
  {
    id: "personal",
    key_configured: true,
    workspace_id: "ws_personal",
    read_access: true,
    manage_access: false,
  },
]

const managedTunnels: ManagedTunnel[] = [
  {
    metadata: {
      id: "tunnel_shared",
      name: "Shared tunnel",
      description: "Visible through both profiles",
      fetched_at: "2026-09-16T00:00:00Z",
    },
    admin_profiles: ["work", "personal"],
  },
  {
    metadata: {
      id: "tunnel_remote",
      name: "Remote tunnel",
      description: "Work tunnel",
      fetched_at: "2026-09-16T00:00:00Z",
    },
    admin_profiles: ["work"],
  },
]

function localWith(id: string, patch: Partial<LocalTunnel> = {}) {
  const current = localTunnels.find((item) => item.id === id)!
  return { ...current, ...patch }
}

describe("TunnelPage", () => {
  beforeEach(() => {
    vi.spyOn(adminApi, "localTunnels").mockResolvedValue(localTunnels)
    vi.spyOn(adminApi, "tunnelAdminProfiles").mockResolvedValue(adminProfiles)
    vi.spyOn(adminApi, "managedTunnelCollection").mockResolvedValue(
      managedTunnels
    )
    vi.spyOn(adminApi, "enableLocalTunnel").mockImplementation(async (id) =>
      localWith(id, { enabled: true })
    )
    vi.spyOn(adminApi, "disableLocalTunnel").mockImplementation(async (id) =>
      localWith(id, { enabled: false })
    )
    vi.spyOn(adminApi, "startLocalTunnel").mockImplementation(async (id) =>
      localWith(id, { status: { ...localWith(id).status, running: true } })
    )
    vi.spyOn(adminApi, "stopLocalTunnel").mockImplementation(async (id) =>
      localWith(id, {
        status: { ...localWith(id).status, running: false, ready: false },
      })
    )
    vi.spyOn(adminApi, "detachLocalTunnel").mockResolvedValue(undefined)
    vi.spyOn(adminApi, "addTunnelAdminProfile").mockImplementation(
      async (request) => ({
        id: request.id ?? "new",
        key_configured: true,
        organization_id: request.organization_id,
        workspace_id: request.workspace_id,
        tenant_id: request.tenant_id,
        read_access: false,
        manage_access: false,
      })
    )
    vi.spyOn(adminApi, "verifyTunnelAdminProfile").mockImplementation(
      async (id) => adminProfiles.find((item) => item.id === id)!
    )
    vi.spyOn(adminApi, "removeTunnelAdminProfile").mockResolvedValue(undefined)
    vi.spyOn(adminApi, "createManagedTunnelByProfile").mockResolvedValue(
      managedTunnels[1]
    )
    vi.spyOn(adminApi, "updateManagedTunnelByProfile").mockResolvedValue(
      managedTunnels[1]
    )
    vi.spyOn(adminApi, "deleteManagedTunnelByProfile").mockResolvedValue(
      managedTunnels[1]
    )
    vi.spyOn(adminApi, "attachManagedTunnel").mockResolvedValue({
      ...localTunnels[0],
      id: "tunnel_remote",
      admin_profile_id: "work",
      status: { ...localTunnels[0].status, id: "tunnel_remote" },
    })
    vi.spyOn(adminApi, "attachLocalTunnel").mockImplementation(
      async (request) => ({
        id: request.id,
        enabled: request.enabled,
        runtime_key_configured: Boolean(request.api_key),
        admin_profile_id: request.admin_profile_id,
        organization_id: request.organization_id,
        control_plane_base_url: request.control_plane_base_url,
        status: {
          provider: "openai",
          enabled: request.enabled,
          running: false,
          ready: false,
          restarting: false,
          id: request.id,
        },
      })
    )
    vi.spyOn(adminApi, "updateLocalTunnel").mockImplementation(
      async (id, request) =>
        localWith(id, {
          enabled: request.enabled,
          admin_profile_id: request.admin_profile_id,
          organization_id: request.organization_id,
          control_plane_base_url: request.control_plane_base_url,
        })
    )
    vi.spyOn(adminApi, "updateTunnelAdminProfile").mockImplementation(
      async (id, request) => ({
        ...adminProfiles.find((item) => item.id === id)!,
        organization_id: request.organization_id,
        workspace_id: request.workspace_id,
        tenant_id: request.tenant_id,
        control_plane_base_url: request.control_plane_base_url,
      })
    )
  })

  afterEach(() => vi.restoreAllMocks())

  it("renders multiple local instances and controls each tunnel by id", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)

    expect(await screen.findByText("Work tunnel")).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Stop tunnel_a" })
    ).toBeInTheDocument()
    expect(screen.getByText("tunnel_b")).toBeInTheDocument()
    expect(screen.getByText("Ready")).toBeInTheDocument()
    expect(screen.getByText("Disabled")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Stop tunnel_a" }))
    await waitFor(() =>
      expect(adminApi.stopLocalTunnel).toHaveBeenCalledWith("tunnel_a")
    )

    await user.click(screen.getByRole("button", { name: "Detach tunnel_b" }))
    await waitFor(() =>
      expect(adminApi.detachLocalTunnel).toHaveBeenCalledWith("tunnel_b")
    )
    expect(screen.queryByText("tunnel_b")).not.toBeInTheDocument()
  })

  it("shows multiple admin profiles and scopes verification/removal to a profile", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(await screen.findByRole("tab", { name: "Admin profiles" }))

    expect(screen.getByText("work")).toBeInTheDocument()
    expect(screen.getByText("personal")).toBeInTheDocument()
    expect(screen.getByText("organization:org_work")).toBeInTheDocument()
    expect(screen.getByText("workspace:ws_personal")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Verify work" }))
    await waitFor(() =>
      expect(adminApi.verifyTunnelAdminProfile).toHaveBeenCalledWith("work")
    )
    await user.click(screen.getByRole("button", { name: "Remove personal" }))
    await waitFor(() =>
      expect(adminApi.removeTunnelAdminProfile).toHaveBeenCalledWith("personal")
    )
  })

  it("attaches a managed tunnel without replacing existing local tunnels", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(
      await screen.findByRole("tab", { name: "Managed tunnels" })
    )

    expect(await screen.findByText("Shared tunnel")).toBeInTheDocument()
    expect(screen.getByText("Remote tunnel")).toBeInTheDocument()
    expect(screen.getAllByText("work").length).toBeGreaterThan(0)
    const attachButtons = screen.getAllByRole("button", { name: "Attach" })
    await user.click(attachButtons[1])
    expect(screen.getByText("Attach managed tunnel?")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Attach tunnel" }))

    await waitFor(() =>
      expect(adminApi.attachManagedTunnel).toHaveBeenCalledWith(
        "tunnel_remote",
        {
          auto_generate_runtime_key: true,
          project_id: undefined,
          enabled: true,
        },
        "work"
      )
    )
    expect(
      await screen.findByText("Managed tunnel tunnel_remote attached.")
    ).toBeInTheDocument()
    expect(localTunnels).toHaveLength(2)
  })

  it("keeps read-only admin profiles useful for discovery and attach while hiding mutations", async () => {
    vi.mocked(adminApi.tunnelAdminProfiles).mockResolvedValue([
      adminProfiles[1],
    ])
    vi.mocked(adminApi.managedTunnelCollection).mockResolvedValue([
      { ...managedTunnels[0], admin_profiles: ["personal"] },
    ])
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(
      await screen.findByRole("tab", { name: "Managed tunnels" })
    )

    expect(await screen.findByText("Shared tunnel")).toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Create tunnel" })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: /^Edit$/ })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole("button", { name: "Delete" })
    ).not.toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Attach" })).toBeInTheDocument()
  })

  it("attaches a local tunnel from the local panel", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(
      await screen.findByRole("button", { name: "Add local tunnel" })
    )
    await user.type(screen.getByLabelText("Tunnel ID"), "tunnel_c")
    await user.type(screen.getByLabelText("Runtime API key"), "sk-local")
    await user.click(screen.getByRole("button", { name: "Add local tunnel" }))
    await waitFor(() =>
      expect(adminApi.attachLocalTunnel).toHaveBeenCalledWith({
        id: "tunnel_c",
        enabled: true,
        api_key: "sk-local",
      })
    )
    expect(await screen.findByText("tunnel_c")).toBeInTheDocument()
    expect(screen.getByText("Work tunnel")).toBeInTheDocument()
    expect(screen.getByText("tunnel_b")).toBeInTheDocument()
  })

  it("updates a local tunnel without replacing a blank runtime key", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(
      await screen.findByRole("button", { name: "Edit tunnel_a" })
    )
    const org = screen.getByLabelText("Organization ID")
    await user.clear(org)
    await user.type(org, "org_new")
    await user.click(screen.getByRole("button", { name: "Save local tunnel" }))
    await waitFor(() =>
      expect(adminApi.updateLocalTunnel).toHaveBeenCalledWith("tunnel_a", {
        id: "tunnel_a",
        enabled: true,
        admin_profile_id: "work",
        organization_id: "org_new",
      })
    )
    expect(
      vi.mocked(adminApi.updateLocalTunnel).mock.calls[0][1].api_key
    ).toBeUndefined()
  })

  it("updates an admin profile without replacing a blank key", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(await screen.findByRole("tab", { name: "Admin profiles" }))
    await user.click(screen.getByRole("button", { name: "Edit work" }))
    const scope = screen.getByLabelText("Scope ID")
    await user.clear(scope)
    await user.type(scope, "org_new")
    await user.click(screen.getByRole("button", { name: "Save admin profile" }))
    await waitFor(() =>
      expect(adminApi.updateTunnelAdminProfile).toHaveBeenCalledWith("work", {
        id: "work",
        organization_id: "org_new",
      })
    )
    expect(
      vi.mocked(adminApi.updateTunnelAdminProfile).mock.calls[0][1].admin_key
    ).toBeUndefined()
  })

  it("disambiguates duplicate tunnel names with short ids", async () => {
    const dupA = "tunnel_aaaaaaaaaaaaaaaaaaaaaaaac3330bcd"
    const dupB = "tunnel_bbbbbbbbbbbbbbbbbbbbbbb3ce094ac"
    vi.mocked(adminApi.localTunnels).mockResolvedValue([
      {
        ...localTunnels[0],
        id: dupA,
        status: {
          ...localTunnels[0].status,
          id: dupA,
          metadata: {
            ...localTunnels[0].status.metadata!,
            id: dupA,
            name: "Production",
          },
        },
      },
      {
        ...localTunnels[0],
        id: dupB,
        admin_profile_id: undefined,
        status: {
          ...localTunnels[0].status,
          id: dupB,
          running: false,
          ready: false,
          metadata: {
            ...localTunnels[0].status.metadata!,
            id: dupB,
            name: "Production",
          },
        },
      },
    ])
    render(<TunnelPage />)
    expect(await screen.findByText("Production · c3330bcd")).toBeInTheDocument()
    expect(screen.getByText("Production · 3ce094ac")).toBeInTheDocument()
  })

  it("requires an explicit admin profile when a managed tunnel is visible to more than one profile", async () => {
    const user = userEvent.setup()
    render(<TunnelPage />)
    await user.click(
      await screen.findByRole("tab", { name: "Managed tunnels" })
    )
    expect(await screen.findByText("Shared tunnel")).toBeInTheDocument()
    const attachButtons = screen.getAllByRole("button", { name: "Attach" })
    await user.click(attachButtons[0])
    expect(screen.getByRole("button", { name: "Attach tunnel" })).toBeDisabled()
  })
})
