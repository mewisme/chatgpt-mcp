#!/usr/bin/env node
import { rm, symlink, writeFile } from "node:fs/promises"
import { basename, delimiter, dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import process from "node:process"
import { spawnSync } from "node:child_process"

process.noDeprecation = true

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const args = process.argv.slice(2)
if (args.includes("--help") || args.includes("-h")) {
  console.log(`Usage: node scripts/install-local.mjs

Cross-platform local Go build/install for Linux, Windows, and macOS.
The Admin UI is distributed independently as the admin-ui plugin.`)
  process.exit(0)
}
if (args.length) fail(`unknown argument: ${args[0]}`)

const go = process.platform === "win32" ? "go.exe" : "go"
console.log(`[INFO] repository: ${root}`)
console.log(`[INFO] platform: ${process.platform}/${process.arch}`)
run(go, ["install", "."])
const binaryPath = installedBinaryPath()
const aliasPath = await installAlias(binaryPath)
console.log(`[OK] installed: ${binaryPath}`)
console.log(`[OK] alias: ${aliasPath}`)

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
    await writeFile(aliasPath, '@echo off\r\nset "CHATGPT_MCP_CLI_NAME=cgm"\r\n"%~dp0chatgpt-mcp.exe" %*\r\n', "ascii")
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
