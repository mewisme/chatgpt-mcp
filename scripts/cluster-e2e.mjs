import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { createServer } from "node:net"
import { tmpdir } from "node:os"
import path from "node:path"
import process from "node:process"
import { spawn, spawnSync } from "node:child_process"

process.noDeprecation = true

const protocolVersion = "2026-07-28"
const input = process.argv[2]
if (!input) fail("usage: node scripts/cluster-e2e.mjs <binary>")

const binary = path.resolve(input)
const home = await mkdtemp(path.join(tmpdir(), "chatgpt-mcp-cluster-e2e-"))
const env = { ...process.env, HOME: home, USERPROFILE: home }
delete env.CHATGPT_MCP_TOOL_CONTEXT
const relayRoot = path.join(home, "relay")
const runtimeARoot = path.join(home, "runtime-a")
const runtimeBRoot = path.join(home, "runtime-b")
const workspaceA = path.join(home, "workspace-a")
const workspaceB = path.join(home, "workspace-b")
const relayPort = await freePort()
const serverAPort = await freePort()
const adminAPort = await freePort()
const serverBPort = await freePort()
const adminBPort = await freePort()
const relayToken = "cluster-e2e-secret"
const relayURL = `ws://127.0.0.1:${relayPort}/cluster`
let relay = null
let runtimeA = null
let runtimeB = null

try {
  await mkdir(workspaceA, { recursive: true })
  await mkdir(workspaceB, { recursive: true })
  await writeFile(path.join(workspaceB, "owner.txt"), "from-runtime-b\n")
  initRoot(relayRoot)
  run(relayRoot, ["config", "set", "cluster.relay_token", relayToken])
  initRuntime(runtimeARoot, serverAPort, adminAPort)
  initRuntime(runtimeBRoot, serverBPort, adminBPort)
  configureCluster(runtimeARoot)
  configureCluster(runtimeBRoot)
  run(runtimeARoot, ["workspace", "register", workspaceA])
  run(runtimeBRoot, ["workspace", "register", workspaceB])
  const workspaceBID = workspaceList(runtimeBRoot).find((item) => path.resolve(item.path) === path.resolve(workspaceB))?.id
  if (!workspaceBID) fail("runtime B workspace_id was not found")

  relay = spawnProcess(relayRoot, ["cluster", "relay", "--listen", `127.0.0.1:${relayPort}`])
  await waitForHealth(`http://127.0.0.1:${relayPort}/health`, relay)
  runtimeA = spawnProcess(runtimeARoot, ["serve"])
  runtimeB = spawnProcess(runtimeBRoot, ["serve"])
  await waitForHealth(`http://127.0.0.1:${serverAPort}/health`, runtimeA)
  await waitForHealth(`http://127.0.0.1:${serverBPort}/health`, runtimeB)
  await waitForCluster(runtimeARoot, 2)
  await waitForCluster(runtimeBRoot, 2)
  await verifyRemoteRead(serverAPort, workspaceBID, "from-runtime-b")
  await verifyRemoteWrite(serverAPort, workspaceBID, "remote-before-restart.txt", "written-through-a")
  if (await readFile(path.join(workspaceB, "remote-before-restart.txt"), "utf8") !== "written-through-a") fail("remote mutation did not land on runtime B")
  await verifyRelayMetrics(2)

  await stopChild(relay)
  relay = null
  await waitForClusterDisconnected(runtimeARoot)
  await waitForClusterDisconnected(runtimeBRoot)
  relay = spawnProcess(relayRoot, ["cluster", "relay", "--listen", `127.0.0.1:${relayPort}`])
  await waitForHealth(`http://127.0.0.1:${relayPort}/health`, relay)
  await waitForCluster(runtimeARoot, 2)
  await waitForCluster(runtimeBRoot, 2)
  await verifyRemoteRead(serverAPort, workspaceBID, "from-runtime-b")
  await verifyRelayMetrics(2)

  await stopRuntime(runtimeBRoot, runtimeB)
  runtimeB = null
  await waitForWorkspaceOffline(runtimeARoot, workspaceBID)
  await verifyRemoteFailure(serverAPort, workspaceBID, "workspace owner is offline")
  runtimeB = spawnProcess(runtimeBRoot, ["serve"])
  await waitForHealth(`http://127.0.0.1:${serverBPort}/health`, runtimeB)
  await waitForCluster(runtimeARoot, 2)
  await waitForCluster(runtimeBRoot, 2)
  await verifyRemoteWrite(serverAPort, workspaceBID, "remote-after-restart.txt", "owner-reconnected")
  if (await readFile(path.join(workspaceB, "remote-after-restart.txt"), "utf8") !== "owner-reconnected") fail("remote mutation failed after owner reconnect")

  console.log("[OK] cluster e2e: relay + two runtimes -> remote read/write -> relay restart/reconnect -> owner restart/re-advertise")
} finally {
  if (runtimeA) await stopRuntime(runtimeARoot, runtimeA).catch(() => undefined)
  if (runtimeB) await stopRuntime(runtimeBRoot, runtimeB).catch(() => undefined)
  if (relay) await stopChild(relay).catch(() => undefined)
  await rm(home, { recursive: true, force: true })
}

