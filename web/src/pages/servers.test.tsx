import { render, screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"
import { TooltipProvider } from "@/components/ui/tooltip"
import { ServersPage } from "@/pages/servers"

const server = { id: "local", name: "Local server", transport: "http", enabled: true, url: "http://127.0.0.1:3000/mcp", auth: { type: "none" }, expose: "all" }

describe("servers page", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("keeps the page list-first and opens health/tools in server detail", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(input instanceof Request ? input.url : String(input), "http://localhost")
      if (url.pathname === "/api/upstream") return json([server])
      if (url.pathname === "/api/upstream/local/status") return json({ ...server, auth: "none", health: "connected", connected: true, tool_count: 1, proxied_tools: ["local__read_file"] })
      if (url.pathname === "/api/upstream/local/tools") return json({ server_id: "local", tools: [{ name: "read_file", description: "Read one file" }], proxied_tools: ["local__read_file"] })
      throw new Error(`Unhandled request: ${url.pathname}${url.search}`)
    }))
    const user = userEvent.setup()
    render(<TooltipProvider><ServersPage /></TooltipProvider>)
    expect(await screen.findByText("Local server")).toBeInTheDocument()
    expect(screen.queryByRole("tab", { name: "Overview" })).not.toBeInTheDocument()
    await user.click(screen.getByText("Local server"))
    expect(await screen.findByRole("tab", { name: "Overview" })).toBeInTheDocument()
    await user.click(screen.getByRole("tab", { name: "Tools" }))
    expect(await screen.findByText("read_file")).toBeInTheDocument()
    expect(screen.getByText("Proxied")).toBeInTheDocument()
  })
})

function json(value: unknown) { return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } }) }
