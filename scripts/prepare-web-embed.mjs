#!/usr/bin/env node
import { access } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import process from "node:process"
import { spawnSync } from "node:child_process"

process.noDeprecation = true

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const args = process.argv.slice(2)
const options = { installDeps: true, fromDist: false, output: resolve(root, "dist/plugins") }
for (let i = 0; i < args.length; i++) {
  const arg = args[i]
  if (arg === "--no-deps") options.installDeps = false
  else if (arg === "--from-dist") options.fromDist = true
  else if (arg === "--output") options.output = resolve(root, args[++i] ?? fail("--output requires a path"))
  else if (arg === "--help" || arg === "-h") {
    console.log(`Usage: node scripts/prepare-web-embed.mjs [--no-deps] [--from-dist] [--output PATH]

Compatibility helper that builds the Admin UI plugin. It no longer embeds assets in the Go binary.`)
    process.exit(0)
  } else fail(`unknown argument: ${arg}`)
}

if (!options.fromDist) {
  if (options.installDeps) runPnpm(["--dir", "web", "install", "--frozen-lockfile"])
  runPnpm(["--dir", "web", "build"])
}
await requireFile("web/dist/index.html")
run(process.platform === "win32" ? "go.exe" : "go", ["run", "./plugins/admin-ui/build", "--source-root", "web/dist", "--output", options.output])

async function requireFile(relative) {
  try {
    await access(resolve(root, relative))
  } catch {
    fail(`required file not found: ${relative}`)
  }
}

function runPnpm(commandArgs) {
  const command = process.platform === "win32" ? "pnpm.cmd" : "pnpm"
  console.log(`[RUN] pnpm ${commandArgs.join(" ")}`)
  const result = spawnSync(command, commandArgs, { cwd: root, stdio: "inherit", windowsHide: true, shell: process.platform === "win32" })
  if (result.error) fail(`pnpm: ${result.error.message}`)
  if (result.status !== 0) fail(`pnpm exited with code ${result.status}`)
}

function run(command, commandArgs) {
  console.log(`[RUN] ${command} ${commandArgs.join(" ")}`)
  const result = spawnSync(command, commandArgs, { cwd: root, stdio: "inherit", windowsHide: true })
  if (result.error) fail(`${command}: ${result.error.message}`)
  if (result.status !== 0) fail(`${command} exited with code ${result.status}`)
}

function fail(message) {
  console.error(`[FAIL] ${message}`)
  process.exit(1)
}
