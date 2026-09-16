package pluginhost

import (
	"context"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
)

var RuntimeHost = runtimeplugin.NewHost()

func StopRuntime() {
	RuntimeHost.Close(context.Background())
}
