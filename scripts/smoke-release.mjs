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
delete env.CHATGPT_MCP_TOOL_CONTEXT
const configDir = path.join(home, "config")
const defaultConfigDir = path.join(home, ".config", "chatgpt-mcp")
const defaultSentinel = path.join(defaultConfigDir, "release-smoke-sentinel")
const allowedDir = path.join(home, "allowed")
const globalArgs = ["--config-dir", configDir]
const serverPort = await freePort()
const adminPort = await freePort()
let child = null
let follower = null
let occupied = null

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
  run(["config", "set", "server.allow_insecure_http", "true"])
  run(["config", "set", "server.expose", "0.0.0.0"])
  run(["config", "verify"])
  run(["config", "set", "server.expose", "false"])
  run(["config", "set", "server.allow_insecure_http", "false"])
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
  await verifyMCP(serverPort, false)
  const foregroundStatus = run(["status"], { quiet: true })
  for (const expected of ["✓ chatgpt-mcp is running", "session     run_", "mode        foreground", "status      disabled"]) {
    if (!foregroundStatus.includes(expected)) fail(`foreground status missing ${JSON.stringify(expected)}:\n${foregroundStatus}`)
  }

  const servePID = child.pid
  const reloadedServerPort = await freePort()
  const reloadedAdminPort = await freePort()
  run(["config", "set", "server.port", String(reloadedServerPort)])
  run(["config", "set", "admin.port", String(reloadedAdminPort)])
  run(["config", "set", "features.ponytail.enabled", "true"])
  run(["config", "reload"])
  if (child.pid !== servePID || child.exitCode !== null) fail("config reload restarted or stopped the serve process")
  await waitForHealth(`http://127.0.0.1:${reloadedServerPort}/health`, child, () => `${stdout}\n${stderr}`)
  await waitForHealth(`http://127.0.0.1:${reloadedAdminPort}/api/health`, child, () => `${stdout}\n${stderr}`)
  await verifyMCP(reloadedServerPort, true)

  occupied = await occupyPort()
  run(["config", "set", "server.port", String(occupied.port)])
  runExpectFailure(["config", "reload"])
  if (child.pid !== servePID || child.exitCode !== null) fail("failed config reload stopped the serve process")
  await waitForHealth(`http://127.0.0.1:${reloadedServerPort}/health`, child, () => `${stdout}\n${stderr}`)
  run(["config", "set", "server.port", String(reloadedServerPort)])
  run(["config", "reload"])
  await closeServer(occupied.server)
  occupied = null

  await stopRuntimeChild(child)
  child = null
  runExpectFailure(["config", "reload"])

  const history = run(["logs", "--debug", "--event", "server.*", "--tail", "50"], { quiet: true })
  if (!history.includes("server.ready") && !history.includes("Server ready")) fail(`runtime history missing server readiness event:\n${history}`)
  if (!history.includes("── session run_")) fail(`runtime history missing session boundary:\n${history}`)
  if (!/^\d{2}:\d{2}:\d{2} /m.test(history)) fail(`runtime history missing replay timestamp:\n${history}`)
  const noTimeHistory = run(["logs", "--event", "server.*", "--tail", "10", "--no-time"], { quiet: true })
  if (/^\d{2}:\d{2}:\d{2} /m.test(noTimeHistory)) fail(`--no-time still rendered replay timestamp:\n${noTimeHistory}`)
  const sessionLifecycle = run(["logs", "--event", "runtime.session.*", "--tail", "10"], { quiet: true })
  if (!sessionLifecycle.includes("Runtime session ended")) fail(`runtime history missing session end marker:\n${sessionLifecycle}`)
  const logPath = run(["logs", "path"], { quiet: true })
  if (!logPath.includes(path.join(configDir, "logs", "runtime.jsonl"))) fail(`logs path does not use isolated config root:\n${logPath}`)

  const managedServiceID = "release-smoke-managed"
  child = spawn(binary, [...globalArgs, "_service", "run", "--service-id", managedServiceID, "--service-scope", "user"], { env, stdio: ["ignore", "pipe", "pipe"], windowsHide: true })
  stdout = ""
  stderr = ""
  child.stdout.on("data", (chunk) => { stdout += chunk.toString() })
  child.stderr.on("data", (chunk) => { stderr += chunk.toString() })
  await waitForHealth(`http://127.0.0.1:${reloadedServerPort}/health`, child, () => `${stdout}\n${stderr}`)
  await waitForHealth(`http://127.0.0.1:${reloadedAdminPort}/api/health`, child, () => `${stdout}\n${stderr}`)

  const managedStatus = run(["status"], { quiet: true })
  for (const expected of ["✓ chatgpt-mcp is running", "managed     user ·", `service     ${managedServiceID}`, "session     run_", "status      disabled"]) {
    if (!managedStatus.includes(expected)) fail(`managed status missing ${JSON.stringify(expected)}:\n${managedStatus}`)
  }
  const managedLogs = run(["logs", "--debug", "--event", "server.*", "--grep", "Server", "--tail", "50"], { quiet: true })
  if (!managedLogs.includes("server.ready") && !managedLogs.includes("Server ready")) fail(`managed runtime logs filter returned no server event:\n${managedLogs}`)
  const managedJSON = run(["--log-format=json", "logs", "--event", "server.ready", "--tail", "1"], { quiet: true })
  const managedJSONEvent = JSON.parse(managedJSON.split(/\r?\n/).filter(Boolean).at(-1))
  if (typeof managedJSONEvent.run_id !== "string" || !managedJSONEvent.run_id.startsWith("run_") || managedJSONEvent.service_scope !== "user") {
    fail(`managed JSON logs missing session/service metadata:\n${managedJSON}`)
  }

  run(["logs", "clear", "--force"], { quiet: true })
  let followerStdout = ""
  let followerStderr = ""
  follower = spawn(binary, [...globalArgs, "logs", "--follow", "--event", "config.reloaded", "--tail", "0"], { env, stdio: ["ignore", "pipe", "pipe"], windowsHide: true })
  follower.stdout.on("data", (chunk) => { followerStdout += chunk.toString() })
  follower.stderr.on("data", (chunk) => { followerStderr += chunk.toString() })
  await sleep(250)
  run(["config", "reload"], { quiet: true })
  await waitForText("runtime log follow", follower, () => `${followerStdout}\n${followerStderr}`, "Configuration reloaded")
  await stopChild(follower)
  follower = null

  await stopRuntimeChild(child)
  child = null
  const stoppedLogs = run(["logs", "--event", "config.reloaded", "--tail", "10"], { quiet: true })
  if (!stoppedLogs.includes("Configuration reloaded")) fail(`persisted logs unavailable after managed runtime stopped:\n${stoppedLogs}`)

  run(["uninit"], { quiet: true })
  if (await readFile(defaultSentinel, "utf8") !== "keep\n") fail("isolated commands modified the default config root")
  console.log("[OK] release smoke: init -> verify -> config -> serve/reload/rollback -> MCP -> logs -> managed runtime -> follow/clear -> stop -> uninit")
} finally {
  if (occupied) await closeServer(occupied.server).catch(() => undefined)
  if (follower) await stopChild(follower).catch(() => undefined)
  if (child) await stopChild(child).catch(() => undefined)
  await rm(home, { recursive: true, force: true })
}

