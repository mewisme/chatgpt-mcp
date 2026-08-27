import { cp, mkdir, rm } from "node:fs/promises"
import { resolve } from "node:path"

const root = process.cwd()
const source = resolve(root, "web/dist")
const target = resolve(root, "internal/web/dist")

await rm(target, { recursive: true, force: true })
await mkdir(target, { recursive: true })
await cp(source, target, { recursive: true })
