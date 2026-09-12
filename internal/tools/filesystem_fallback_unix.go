//go:build !windows

package tools

import "strings"

func deleteDirectoryFallback(path string) string {
	return "rm -rf -- '" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
}
