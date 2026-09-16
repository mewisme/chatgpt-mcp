import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { ThemeProvider } from "@/components/theme-provider"
import { adminToken, type PublicConfig } from "@/lib/api"
import { SettingsPage } from "@/pages/settings"

const config: PublicConfig = {
  server: {
    enabled: true,
    port: 37421,
    expose: { mode: "none", interfaces: [] },
    allow_insecure_http: false,
  },
  admin: { enabled: true, port: 37422 },
  auth: {
    mcp_enabled: true,
    admin_enabled: true,
    mcp_token_configured: true,
    mcp_token_revealable: true,
    admin_token_configured: true,
  },
  permissions: { allow_dirs: [] },
  shell: { path: [] },
}

describe("SettingsPage Direct MCP HTTP token", () => {
  beforeEach(() => adminToken.set("test-admin-token"))
  afterEach(() => {
    adminToken.clear()
    vi.unstubAllGlobals()
  })

  it("reveals and copies the stored token without rotating it", async () => {
    const user = userEvent.setup()
    const token = "mcp_reusable_token_value_123456"
    let rotated = false
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = requestPath(input)
      if (path === "/api/config") return json(config)
      if (path === "/api/network/interfaces") return json([])
      if (path === "/api/tunnels") return json([])
      if (path === "/api/plugins" || path === "/api/plugins?scope=global") return json([])
      if (path === "/api/workspaces") return json([])
      if (path === "/api/auth/mcp-token" && (!init?.method || init.method === "GET")) {
        return json({ token, configured: true, revealable: true, enabled: true })
      }
      if (path === "/api/auth/mcp-token" && init?.method === "POST") {
        rotated = true
        return json({ token: "mcp_rotated", configured: true, revealable: true, enabled: true })
      }
      throw new Error(`Unhandled test request: ${path} ${init?.method ?? "GET"}`)
    }))

    render(<ThemeProvider><SettingsPage /></ThemeProvider>)
    await user.click(await screen.findByRole("tab", { name: "Authentication" }))
    expect(await screen.findByText("Direct MCP HTTP token")).toBeInTheDocument()
    expect(screen.getByText("http://127.0.0.1:37421/mcp")).toBeInTheDocument()
    expect(screen.getByText(/Reuse this token when adding this MCP server to ChatGPT/)).toBeInTheDocument()
    expect(screen.queryByText(token)).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Reveal" }))
    expect(await screen.findByText(token)).toBeInTheDocument()
    expect(rotated).toBe(false)
    expect(screen.getByRole("button", { name: "Copy" })).toBeEnabled()
  })

  it("shows rotate-to-reveal for a legacy hash-only token", async () => {
    const user = userEvent.setup()
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = requestPath(input)
      if (path === "/api/config") {
        return json({
          ...config,
          auth: { ...config.auth, mcp_token_revealable: false },
        })
      }
      if (path === "/api/network/interfaces") return json([])
      if (path === "/api/tunnels") return json([])
      if (path === "/api/plugins" || path === "/api/plugins?scope=global") return json([])
      if (path === "/api/workspaces") return json([])
      throw new Error(`Unhandled test request: ${path}`)
    }))
    render(<ThemeProvider><SettingsPage /></ThemeProvider>)
    await user.click(await screen.findByRole("tab", { name: "Authentication" }))
    expect(await screen.findByText("configured, rotate to reveal")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Reveal" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "Copy" })).toBeDisabled()
  })
})

function requestPath(input: RequestInfo | URL) {
  const raw = input instanceof Request ? input.url : String(input)
  const url = new URL(raw, "http://localhost")
  return `${url.pathname}${url.search}`
}
function json(value: unknown) {
  return new Response(JSON.stringify(value), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}