function initRoot(root) { run(root, ["init"], { quiet: true }) }

function initRuntime(root, serverPort, adminPort) {
  initRoot(root)
  run(root, ["config", "set", "server.port", String(serverPort)])
  run(root, ["config", "set", "admin.port", String(adminPort)])
  run(root, ["config", "set", "auth.mcp_enabled", "false"])
  run(root, ["config", "set", "auth.admin_enabled", "false"])
}

function configureCluster(root) {
  run(root, ["config", "set", "cluster.relay_url", relayURL])
  run(root, ["config", "set", "cluster.relay_token", relayToken])
  run(root, ["config", "set", "cluster.enabled", "true"])
  run(root, ["config", "verify"])
}

function workspaceList(root) { return JSON.parse(run(root, ["workspace", "list", "--json"], { quiet: true })) }

function clusterStatus(root) {
  const output = run(root, ["--log-format=json", "cluster", "status"], { quiet: true })
  return JSON.parse(output.split(/\r?\n/).filter(Boolean).at(-1))
}

function run(root, args, { quiet = false } = {}) {
  const result = spawnSync(binary, ["--config-dir", root, ...args], { env, encoding: "utf8", windowsHide: true })
  if (result.error) fail(`${args.join(" ")}: ${result.error.message}`)
  if (result.status !== 0) fail(`${args.join(" ")} failed with exit code ${result.status}\n${[result.stdout, result.stderr].filter(Boolean).join("\n").trim()}`)
  const output = [result.stdout, result.stderr].filter(Boolean).join("").trim()
  if (!quiet && output) console.log(output)
  return output
}

function spawnProcess(root, args) {
  const child = spawn(binary, ["--config-dir", root, ...args], { env, stdio: ["ignore", "pipe", "pipe"], windowsHide: true })
  child.stdoutText = ""
  child.stderrText = ""
  child.stdout.on("data", (chunk) => { child.stdoutText += chunk.toString() })
  child.stderr.on("data", (chunk) => { child.stderrText += chunk.toString() })
  return child
}

async function waitForHealth(url, child) {
  await waitFor(`health ${url}`, async () => {
    if (child.exitCode !== null) fail(`process exited with code ${child.exitCode}\n${processOutput(child)}`)
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(750) })
      if (!response.ok) return false
      return (await response.json())?.ok === true
    } catch { return false }
  })
}

async function waitForCluster(root, onlineMembers) {
  await waitFor(`cluster ${root}`, () => {
    try {
      const status = clusterStatus(root)
      return status.connected === true && status.online_member_count === onlineMembers && status.catalog_compatible === true
    } catch { return false }
  })
}

async function waitForClusterDisconnected(root) {
  await waitFor(`cluster disconnect ${root}`, () => {
    try { return clusterStatus(root).connected === false } catch { return false }
  })
}

async function waitForWorkspaceOffline(root, workspaceID) {
  await waitFor(`workspace offline ${workspaceID}`, () => {
    try {
      const status = clusterStatus(root)
      return status.connected === true && status.workspaces?.some((item) => item.workspace_id === workspaceID && item.online === false)
    } catch { return false }
  })
}

