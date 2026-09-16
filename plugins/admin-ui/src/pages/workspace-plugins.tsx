import { lazy, Suspense, useEffect, useState } from "react"
import { useOutletContext } from "react-router-dom"
import { PageError, PageLoading } from "@/components/page-state"
import { adminApi, type PluginConfig } from "@/lib/api"
import type { WorkspaceOutletContext } from "@/pages/workspace"

const PluginConfigCard = lazy(() => import("@/pages/settings").then((module) => ({ default: module.PluginConfigCard })))

export function WorkspacePluginsPage() {
  const { workspace } = useOutletContext<WorkspaceOutletContext>()
  const [pluginConfigs, setPluginConfigs] = useState<PluginConfig[]>([])
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState("")
  const [error, setError] = useState("")
  useEffect(() => {
    let active = true
    void adminApi.plugins("workspace", workspace.id)
      .then((plugins) => Promise.all(plugins.filter((item) => item.lifecycle.configure).map((item) => adminApi.pluginConfig(item.id, "workspace", workspace.id))))
      .then((configs) => { if (active) { setPluginConfigs(configs); setError("") } })
      .catch((value) => { if (active) setError(errorText(value)) })
    return () => { active = false }
  }, [workspace.id])
  return <Suspense fallback={<PageLoading rows={5} />}><div className="space-y-6">{error ? <PageError message={error} /> : null}{message ? <div className="text-sm text-muted-foreground">{message}</div> : null}{pluginConfigs.length === 0 ? <div className="rounded-xl border bg-card p-6 text-sm text-muted-foreground">No configurable workspace plugins are installed here.</div> : pluginConfigs.map((item) => <PluginConfigCard key={item.id} config={item} busy={busy} onChange={(next) => setPluginConfigs((current) => current.map((entry) => entry.id === next.id ? next : entry))} onBusy={setBusy} onMessage={setMessage} onError={setError} />)}</div></Suspense>
}

function errorText(value: unknown) { return value instanceof Error ? value.message : String(value) }