function run(args, { quiet = false } = {}) {
  if (globalArgs[0] !== "--config-dir" || !globalArgs[1] || path.resolve(globalArgs[1]) === path.resolve(defaultConfigDir)) {
    fail("runtime smoke requires an explicit isolated --config-dir")
  }
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
  return [result.stdout, result.stderr].filter(Boolean).join("").trim()
}

function runExpectFailure(args) {
  if (globalArgs[0] !== "--config-dir" || !globalArgs[1] || path.resolve(globalArgs[1]) === path.resolve(defaultConfigDir)) {
    fail("runtime smoke requires an explicit isolated --config-dir")
  }
  const result = spawnSync(binary, [...globalArgs, ...args], { env, encoding: "utf8", windowsHide: true })
  if (result.error) fail(`${args.join(" ")}: ${result.error.message}`)
  if (result.status === 0) fail(`${args.join(" ")} unexpectedly succeeded`)
}

async function verifyMCP(port, ponytailEnabled) {
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
  if (!toolNames.has("get_version")) fail(`get_version missing from tools/list: ${JSON.stringify(tools.body)}`)
  if (toolNames.has("ponytail_turn") !== ponytailEnabled) fail(`ponytail_turn state did not match runtime config: ${JSON.stringify(tools.body)}`)
  if (!toolNames.has("caveman_turn")) fail(`enabled caveman_turn missing from tools/list: ${JSON.stringify(tools.body)}`)
  if (!Number.isFinite(tools.body.result.ttlMs) || typeof tools.body.result.cacheScope !== "string") {
    fail(`tools/list cache hints are missing: ${JSON.stringify(tools.body)}`)
  }

  const version = await mcpRequest(port, "tools/call", { name: "get_version", arguments: {} }, 3)
  assertStatus(version.response, 200, "get_version")
  const versionInfo = version.body?.result?.structuredContent
  if (!versionInfo || typeof versionInfo.version !== "string" || typeof versionInfo.commit !== "string" || typeof versionInfo.build_time !== "string") {
    fail(`get_version returned invalid structured content: ${JSON.stringify(version.body)}`)
  }

  const legacy = await mcpRequest(port, "initialize", {}, 4)
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
  const headers = {
    "Content-Type": "application/json",
    "MCP-Protocol-Version": protocolVersion,
    "Mcp-Method": method,
  }
  if (method === "tools/call" && typeof params?.name === "string") headers["Mcp-Name"] = params.name
  const response = await fetch(`http://127.0.0.1:${port}/mcp`, {
    method: "POST",
    headers,
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

async function occupyPort() {
  return await new Promise((resolve, reject) => {
    const server = createServer()
    server.once("error", reject)
    server.listen(0, "127.0.0.1", () => {
      const address = server.address()
      const port = typeof address === "object" && address ? address.port : 0
      if (!port) {
        server.close()
        reject(new Error("failed to reserve occupied port"))
        return
      }
      resolve({ server, port })
    })
  })
}

async function closeServer(server) {
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()))
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

async function waitForText(label, processHandle, output, expected) {
  const deadline = Date.now() + 10000
  while (Date.now() < deadline) {
    const text = output()
    if (text.includes(expected)) return
    if (processHandle.exitCode !== null) fail(`${label} exited with code ${processHandle.exitCode} before ${JSON.stringify(expected)} appeared\n${text.trim()}`)
    await sleep(50)
  }
  fail(`${label} timed out waiting for ${JSON.stringify(expected)}\n${output().trim()}`)
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

async function stopRuntimeChild(server) {
  if (server.exitCode !== null) return
  const controlPath = path.join(configDir, ".runtime-control.json")
  let control
  try {
    control = JSON.parse(await readFile(controlPath, "utf8"))
  } catch (error) {
    fail(`runtime control state unavailable before graceful shutdown: ${error instanceof Error ? error.message : String(error)}`)
  }
  if (!control?.address || !control?.token || control?.pid !== server.pid) {
    fail(`runtime control state does not match child pid ${server.pid}: ${JSON.stringify({ pid: control?.pid, address: control?.address })}`)
  }
  let response
  try {
    response = await fetch(`http://${control.address}/shutdown`, {
      method: "POST",
      headers: { Authorization: `Bearer ${control.token}` },
      signal: AbortSignal.timeout(5000),
    })
  } catch (error) {
    fail(`runtime graceful shutdown request failed: ${error instanceof Error ? error.message : String(error)}`)
  }
  if (!response.ok) fail(`runtime graceful shutdown returned HTTP ${response.status}`)
  const exited = await Promise.race([
    new Promise((resolve) => server.once("exit", () => resolve(true))),
    sleep(5000).then(() => false),
  ])
  if (!exited) fail(`runtime did not exit after graceful shutdown: pid ${server.pid}`)
}

function sleep(ms) { return new Promise((resolve) => setTimeout(resolve, ms)) }
function fail(message) { console.error(`[FAIL] ${message}`); process.exit(1) }
