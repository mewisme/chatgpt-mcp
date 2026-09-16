#!/usr/bin/env node
import { access, copyFile, mkdir, readFile, rm, stat } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, isAbsolute, relative, resolve, sep } from "node:path"
import { fileURLToPath } from "node:url"
import process from "node:process"
import { spawnSync } from "node:child_process"

process.noDeprecation = true

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const definitionPath = resolve(root, "plugins/workflow.json")
const idPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

export async function loadWorkflow(path = definitionPath) {
  const workflow = JSON.parse(await readFile(path, "utf8"))
  return validateWorkflow(workflow)
}

export function validateWorkflow(workflow) {
  if (!workflow || workflow.schema !== 1 || !Array.isArray(workflow.plugins) || !workflow.plugins.length) throw new Error("plugin workflow schema must be 1 with a non-empty plugins array")
  const ids = new Set()
  for (const plugin of workflow.plugins) {
    if (!plugin || typeof plugin.id !== "string" || !idPattern.test(plugin.id)) throw new Error(`invalid plugin workflow id ${JSON.stringify(plugin?.id)}`)
    if (ids.has(plugin.id)) throw new Error(`duplicate plugin workflow id ${plugin.id}`)
    ids.add(plugin.id)
    validateBuild(plugin)
    if (plugin.smoke !== undefined) validateSmoke(plugin)
    if (plugin.core !== undefined) validateCore(plugin)
  }
  return workflow
}

export function workflowMatrix(workflow, kind) {
  if (kind === "build") return { include: workflow.plugins.map(plugin => ({ id: plugin.id, runner: plugin.build.runner })) }
  if (kind === "smoke") return { include: workflow.plugins.filter(plugin => plugin.smoke).map(plugin => ({ id: plugin.id, runner: plugin.smoke.runner })) }
  throw new Error(`unknown plugin workflow matrix ${kind}`)
}

export async function validateRepositoryWorkflow(workflow) {
  const index = JSON.parse(await readFile(resolve(root, "plugins/registry/index.json"), "utf8"))
  const defined = workflow.plugins.map(plugin => plugin.id).sort()
  const registered = Object.keys(index.plugins ?? {}).sort()
  if (JSON.stringify(defined) !== JSON.stringify(registered)) throw new Error(`plugin workflow definitions do not match registry plugins: workflow=${defined.join(",")} registry=${registered.join(",")}`)
  for (const plugin of workflow.plugins) {
    const manifest = await readPluginManifest(plugin.id)
    const entry = index.plugins[plugin.id]
    if (entry.name !== manifest.name || entry.type !== manifest.type || entry.license !== manifest.license || !entry.versions?.[manifest.version]) throw new Error(`plugin workflow ${plugin.id} source manifest is not represented by registry index`)
    if (!sameCore(plugin.core, entry.core)) throw new Error(`plugin workflow ${plugin.id} core metadata does not match registry index`)
  }
  return workflow
}

export async function buildPlugin(workflow, id, outputRoot) {
  const plugin = findPlugin(workflow, id)
  const output = resolve(root, outputRoot)
  await mkdir(output, { recursive: true })
  const sourceManifest = await readPluginManifest(id)
  const expectedManifest = resolve(output, `${id}-${sourceManifest.version}.json`)
  if ((plugin.build.mode ?? "command") === "manifest") await copyFile(resolve(root, `plugins/${id}/plugin.json`), expectedManifest)
  else {
    const command = plugin.build.command.map(value => interpolate(value, { id, version: sourceManifest.version, output }))
    run(command[0], command.slice(1))
  }
  await access(expectedManifest)
  const generated = JSON.parse(await readFile(expectedManifest, "utf8"))
  if (generated.id !== id || generated.version !== sourceManifest.version) throw new Error(`plugin ${id} build produced mismatched manifest identity`)
  return expectedManifest
}

export async function smokePlugin(workflow, id) {
  const plugin = findPlugin(workflow, id)
  if (!plugin.smoke) throw new Error(`plugin ${id} has no smoke definition`)
  const manifest = await readPluginManifest(id)
  const base = resolve(tmpdir(), `chatgpt-mcp-plugin-smoke-${id}-${process.pid}-${Date.now()}`)
  const configRoot = resolve(base, "config")
  try {
    run(goExecutable(), ["run", ".", "--config-dir", configRoot, "plugin", "install", id])
    run(goExecutable(), ["run", ".", "--config-dir", configRoot, "plugin", "verify", id])
    const installedRoot = resolve(`${configRoot}-data`, "plugins", id, manifest.version)
    const probe = plugin.smoke.probe
    const target = resolveInside(installedRoot, probe.path)
    if (probe.kind === "file") {
      const info = await stat(target)
      if (!info.isFile()) throw new Error(`plugin ${id} smoke probe is not a file: ${probe.path}`)
      return
    }
    const result = run(target, probe.args ?? [], { capture: true })
    if (probe.stdout !== undefined && result.stdout.trim() !== probe.stdout) throw new Error(`plugin ${id} smoke output mismatch: ${JSON.stringify(result.stdout.trim())}`)
  } finally {
    await rm(base, { recursive: true, force: true })
  }
}

function validateBuild(plugin) {
  const build = plugin.build
  if (!build || typeof build.runner !== "string" || !build.runner.trim()) throw new Error(`plugin workflow ${plugin.id} requires build.runner`)
  const mode = build.mode ?? "command"
  if (mode !== "command" && mode !== "manifest") throw new Error(`plugin workflow ${plugin.id} has unsupported build mode ${mode}`)
  if (mode === "command") validateCommand(plugin.id, "build.command", build.command)
}

