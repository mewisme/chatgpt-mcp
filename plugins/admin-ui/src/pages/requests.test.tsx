import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { adminToken, type ApprovalRequest } from "@/lib/api"
import { RequestsPage } from "@/pages/requests"

describe("RequestsPage", () => {
  beforeEach(() => adminToken.set("test-admin-token"))
  afterEach(() => {
    adminToken.clear()
    vi.unstubAllGlobals()
  })

  it("shows approval history, filters it, and resolves a pending request", async () => {
    const user = userEvent.setup()
    const pending = request("req_pending", "pending", "cgm update")
    const consumed = request("req_consumed", "consumed", "cgm install")
    let items = [pending, consumed]
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = requestPath(input)
        if (path === "/api/requests?status=&workspace_id=ws_test")
          return json(items)
        if (path === "/api/requests/stream?workspace_id=ws_test")
          return approvalStream()
        if (path === "/api/requests/req_pending")
          return json(items.find((item) => item.id === pending.id))
        if (
          path === "/api/requests/req_pending/approve" &&
          init?.method === "POST"
        ) {
          const approved = {
            ...pending,
            status: "approved",
            resolved_at: new Date().toISOString(),
            resolved_by: "admin",
          }
          items = [approved, consumed]
          return json(approved)
        }
        throw new Error(`Unhandled test request: ${path}`)
      })
    )

    renderPage()
    expect(await screen.findByText("Allow cgm update")).toBeInTheDocument()
    expect(screen.getByText("Allow cgm install")).toBeInTheDocument()
    expect(screen.getByText("1 pending")).toBeInTheDocument()

    const search = screen.getByPlaceholderText(
      "Search request, tool, source, tunnel..."
    )
    await user.type(search, "consumed")
    expect(screen.queryByText("Allow cgm update")).not.toBeInTheDocument()
    expect(screen.getByText("Allow cgm install")).toBeInTheDocument()
    await user.clear(search)

    await user.click(screen.getByText("Allow cgm update"))
    expect(
      await screen.findByText(/Control approval request · req_pending/)
    ).toBeInTheDocument()
    await user.click(screen.getByRole("tab", { name: "Details" }))
    expect(screen.getByRole("code").textContent).toContain(
      '"command": "cgm update"'
    )
    await user.click(screen.getByRole("button", { name: /Approve/ }))
    await waitFor(() =>
      expect(screen.getAllByText("approved").length).toBeGreaterThan(0)
    )
    await waitFor(() =>
      expect(screen.getByText("0 pending")).toBeInTheDocument()
    )
  })

  it("lists every workspace from the global route", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = requestPath(input)
        if (path === "/api/requests?status=") return json([])
        if (path === "/api/requests/stream") return approvalStream()
        throw new Error(`Unhandled test request: ${path}`)
      })
    )
    render(
      <ThemeProvider>
        <TooltipProvider>
          <RequestsPage />
        </TooltipProvider>
      </ThemeProvider>
    )
    expect(await screen.findByText("Approval requests")).toBeInTheDocument()
    expect(
      screen.getByText("Review control approvals and resolved request history.")
    ).toBeInTheDocument()
  })

  it("approves with a reason and similar-command grant", async () => {
    const user = userEvent.setup()
    const pending = {
      ...request("req_similar", "pending", "git push origin main"),
      command: "git push origin main",
      similar_command_pattern: "git push **",
    }
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = requestPath(input)
        if (path === "/api/requests?status=&workspace_id=ws_test")
          return json([pending])
        if (path === "/api/requests/stream?workspace_id=ws_test")
          return approvalStream()
        if (path === "/api/requests/req_similar") return json(pending)
        if (
          path === "/api/requests/req_similar/approve" &&
          init?.method === "POST"
        ) {
          const body = JSON.parse(String(init.body || "{}"))
          if (body.reason !== "reviewed" || body.allow_similar !== true) {
            throw new Error(`unexpected approve body ${JSON.stringify(body)}`)
          }
          return json({
            ...pending,
            status: "approved",
            reason: "reviewed",
            runtime_session_grant: true,
          })
        }
        throw new Error(`Unhandled test request: ${path}`)
      })
    )
    renderPage()
    await user.click(await screen.findByText("Allow git push origin main"))
    await user.type(await screen.findByLabelText("Reason"), "reviewed")
    await user.click(
      screen.getByText("Allow similar commands for all MCP sessions (1h)")
    )
    await user.click(screen.getByRole("button", { name: /Approve/ }))
    await waitFor(() =>
      expect(screen.getAllByText("approved").length).toBeGreaterThan(0)
    )
  })

  it("labels two tunnels and matches search on name or id", async () => {
    const user = userEvent.setup()
    const alpha = {
      ...request("req_alpha", "pending", "cgm update"),
      tunnel_id: "tunnel_aaaaaaaaaaaaaaaa",
      tunnel_name: "Alpha",
      title: "Allow alpha",
    }
    const beta = {
      ...request("req_beta", "pending", "cgm install"),
      tunnel_id: "tunnel_bbbbbbbbbbbbbbbb",
      tunnel_name: "Beta",
      title: "Allow beta",
    }
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = requestPath(input)
        if (path === "/api/requests?status=&workspace_id=ws_test")
          return json([alpha, beta])
        if (path === "/api/requests/stream?workspace_id=ws_test")
          return approvalStream()
        throw new Error(`Unhandled test request: ${path}`)
      })
    )
    renderPage()
    expect(await screen.findByText("Allow alpha")).toBeInTheDocument()
    expect(screen.getAllByText(/ · Alpha/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/ · Beta/).length).toBeGreaterThan(0)
    expect(screen.getByText("All tunnels")).toBeInTheDocument()
    expect(
      screen.queryByText("tunnel_aaaaaaaaaaaaaaaa")
    ).not.toBeInTheDocument()
    await user.type(
      screen.getByPlaceholderText("Search request, tool, source, tunnel..."),
      "Beta"
    )
    expect(screen.queryByText("Allow alpha")).not.toBeInTheDocument()
    expect(screen.getByText("Allow beta")).toBeInTheDocument()
  })
})

function renderPage() {
  return render(
    <ThemeProvider>
      <TooltipProvider>
        <RequestsPage workspaceID="ws_test" />
      </TooltipProvider>
    </ThemeProvider>
  )
}

function request(id: string, status: string, command: string): ApprovalRequest {
  const now = Date.now()
  return {
    id,
    status,
    workspace_id: "ws_test",
    session_hash: "hash-session",
    source: "tunnel",
    target_tool: "run_command",
    arguments: { workspace_id: "ws_test", command },
    guard_code: "control_plane_mutation",
    guard_reason: "control-plane mutation denied",
    title: `Allow ${command}`,
    created_at: new Date(now).toISOString(),
    expires_at: new Date(now + 60_000).toISOString(),
  }
}

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
function approvalStream() {
  const encoder = new TextEncoder()
  const body = new ReadableStream({
    start(controller) {
      controller.enqueue(
        encoder.encode('event: ready\ndata: {"latest_sequence":0}\n\n')
      )
      controller.close()
    },
  })
  return new Response(body, {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  })
}
