import { lazy, Suspense, useEffect, useState } from "react"
import { LoaderCircle } from "lucide-react"
import { useMatches } from "react-router-dom"
import { adminDocumentTitle, type AdminRouteHandle } from "@/lib/admin-navigation"
import { adminApi, adminToken, ApiError } from "@/lib/api"

const AuthenticatedShell = lazy(() => import("@/components/authenticated-shell").then((module) => ({ default: module.AuthenticatedShell })))
const LoginPage = lazy(() => import("@/pages/login").then((module) => ({ default: module.LoginPage })))
const fallbackHandle: AdminRouteHandle = { title: "Overview", description: "Runtime health and configuration at a glance." }

export function App() {
  const matches = useMatches()
  const route = [...matches].reverse().map((match) => match.handle).find(isAdminRouteHandle) ?? fallbackHandle
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)
  const [authRequired, setAuthRequired] = useState(true)

  useEffect(() => {
    void adminApi.health().then((health) => { setAuthRequired(health.auth_enabled); setAuthenticated(true) }).catch((value) => {
      if (value instanceof ApiError && value.status === 401) { adminToken.clear(); setAuthRequired(true); setAuthenticated(false); return }
      setAuthenticated(false)
    })
  }, [])

  useEffect(() => { document.title = adminDocumentTitle(authenticated === null ? "Connecting" : authenticated ? route.title : "Login") }, [authenticated, route.title])

  if (authenticated === null) return <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />Connecting to admin API...</div>
  if (!authenticated) return <Suspense fallback={<FullPageLoading label="Loading sign in..." />}><LoginPage onAuthenticated={() => { setAuthRequired(true); setAuthenticated(true) }} /></Suspense>

  function signOut() { adminToken.clear(); setAuthenticated(false) }

  return <Suspense fallback={<FullPageLoading label="Loading admin..." />}><AuthenticatedShell route={route} authRequired={authRequired} onSignOut={signOut} /></Suspense>
}

function isAdminRouteHandle(value: unknown): value is AdminRouteHandle { return Boolean(value && typeof value === "object" && "title" in value && "description" in value) }
function FullPageLoading({ label }: { label: string }) { return <div className="flex min-h-screen items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin" />{label}</div> }

export default App
