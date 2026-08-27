import { Home, Server, Settings, Wrench } from "lucide-react"
import { Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent, SidebarGroupLabel, SidebarMenu, SidebarMenuButton, SidebarMenuItem } from "@/components/ui/sidebar"

const items = [{ title: "Overview", icon: Home }, { title: "Tools", icon: Wrench }, { title: "MCP Servers", icon: Server }, { title: "Settings", icon: Settings }]

export function AppSidebar() {
  return <Sidebar><SidebarContent><SidebarGroup><SidebarGroupLabel>ChatGPT MCP</SidebarGroupLabel><SidebarGroupContent><SidebarMenu>{items.map((item) => <SidebarMenuItem key={item.title}><SidebarMenuButton><item.icon />{item.title}</SidebarMenuButton></SidebarMenuItem>)}</SidebarMenu></SidebarGroupContent></SidebarGroup></SidebarContent></Sidebar>
}
