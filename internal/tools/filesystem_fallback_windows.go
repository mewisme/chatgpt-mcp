//go:build windows

package tools

import "strings"

func deleteDirectoryFallback(path string) string {
	return "Remove-Item -LiteralPath '" + strings.ReplaceAll(path, "'", "''") + "' -Recurse -Force"
}
