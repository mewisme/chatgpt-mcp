package version

import (
	"fmt"
	"runtime/debug"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		if Version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
}

func String() string {
	return fmt.Sprintf("chatgpt-mcp version %s (%s) %s", Version, Commit, Date)
}

func Short() string {
	return fmt.Sprintf("%s (%s) %s", Version, Commit, Date)
}
