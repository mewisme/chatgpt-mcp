import { access, cp, mkdir, rm } from "node:fs/promises"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..")
const source = resolve(root, "web/dist")
const target = resolve(root, "internal/web/dist")

await access(resolve(source, "index.html"))
await rm(target, { recursive: true, force: true })
await mkdir(target, { recursive: true })
await cp(source, target, { recursive: true })
await access(resolve(target, "index.html"))

console.log("[OK] web embed prepared")