async function verifyRelayMetrics(activeConnections) {
  const response = await fetch(`http://127.0.0.1:${relayPort}/metrics`, { headers: { Authorization: `Bearer ${relayToken}` }, signal: AbortSignal.timeout(2000) })
  if (!response.ok) fail(`relay metrics status = ${response.status}`)
  const metrics = await response.json()
  if (metrics.active_connections !== activeConnections || metrics.online_member_count !== activeConnections || metrics.member_count !== activeConnections || metrics.catalog_compatible !== true) {
    fail(`unexpected relay metrics: ${JSON.stringify(metrics)}`)
  }
}

async function verifyRemoteRead(port, workspaceID, expected) {
  const body = await toolCall(port, "read_text_file", { workspace_id: workspaceID, path: "owner.txt" })
  if (body?.result?.isError) fail(`remote read returned tool error: ${JSON.stringify(body)}`)
  if (body?.result?.structuredContent?.content !== `${expected}\n`) fail(`remote read content = ${JSON.stringify(body?.result?.structuredContent)}`)
}

async function verifyRemoteWrite(port, workspaceID, file, content) {
  const body = await toolCall(port, "write_file", { workspace_id: workspaceID, path: file, content })
  if (body?.result?.isError) fail(`remote write returned tool error: ${JSON.stringify(body)}`)
}

async function verifyRemoteFailure(port, workspaceID, expected) {
  const body = await toolCall(port, "read_text_file", { workspace_id: workspaceID, path: "owner.txt" })
  const text = JSON.stringify(body)
  if (!body?.result?.isError || !text.includes(expected)) fail(`remote failure did not include ${JSON.stringify(expected)}: ${text}`)
}

async function toolCall(port, name, args) {
  const payload = { jsonrpc: "2.0", id: Date.now(), method: "tools/call", params: { name, arguments: args, _meta: { "io.modelcontextprotocol/protocolVersion": protocolVersion, "io.modelcontextprotocol/clientCapabilities": {}, "io.modelcontextprotocol/clientInfo": { name: "cluster-e2e", version: "1.0.0" } } } }
  const response = await fetch(`http://127.0.0.1:${port}/mcp`, { method: "POST", headers: { "Content-Type": "application/json", "MCP-Protocol-Version": protocolVersion, "Mcp-Method": "tools/call", "Mcp-Name": name }, body: JSON.stringify(payload), signal: AbortSignal.timeout(5000) })
  if (!response.ok) fail(`${name} HTTP status = ${response.status}: ${await response.text()}`)
  return await response.json()
}

async function stopRuntime(root, child) {
  if (child.exitCode !== null) return
  const control = JSON.parse(await readFile(path.join(root, ".runtime-control.json"), "utf8"))
  if (!control?.address || !control?.token || control.pid !== child.pid) fail(`runtime control state does not match pid ${child.pid}`)
  const response = await fetch(`http://${control.address}/shutdown`, { method: "POST", headers: { Authorization: `Bearer ${control.token}` }, signal: AbortSignal.timeout(3000) })
  if (!response.ok) fail(`runtime shutdown returned HTTP ${response.status}`)
  if (!await waitForExit(child, 5000)) fail(`runtime ${child.pid} did not exit after graceful shutdown`)
}

async function stopChild(child) {
  if (child.exitCode !== null) return
  child.kill("SIGTERM")
  if (await waitForExit(child, 5000)) return
  child.kill("SIGKILL")
  await waitForExit(child, 2000)
}

async function waitForExit(child, timeout) {
  if (child.exitCode !== null) return true
  return await Promise.race([new Promise((resolve) => child.once("exit", () => resolve(true))), sleep(timeout).then(() => false)])
}

async function waitFor(label, condition, timeout = 15000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await condition()) return
    await sleep(100)
  }
  fail(`${label} timed out`)
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

function processOutput(child) { return `${child.stdoutText || ""}\n${child.stderrText || ""}`.trim() }
function sleep(ms) { return new Promise((resolve) => setTimeout(resolve, ms)) }
function fail(message) { throw new Error(message) }