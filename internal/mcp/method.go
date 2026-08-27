package mcp

func IsSupportedMethod(method string) bool {
	switch method {
	case "initialize", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}
