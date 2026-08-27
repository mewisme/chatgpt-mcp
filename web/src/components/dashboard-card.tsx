import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

type Props = { title: string; value: string }

export function DashboardCard({ title, value }: Props) {
  return <Card><CardHeader><CardTitle>{title}</CardTitle></CardHeader><CardContent className="text-2xl font-semibold">{value}</CardContent></Card>
}
