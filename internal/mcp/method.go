package mcp

func IsSupportedMethod(method string) bool {
	switch method {
	case "initialize", "notifications/initialized", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}