function validateSmoke(plugin) {
  const smoke = plugin.smoke
  if (!smoke || typeof smoke.runner !== "string" || !smoke.runner.trim()) throw new Error(`plugin workflow ${plugin.id} requires smoke.runner`)
  const probe = smoke.probe
  if (!probe || (probe.kind !== "file" && probe.kind !== "command")) throw new Error(`plugin workflow ${plugin.id} smoke probe must be file or command`)
  validateRelativePath(plugin.id, "smoke.probe.path", probe.path)
  if (probe.kind === "command") {
    if (probe.args !== undefined && (!Array.isArray(probe.args) || probe.args.some(value => typeof value !== "string"))) throw new Error(`plugin workflow ${plugin.id} smoke probe args must be strings`)
    if (probe.stdout !== undefined && typeof probe.stdout !== "string") throw new Error(`plugin workflow ${plugin.id} smoke probe stdout must be a string`)
  }
}

function validateCore(plugin) {
  const core = plugin.core
  if (!core || typeof core !== "object" || Array.isArray(core)) throw new Error(`plugin workflow ${plugin.id} core must be an object`)
  for (const key of Object.keys(core)) {
    if (!["required", "enabled", "platforms"].includes(key)) throw new Error(`plugin workflow ${plugin.id} has unsupported core field ${key}`)
  }
  if (core.required !== undefined && typeof core.required !== "boolean") throw new Error(`plugin workflow ${plugin.id} core.required must be a boolean`)
  if (core.enabled !== undefined && typeof core.enabled !== "boolean") throw new Error(`plugin workflow ${plugin.id} core.enabled must be a boolean`)
  if (core.platforms !== undefined) {
    if (!Array.isArray(core.platforms) || !core.platforms.length || core.platforms.some(value => typeof value !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*\/[a-z0-9]+(?:-[a-z0-9]+)*$/.test(value))) {
      throw new Error(`plugin workflow ${plugin.id} core.platforms must be os/arch values`)
    }
  }
}

function sameCore(left, right) {
  const fingerprint = core => core == null ? null : JSON.stringify({
    required: core.required === true,
    enabled: core.enabled !== false,
    platforms: [...(core.platforms ?? [])].sort()
  })
  return fingerprint(left) === fingerprint(right)
}

function validateCommand(id, field, command) {
  if (!Array.isArray(command) || !command.length || command.some(value => typeof value !== "string" || !value)) throw new Error(`plugin workflow ${id} ${field} must be a non-empty argv array`)
}

function validateRelativePath(id, field, path) {
  if (typeof path !== "string" || !path || isAbsolute(path) || path.split(/[\\/]+/).some(part => part === ".." || part === "")) throw new Error(`plugin workflow ${id} ${field} must be a safe relative path`)
}

function findPlugin(workflow, id) {
  const plugin = workflow.plugins.find(candidate => candidate.id === id)
  if (!plugin) throw new Error(`plugin ${id} is not defined in plugins/workflow.json`)
  return plugin
}

async function readPluginManifest(id) {
  const manifest = JSON.parse(await readFile(resolve(root, `plugins/${id}/plugin.json`), "utf8"))
  if (manifest.id !== id || typeof manifest.version !== "string" || !manifest.version) throw new Error(`plugin ${id} source manifest identity is invalid`)
  return manifest
}

function interpolate(value, variables) {
  return value.replace(/\{(id|version|output)\}/g, (_, name) => variables[name])
}

function resolveInside(base, path) {
  const target = resolve(base, path)
  const rel = relative(base, target)
  if (!rel || rel === ".." || rel.startsWith(`..${sep}`) || isAbsolute(rel)) throw new Error(`plugin smoke path escapes installed root: ${path}`)
  return target
}

function goExecutable() {
  return process.platform === "win32" ? "go.exe" : "go"
}

function run(command, args, options = {}) {
  const capture = options.capture === true
  const result = spawnSync(command, args, { cwd: root, encoding: capture ? "utf8" : undefined, stdio: capture ? ["ignore", "pipe", "pipe"] : "inherit", windowsHide: true })
  if (result.error) throw result.error
  if (result.status !== 0) {
    const details = capture ? `\n${result.stdout ?? ""}${result.stderr ?? ""}` : ""
    throw new Error(`${command} exited with code ${result.status}${details}`)
  }
  return result
}

async function main() {
  const [operation, arg1, arg2] = process.argv.slice(2)
  const workflow = await loadWorkflow()
  if (operation === "validate") {
    await validateRepositoryWorkflow(workflow)
    return
  }
  if (operation === "matrix") {
    process.stdout.write(`${JSON.stringify(workflowMatrix(workflow, arg1))}\n`)
    return
  }
  if (operation === "build") {
    if (!arg1 || !arg2) throw new Error("usage: plugin-workflow.mjs build <plugin-id> <output-dir>")
    await buildPlugin(workflow, arg1, arg2)
    return
  }
  if (operation === "smoke") {
    if (!arg1) throw new Error("usage: plugin-workflow.mjs smoke <plugin-id>")
    await smokePlugin(workflow, arg1)
    return
  }
  throw new Error("usage: plugin-workflow.mjs validate | matrix <build|smoke> | build <plugin-id> <output-dir> | smoke <plugin-id>")
}

const invoked = process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)
if (invoked) main().catch(error => {
  console.error(`[FAIL] ${error.message}`)
  process.exit(1)
})
