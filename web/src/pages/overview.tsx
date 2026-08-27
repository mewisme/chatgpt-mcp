import { DashboardCard } from "@/components/dashboard-card"

export function OverviewPage({ data }: { data: { workspaces: number; tools: number } }) {
  return <div className="grid gap-4 md:grid-cols-2"><DashboardCard title="Workspaces" value={data.workspaces}/><DashboardCard title="Tools" value={data.tools}/></div>
}
