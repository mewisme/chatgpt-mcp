#!/usr/bin/env node
import { readFile, writeFile } from "node:fs/promises"
import { resolve } from "node:path"
import process from "node:process"

const options = { check: false, scoop: "dist/scoop/chatgpt-mcp.json", homebrew: "dist/homebrew/Casks/chatgpt-mcp.rb" }
for (let index = 2; index < process.argv.length; index++) {
  const arg = process.argv[index]
  if (arg === "--check") options.check = true
  else if (arg === "--scoop") options.scoop = process.argv[++index] || fail("--scoop requires a path")
  else if (arg === "--homebrew") options.homebrew = process.argv[++index] || fail("--homebrew requires a path")
  else if (arg === "--help" || arg === "-h") {
    console.log("Usage: node scripts/prepare-package-aliases.mjs [--check] [--scoop <path>] [--homebrew <path>]")
    process.exit(0)
  } else fail(`unknown argument: ${arg}`)
}

const scoopPath = resolve(options.scoop)
const homebrewPath = resolve(options.homebrew)
if (options.check) {
  await checkScoop(scoopPath)
  await checkHomebrew(homebrewPath)
  console.log("[OK] package aliases verified: chatgpt-mcp + cgm")
} else {
  await patchScoop(scoopPath)
  await patchHomebrew(homebrewPath)
  console.log("[OK] package aliases prepared: chatgpt-mcp + cgm")
}

async function patchScoop(path) {
  const manifest = JSON.parse(await readFile(path, "utf8"))
  const scopes = scoopScopes(manifest)
  if (scopes.length === 0) fail(`Scoop manifest has no bin entries: ${path}`)
  for (const scope of scopes) scope.bin = [["chatgpt-mcp.exe", "chatgpt-mcp"], ["chatgpt-mcp.exe", "cgm"]]
  await writeFile(path, `${JSON.stringify(manifest, null, 4)}\n`)
}

async function checkScoop(path) {
  const manifest = JSON.parse(await readFile(path, "utf8"))
  const scopes = scoopScopes(manifest)
  if (scopes.length === 0 || scopes.some((scope) => !hasScoopAlias(scope.bin, "chatgpt-mcp") || !hasScoopAlias(scope.bin, "cgm"))) fail(`Scoop aliases missing: ${path}`)
}

function scoopScopes(manifest) {
  const scopes = []
  if (manifest && Object.hasOwn(manifest, "bin")) scopes.push(manifest)
  for (const scope of Object.values(manifest?.architecture || {})) if (scope && typeof scope === "object" && Object.hasOwn(scope, "bin")) scopes.push(scope)
  return scopes
}

function hasScoopAlias(bin, alias) {
  return Array.isArray(bin) && bin.some((entry) => Array.isArray(entry) && entry[0] === "chatgpt-mcp.exe" && entry[1] === alias)
}

async function patchHomebrew(path) {
  let content = (await readFile(path, "utf8")).replace(/^\s*binary "chatgpt-mcp", target: "cmcp"\s*\n?/gm, "")
  if (!/^\s*binary "chatgpt-mcp", target: "cgm"\s*$/m.test(content)) {
    const match = content.match(/^(\s*)binary "chatgpt-mcp"\s*$/m)
    if (!match) fail(`Homebrew chatgpt-mcp binary stanza missing: ${path}`)
    content = content.replace(match[0], `${match[0]}\n${match[1]}binary "chatgpt-mcp", target: "cgm"`)
  }
  await writeFile(path, content.endsWith("\n") ? content : `${content}\n`)
}

async function checkHomebrew(path) {
  const content = await readFile(path, "utf8")
  if (!/^\s*binary "chatgpt-mcp"\s*$/m.test(content) || !/^\s*binary "chatgpt-mcp", target: "cgm"\s*$/m.test(content) || /^\s*binary "chatgpt-mcp", target: "cmcp"\s*$/m.test(content)) fail(`Homebrew aliases invalid: ${path}`)
}

function fail(message) {
  console.error(`[FAIL] ${message}`)
  process.exit(1)
}