import assert from "node:assert/strict"
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"
import { spawnSync } from "node:child_process"
import test from "node:test"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const script = resolve(root, "scripts/prepare-package-aliases.mjs")

test("prepares and verifies Scoop and Homebrew aliases", async () => {
  const dir = await mkdtemp(resolve(tmpdir(), "chatgpt-mcp-package-aliases-"))
  const scoop = resolve(dir, "chatgpt-mcp.json")
  const homebrew = resolve(dir, "chatgpt-mcp.rb")
  try {
    await writeFile(scoop, `${JSON.stringify({ version: "1.0.0", architecture: { "64bit": { bin: "chatgpt-mcp.exe" }, arm64: { bin: "chatgpt-mcp.exe" } } }, null, 2)}\n`)
    await writeFile(homebrew, 'cask "chatgpt-mcp" do\n  version "1.0.0"\n  binary "chatgpt-mcp"\n  binary "chatgpt-mcp", target: "cmcp"\nend\n')

    run(["--scoop", scoop, "--homebrew", homebrew])
    run(["--check", "--scoop", scoop, "--homebrew", homebrew])
    run(["--scoop", scoop, "--homebrew", homebrew])

    const manifest = JSON.parse(await readFile(scoop, "utf8"))
    const expected = [["chatgpt-mcp.exe", "chatgpt-mcp"], ["chatgpt-mcp.exe", "cgm"]]
    assert.deepEqual(manifest.architecture["64bit"].bin, expected)
    assert.deepEqual(manifest.architecture.arm64.bin, expected)
    const cask = await readFile(homebrew, "utf8")
    assert.equal((cask.match(/binary "chatgpt-mcp"/g) || []).length, 2)
    assert.match(cask, /binary "chatgpt-mcp", target: "cgm"/)
    assert.doesNotMatch(cask, /target: "cmcp"/)
  } finally {
    await rm(dir, { recursive: true, force: true })
  }
})

function run(args) {
  const result = spawnSync(process.execPath, [script, ...args], { cwd: root, encoding: "utf8", windowsHide: true })
  assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`)
}