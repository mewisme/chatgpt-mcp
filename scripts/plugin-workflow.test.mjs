import assert from "node:assert/strict"
import { access, constants, mkdtemp, readFile, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { resolve } from "node:path"
import test from "node:test"

import { buildPlugin, loadWorkflow, validateRepositoryWorkflow, validateWorkflow, workflowMatrix } from "./plugin-workflow.mjs"

test("admin-ui frontend lives in the plugin directory", async () => {
  await access(resolve("plugins/admin-ui/package.json"), constants.F_OK)
  await access(resolve("plugins/admin-ui/src"), constants.F_OK)
  await assert.rejects(() => access(resolve("web/package.json"), constants.F_OK))
})

test("plugin workflow definitions drive build and smoke matrices", async () => {
  const workflow = await loadWorkflow()
  const build = workflowMatrix(workflow, "build").include
  const smoke = workflowMatrix(workflow, "smoke").include
  assert.deepEqual(build.map(entry => entry.id), workflow.plugins.map(plugin => plugin.id))
  assert.deepEqual(smoke.map(entry => entry.id), workflow.plugins.filter(plugin => plugin.smoke).map(plugin => plugin.id))
  for (const entry of build) {
    const plugin = workflow.plugins.find(candidate => candidate.id === entry.id)
    assert.equal(entry.runner, plugin.build.runner)
  }
})

test("plugin workflow definitions match registry and source manifests", async () => {
  const workflow = await loadWorkflow()
  await validateRepositoryWorkflow(workflow)
})

test("declared core plugins are default-enabled and optional", async () => {
  const workflow = await loadWorkflow()
  const coreIDs = ["admin-ui", "secure-mcp-tunnel", "tui", "markdown-formatter"]
  for (const id of coreIDs) {
    const plugin = workflow.plugins.find(candidate => candidate.id === id)
    assert.equal(plugin?.core?.required, false, id)
    assert.equal(plugin?.core?.enabled, true, id)
  }
  for (const plugin of workflow.plugins.filter(candidate => !coreIDs.includes(candidate.id))) {
    assert.equal(plugin.core, undefined, plugin.id)
  }
})

test("plugin workflow rejects invalid core metadata", () => {
  assert.throws(() => validateWorkflow({
    schema: 1,
    plugins: [{ id: "demo", build: { runner: "ubuntu-latest", mode: "manifest" }, core: { required: "yes" } }]
  }), /core\.required must be a boolean/)
})

test("plugin workflow rejects duplicate plugin ids", () => {
  assert.throws(() => validateWorkflow({
    schema: 1,
    plugins: [
      { id: "demo", build: { runner: "ubuntu-latest", mode: "manifest" } },
      { id: "demo", build: { runner: "ubuntu-latest", mode: "manifest" } }
    ]
  }), /duplicate plugin workflow id demo/)
})

test("manifest-only build copies the versioned source manifest", async () => {
  const workflow = await loadWorkflow()
  const plugin = workflow.plugins.find(candidate => candidate.build.mode === "manifest")
  assert.ok(plugin)
  const source = JSON.parse(await readFile(resolve(`plugins/${plugin.id}/plugin.json`), "utf8"))
  const output = await mkdtemp(resolve(tmpdir(), "chatgpt-mcp-plugin-workflow-"))
  try {
    const manifestPath = await buildPlugin(workflow, plugin.id, output)
    const manifest = JSON.parse(await readFile(manifestPath, "utf8"))
    assert.equal(manifest.id, source.id)
    assert.equal(manifest.version, source.version)
  } finally {
    await rm(output, { recursive: true, force: true })
  }
})
