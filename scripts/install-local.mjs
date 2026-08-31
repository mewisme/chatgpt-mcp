#!/usr/bin/env node
import { access, rm, symlink, writeFile } from "node:fs/promises"
import { basename, delimiter, dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import process from "node:process"
import { spawnSync } from "node:child_process"

process.noDeprecation = true

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const args = process.argv.slice(2)
const options = { installDeps: true, prepareOnly: false, fromDist: false }

for (const arg of args) {
  if (arg === "--no-deps") options.installDeps = false
  else if (arg === "--prepare-only") options.prepareOnly = true
  else if (arg === "--from-dist") options.fromDist = true
  else if (arg === "--help" || arg === "-h") {
    console.log(`Usage: node scripts/install-local.mjs [--no-deps] [--prepare-only] [--from-dist]

Cross-platform local build/install for Linux, Windows, and macOS.

Default flow:
  1. pnpm --dir web install --frozen-lockfile
  2. pnpm --dir web build
  3. copy web/dist -> internal/web/dist
  4. go install .
  5. install cgm alias beside chatgpt-mcp

Options:
  --no-deps       Skip pnpm install.
  --prepare-only  Build and prepare embedded web assets without go install .
  --from-dist     Use an existing web/dist and skip pnpm install/build.
  -h, --help      Show this help.`)
    process.exit(0)
  } else fail(`unknown argument: ${arg}`)
}

const pnpm = process.platform === "win32" ? "pnpm.cmd" : "pnpm"
const go = process.platform === "win32" ? "go.exe" : "go"

await requireFile("go.mod")
await requireFile("web/package.json")
await requireFile("web/pnpm-lock.yaml")
await requireFile("scripts/prepare-web-embed.mjs")
if (options.fromDist) await requireFile("web/dist/index.html")

console.log(`[INFO] repository: ${root}`)
console.log(`[INFO] platform: ${process.platform}/${process.arch}`)

if (!options.fromDist) {
  if (options.installDeps) run(pnpm, ["--dir", "web", "install", "--frozen-lockfile"])
  run(pnpm, ["--dir", "web", "build"])
}
run(process.execPath, [resolve(root, "scripts/prepare-web-embed.mjs")])

if (options.prepareOnly) {
  console.log("[OK] local web embed is ready")
  process.exit(0)
}

run(go, ["install", "."])
const binaryPath = installedBinaryPath()
const aliasPath = await installAlias(binaryPath)
console.log(`[OK] installed: ${binaryPath}`)
console.log(`[OK] alias: ${aliasPath}`)

async function requireFile(relative) {
  try {
    await access(resolve(root, relative))
  } catch {
    fail(`required file not found: ${relative}`)
  }
}

function run(command, commandArgs) {
  console.log(`[RUN] ${command} ${commandArgs.join(" ")}`)
  const result = spawnSync(command, commandArgs, { cwd: root, stdio: "inherit", windowsHide: true })
  if (result.error) fail(`${command}: ${result.error.message}`)
  if (result.status !== 0) fail(`${command} exited with code ${result.status}`)
}

function capture(command, commandArgs) {
  const result = spawnSync(command, commandArgs, { cwd: root, encoding: "utf8", windowsHide: true })
  if (result.error || result.status !== 0) return ""
  return result.stdout.trim()
}

function installedBinaryPath() {
  const name = process.platform === "win32" ? "chatgpt-mcp.exe" : "chatgpt-mcp"
  const gobin = capture(go, ["env", "GOBIN"])
  if (gobin) return resolve(gobin, name)
  const gopath = capture(go, ["env", "GOPATH"]).split(delimiter).filter(Boolean)[0]
  return gopath ? resolve(gopath, "bin", name) : name
}

async function installAlias(binaryPath) {
  const dir = dirname(binaryPath)
  if (process.platform === "win32") {
    await rm(resolve(dir, "cmcp.cmd"), { force: true })
    const aliasPath = resolve(dir, "cgm.cmd")
    await writeFile(aliasPath, '@echo off\r\n"%~dp0chatgpt-mcp.exe" %*\r\n', "ascii")
    return aliasPath
  }
  await rm(resolve(dir, "cmcp"), { force: true })
  const aliasPath = resolve(dir, "cgm")
  await rm(aliasPath, { force: true })
  await symlink(basename(binaryPath), aliasPath)
  return aliasPath
}

function fail(message) {
  console.error(`[FAIL] ${message}`)
  process.exit(1)
}
