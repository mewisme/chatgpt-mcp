package pluginhost

import (
	"context"

	cftunnelplugin "go.mewis.me/chatgpt-mcp/plugins/cf-tunnel"
)

func SyncRuntime(ctx context.Context, snap cftunnelplugin.Snapshot) {
	cftunnelplugin.Sync(ctx, snap)
}

func StopRuntime() {
	cftunnelplugin.Stop()
}

func RuntimeStatus() cftunnelplugin.Status {
	return cftunnelplugin.LiveStatus()
}

func SetRuntimeObserver(fn cftunnelplugin.LifecycleObserver) {
	cftunnelplugin.SetLiveObserver(fn)
}
