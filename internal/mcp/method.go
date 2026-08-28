package mcp

func IsSupportedMethod(method string) bool {
	switch method {
	case "server/discover", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}
