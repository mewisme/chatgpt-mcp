import { render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom"
import { ThemeProvider } from "@/components/theme-provider"
import { TooltipProvider } from "@/components/ui/tooltip"
import { WorkspaceExecutionDetail, WorkspaceExecutions } from "@/components/workspace-executions"
import { adminToken, type ExecutionInfo, type ExecutionSnapshot } from "@/lib/api"

describe("WorkspaceExecutions", () => {
  afterEach(() => { adminToken.clear(); vi.unstubAllGlobals() })

  it("routes to execution detail, hydrates snapshot, and streams stdout, stderr, and completion", async () => {
    const user = userEvent.setup()
    const execution: ExecutionInfo = { id: "exec_1", workspace_id: "ws_test", tool: "run_command", command: "pnpm test", cwd: "/projects/test", source: "mcp", started_at: new Date().toISOString(), status: "running" }
    const snapshot: ExecutionSnapshot = { execution, stdout: "start\n", stderr: "", latest_sequence: 1 }
    adminToken.set("test-admin-token")
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = requestPath(input)
      if (path === "/api/workspaces/ws_test/executions?limit=50") return json([execution])
      if (path === "/api/workspaces/ws_test/executions/exec_1") return json(snapshot)
      if (path === "/api/workspaces/ws_test/executions/exec_1/stream") return executionStream(snapshot)
      throw new Error(`Unhandled test request: ${path}`)
    }))

    render(<MemoryRouter initialEntries={["/workspaces/ws_test/activity"]}><ThemeProvider><TooltipProvider><LocationProbe /><Routes><Route path="/workspaces/:workspaceID/activity" element={<WorkspaceExecutions workspaceID="ws_test" />} /><Route path="/workspaces/:workspaceID/activity/:executionID" element={<WorkspaceExecutionDetail workspaceID="ws_test" executionID="exec_1" />} /></Routes></TooltipProvider></ThemeProvider></MemoryRouter>)
    await user.click(await screen.findByText("pnpm test"))
    await waitFor(() => expect(screen.getByTestId("location")).toHaveTextContent("/workspaces/ws_test/activity/exec_1"))
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole("code").textContent).toContain("start\nnext\n"))
    expect(await screen.findByText("success")).toBeInTheDocument()
    expect(screen.getByText("exit 0")).toBeInTheDocument()
    await user.click(screen.getByRole("tab", { name: "stderr" }))
    expect(screen.getByRole("code").textContent).toContain("warn\n")
  })
})

function LocationProbe() { const location = useLocation(); return <span className="sr-only" data-testid="location">{location.pathname}</span> }

function executionStream(snapshot: ExecutionSnapshot) {
  const encoder = new TextEncoder()
  const packets = [
    `event: ready\ndata: ${JSON.stringify(snapshot)}\n\n`,
    `id: 2\nevent: output\ndata: ${JSON.stringify({ sequence: 2, type: "output", execution_id: "exec_1", stream: "stdout", data: "next\n", timestamp: new Date().toISOString() })}\n\n`,
    `id: 3\nevent: output\ndata: ${JSON.stringify({ sequence: 3, type: "output", execution_id: "exec_1", stream: "stderr", data: "warn\n", timestamp: new Date().toISOString() })}\n\n`,
    `id: 4\nevent: completed\ndata: ${JSON.stringify({ sequence: 4, type: "completed", execution_id: "exec_1", status: "success", exit_code: 0, timestamp: new Date().toISOString() })}\n\n`,
  ]
  const body = new ReadableStream({ start(controller) { for (const packet of packets) controller.enqueue(encoder.encode(packet)); controller.close() } })
  return new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } })
}

function requestPath(input: RequestInfo | URL) { const raw = input instanceof Request ? input.url : String(input); const url = new URL(raw, "http://localhost"); return `${url.pathname}${url.search}` }
function json(value: unknown) { return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } }) }
