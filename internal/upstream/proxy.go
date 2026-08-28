package upstream

import "strings"

func ProxyName(prefix, tool string) string {
	prefix = invalidPrefix.ReplaceAllString(strings.TrimSpace(prefix), "_")
	tool = invalidPrefix.ReplaceAllString(strings.TrimSpace(tool), "_")
	return prefix + "__" + tool
}

func ToolIsProxied(server Server, tool Tool) bool {
	for _, name := range (&Manager{}).ProxiedToolNames(server, []Tool{tool}) {
		if name == ProxyName(server.ToolPrefix, tool.Name) {
			return true
		}
	}
	return false
}
