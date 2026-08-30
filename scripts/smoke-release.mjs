import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
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
const configDir = path.join(home, "config")
const defaultConfigDir = path.join(home, ".config", "chatgpt-mcp")
const defaultSentinel = path.join(defaultConfigDir, "release-smoke-sentinel")
const allowedDir = path.join(home, "allowed")
const globalArgs = ["--config-dir", configDir]
const serverPort = await freePort()
const adminPort = await freePort()
let child = null

try {
  await mkdir(defaultConfigDir, { recursive: true })
  await mkdir(allowedDir, { recursive: true })
  await writeFile(defaultSentinel, "keep\n")
  run(["--help"])
  run(["serve", "--help"])
  run(["auth", "mcp", "--help"])
  run(["workspace", "access", "--help"])
  run(["version"])
  run(["init"], { quiet: true })
  run(["config", "set", "permissions.allow_dirs", allowedDir])
  run(["config", "get", "permissions.allow_dirs"])
  run(["config", "verify"])
  run(["config", "convert", "yaml"])
  run(["config", "verify"])
  run(["config", "transform", "toml"])
  run(["config", "validate"])
  run(["config", "set", "server.expose", "0.0.0.0"])
  run(["config", "verify"])
  run(["config", "set", "server.expose", "false"])
  run(["config", "set", "server.port", String(serverPort)])
  run(["config", "set", "admin.port", String(adminPort)])
  run(["config", "set", "auth.mcp_enabled", "false"])
  run(["config", "set", "auth.admin_enabled", "false"])
  run(["config", "set", "features.ponytail.enabled", "false"])
  run(["config", "set", "features.caveman.enabled", "false"])
  run(["config", "set", "features.caveman.enabled", "true"])
  run(["config", "verify"])
  run(["status"])

  child = spawn(binary, [...globalArgs, "serve"], { env, stdio: ["ignore", "pipe", "pipe"], windowsHide: true })
  let stdout = ""
  let stderr = ""
  child.stdout.on("data", (chunk) => { stdout += chunk.toString() })
  child.stderr.on("data", (chunk) => { stderr += chunk.toString() })

  await waitForHealth(`http://127.0.0.1:${serverPort}/health`, child, () => `${stdout}\n${stderr}`)
  await waitForHealth(`http://127.0.0.1:${adminPort}/api/health`, child, () => `${stdout}\n${stderr}`)
  await verifyActivitySSE(adminPort)
  await verifyMCP(serverPort)
  run(["status"])
  await stopChild(child)
  child = null

  run(["uninit"], { quiet: true })
  if (await readFile(defaultSentinel, "utf8") !== "keep\n") fail("isolated commands modified the default config root")
  console.log("[OK] release smoke: init -> verify -> convert/transform -> config -> status -> serve -> health -> Activity SSE ready -> MCP discover/list/conformance -> stop -> uninit")
} finally {
  if (child) await stopChild(child).catch(() => undefined)
  await rm(home, { recursive: true, force: true })
}

function run(args, { quiet = false } = {}) {
  const result = spawnSync(binary, [...globalArgs, ...args], { env, encoding: "utf8", windowsHide: true })
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
  const toolNames = new Set(tools.body.result.tools.map((tool) => tool?.name))
  if (toolNames.has("ponytail_turn")) fail(`disabled ponytail_turn remained in tools/list: ${JSON.stringify(tools.body)}`)
  if (!toolNames.has("caveman_turn")) fail(`enabled caveman_turn missing from tools/list: ${JSON.stringify(tools.body)}`)
  if (!Number.isFinite(tools.body.result.ttlMs) || typeof tools.body.result.cacheScope !== "string") {
    fail(`tools/list cache hints are missing: ${JSON.stringify(tools.body)}`)
  }

  const legacy = await mcpRequest(port, "initialize", {}, 3)
  assertStatus(legacy.response, 404, "initialize")
  if (legacy.body?.error?.code !== -32601) {
    fail(`initialize error code = ${legacy.body?.error?.code}, want -32601`)
  }
}

async function verifyActivitySSE(port) {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 5000)
  try {
    const response = await fetch(`http://127.0.0.1:${port}/api/activity/stream?history=0`, { signal: controller.signal })
    assertStatus(response, 200, "activity SSE")
    if (!response.body || !response.headers.get("content-type")?.includes("text/event-stream")) fail("activity SSE response is not an event stream")
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ""
    while (!buffer.includes("\n\n")) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
    }
    await reader.cancel()
    if (!buffer.includes("event: ready\n") || !buffer.includes('"latest_sequence":')) fail(`activity SSE did not emit ready control event: ${JSON.stringify(buffer)}`)
  } finally {
    clearTimeout(timeout)
    controller.abort()
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
