import { describe, expect, it } from "vitest"
import { adminRouteFromPath } from "@/lib/admin-route"

describe("adminRouteFromPath", () => {
  it("matches static routes before workspace params", () => {
    expect(adminRouteFromPath("/workspaces/requests")).toMatchObject({ id: "requests", navID: "requests", path: "/workspaces/requests" })
  })

  it("matches workspace and child routes with ids", () => {
    expect(adminRouteFromPath("/workspaces/ws_abc")).toMatchObject({ id: "workspace", navID: "workspaces", workspaceID: "ws_abc" })
    expect(adminRouteFromPath("/workspaces/ws_abc/context")).toMatchObject({ id: "workspace-context", workspaceID: "ws_abc" })
    expect(adminRouteFromPath("/workspaces/ws_abc/requests")).toMatchObject({ id: "workspace-requests", workspaceID: "ws_abc" })
    expect(adminRouteFromPath("/workspaces/ws_abc/activity")).toMatchObject({ id: "workspace-activity", workspaceID: "ws_abc" })
  })

  it("matches global instructions and normalizes unknown routes", () => {
    expect(adminRouteFromPath("/workspaces/global")).toMatchObject({ id: "workspace-global", navID: "workspaces" })
    expect(adminRouteFromPath("/missing")).toMatchObject({ id: "overview", path: "/overview" })
  })
})