import { mkdtemp, rm } from "node:fs/promises"
import { createServer } from "node:net"
import { tmpdir } from "node:os"
import path from "node:path"
import process from "node:process"
import { spawn, spawnSync } from "node:child_process"

process.noDeprecation = true

const protocolVersion = "2026-07-28"
const input = process.argv[2]
if (!input) fail("usage: node scripts/smoke-release.mjs <binary>")

const binary = path.resolve(input)
const home = await mkdtemp(path.join(tmpdir(), "chatgpt-mcp-release-smoke-"))
const env = { ...process.env, HOME: home, USERPROFILE: home }
const serverPort = await freePort()
const adminPort = await freePort()
let child = null

try {
  run(["--help"])
  run(["serve", "--help"])
  run(["version"])
  run(["init"], { quiet: true })
  run(["config", "set", "server.expose", "false"])
  run(["config", "set", "server.port", String(serverPort)])
  run(["config", "set", "admin.port", String(adminPort)])
  run(["config", "set", "auth.mcp_enabled", "false"])
  run(["config", "validate"])
  run(["status"])

  child = spawn(binary, ["serve"], { env, stdio: ["ignore", "pipe", "pipe"], windowsHide: true })
  let stdout = ""
  let stderr = ""
  child.stdout.on("data", (chunk) => { stdout += chunk.toString() })
  child.stderr.on("data", (chunk) => { stderr += chunk.toString() })

  await waitForHealth(`http://127.0.0.1:${serverPort}/health`, child, () => `${stdout}\n${stderr}`)
  await verifyMCP(serverPort)
  run(["status"])
  await stopChild(child)
  child = null

  run(["uninit"], { quiet: true })
  console.log("[OK] release smoke: init -> config -> status -> serve -> health -> MCP discover/list/conformance -> stop -> uninit")
} finally {
  if (child) await stopChild(child).catch(() => undefined)
  await rm(home, { recursive: true, force: true })
}

function run(args, { quiet = false } = {}) {
  const result = spawnSync(binary, args, { env, encoding: "utf8", windowsHide: true })
  if (result.error) fail(`${args.join(" ")}: ${result.error.message}`)
  if (result.status !== 0) {
    const output = [result.stdout, result.stderr].filter(Boolean).join("\n").trim()
    fail(`${args.join(" ")} failed with exit code ${result.status}${output ? `\n${output}` : ""}`)
  }
  if (!quiet) {
    const output = [result.stdout, result.stderr].filter(Boolean).join("").trim()
    if (output) console.log(output)
  }
}

async function verifyMCP(port) {
  const discover = await mcpRequest(port, "server/discover", {}, 1)
  assertStatus(discover.response, 200, "server/discover")
  if (discover.response.headers.get("mcp-session-id")) fail("modern MCP response unexpectedly returned Mcp-Session-Id")
  if (!Array.isArray(discover.body?.result?.supportedVersions) || !discover.body.result.supportedVersions.includes(protocolVersion)) {
    fail(`server/discover did not advertise ${protocolVersion}: ${JSON.stringify(discover.body)}`)
  }

  const tools = await mcpRequest(port, "tools/list", {}, 2)
  assertStatus(tools.response, 200, "tools/list")
  if (!Array.isArray(tools.body?.result?.tools) || tools.body.result.tools.length === 0) {
    fail(`tools/list returned no tools: ${JSON.stringify(tools.body)}`)
  }
  if (!Number.isFinite(tools.body.result.ttlMs) || typeof tools.body.result.cacheScope !== "string") {
    fail(`tools/list cache hints are missing: ${JSON.stringify(tools.body)}`)
  }

  const legacy = await mcpRequest(port, "initialize", {}, 3)
  assertStatus(legacy.response, 404, "initialize")
  if (legacy.body?.error?.code !== -32601) {
    fail(`initialize error code = ${legacy.body?.error?.code}, want -32601`)
  }
}

async function mcpRequest(port, method, params, id) {
  const payload = {
    jsonrpc: "2.0",
    id,
    method,
    params: {
      ...params,
      _meta: {
        "io.modelcontextprotocol/protocolVersion": protocolVersion,
        "io.modelcontextprotocol/clientCapabilities": {},
        "io.modelcontextprotocol/clientInfo": { name: "release-smoke", version: "1.0.0" },
      },
    },
  }
  const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "MCP-Protocol-Version": protocolVersion,
      "Mcp-Method": method,
    },
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(5000),
  })
  let body
  try {
    body = await response.json()
  } catch (error) {
    fail(`${method} returned invalid JSON: ${error instanceof Error ? error.message : String(error)}`)
  }
  return { response, body }
}

function assertStatus(response, expected, label) {
  if (response.status !== expected) fail(`${label} status = ${response.status}, want ${expected}`)
}

async function freePort() {
  return await new Promise((resolve, reject) => {
    const server = createServer()
    server.unref()
    server.once("error", reject)
    server.listen(0, "127.0.0.1", () => {
      const address = server.address()
      const port = typeof address === "object" && address ? address.port : 0
      server.close((error) => error ? reject(error) : resolve(port))
    })
  })
}

async function waitForHealth(url, server, output) {
  const deadline = Date.now() + 15000
  while (Date.now() < deadline) {
    if (server.exitCode !== null) fail(`serve exited before health check with code ${server.exitCode}\n${output().trim()}`)
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1000) })
      if (response.ok) {
        const body = await response.json()
        if (body?.ok === true) return
      }
    } catch {}
    await sleep(100)
  }
  fail(`health check timed out: ${url}\n${output().trim()}`)
}

async function stopChild(server) {
  if (server.exitCode !== null) return
  server.kill("SIGTERM")
  const exited = await Promise.race([
    new Promise((resolve) => server.once("exit", () => resolve(true))),
    sleep(5000).then(() => false),
  ])
  if (exited) return
  server.kill("SIGKILL")
  await Promise.race([
    new Promise((resolve) => server.once("exit", resolve)),
    sleep(2000),
  ])
}

function sleep(ms) { return new Promise((resolve) => setTimeout(resolve, ms)) }
function fail(message) { console.error(`[FAIL] ${message}`); process.exit(1) }
